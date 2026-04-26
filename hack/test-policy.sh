#!/usr/bin/env bash
# Applies a PodLabelerPolicy, creates a test pod, polls for the tier label,
# asserts the value, deletes the policy, verifies label removal, then cleans up.
# Exits 0 on success, 1 on failure.
set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
POLICY_NAME="test-policy-nginx"
POD_NAME="test-nginx-tier"
IMAGE="nginx:latest"
EXPECTED_TIER="standard"
POLL_INTERVAL=2
POLL_TIMEOUT=60

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

pass() { echo -e "${GREEN}[PASS]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; exit 1; }
info() { echo "[INFO] $*"; }

cleanup() {
    info "Cleaning up test resources..."
    kubectl delete pod "$POD_NAME" --namespace "$NAMESPACE" --ignore-not-found --wait=false 2>/dev/null || true
    kubectl delete podlabelerpolicy "$POLICY_NAME" --ignore-not-found 2>/dev/null || true
}
trap cleanup EXIT

# ── Step 1: Create PodLabelerPolicy ─────────────────────────────────────────
info "Creating PodLabelerPolicy '$POLICY_NAME' (imagePattern: nginx:*, tier: $EXPECTED_TIER)..."
kubectl apply -f - <<EOF
apiVersion: mesh.knabben.github.com/v1alpha1
kind: PodLabelerPolicy
metadata:
  name: ${POLICY_NAME}
spec:
  imagePattern: "nginx:*"
  tier: ${EXPECTED_TIER}
EOF

# ── Step 2: Create test pod ──────────────────────────────────────────────────
info "Creating pod '$POD_NAME' with image '$IMAGE' in namespace '$NAMESPACE'..."
kubectl run "$POD_NAME" \
    --image="$IMAGE" \
    --namespace="$NAMESPACE" \
    --restart=Never \
    --labels="app=test-tier-nginx" \
    --command -- sleep 3600

# ── Step 3: Poll for tier label ──────────────────────────────────────────────
info "Polling for label tier=$EXPECTED_TIER on pod '$POD_NAME' (timeout: ${POLL_TIMEOUT}s)..."
elapsed=0
while true; do
    actual_tier=$(kubectl get pod "$POD_NAME" --namespace "$NAMESPACE" \
        -o jsonpath='{.metadata.labels.tier}' 2>/dev/null || true)
    if [[ "$actual_tier" == "$EXPECTED_TIER" ]]; then
        pass "Pod '$POD_NAME' has label tier=$actual_tier after ${elapsed}s"
        break
    fi
    if (( elapsed >= POLL_TIMEOUT )); then
        fail "Timed out after ${POLL_TIMEOUT}s waiting for tier=$EXPECTED_TIER (got: '${actual_tier:-<none>}')"
    fi
    sleep "$POLL_INTERVAL"
    (( elapsed += POLL_INTERVAL ))
done

# ── Step 4: Delete policy; verify label removal ──────────────────────────────
info "Deleting PodLabelerPolicy '$POLICY_NAME'..."
kubectl delete podlabelerpolicy "$POLICY_NAME"

info "Polling for tier label removal from pod '$POD_NAME' (timeout: ${POLL_TIMEOUT}s)..."
elapsed=0
while true; do
    actual_tier=$(kubectl get pod "$POD_NAME" --namespace "$NAMESPACE" \
        -o jsonpath='{.metadata.labels.tier}' 2>/dev/null || true)
    if [[ -z "$actual_tier" ]]; then
        pass "Pod '$POD_NAME' has no tier label after policy deletion (${elapsed}s)"
        break
    fi
    if (( elapsed >= POLL_TIMEOUT )); then
        fail "Timed out after ${POLL_TIMEOUT}s; tier label still present: '$actual_tier'"
    fi
    sleep "$POLL_INTERVAL"
    (( elapsed += POLL_INTERVAL ))
done

pass "All assertions passed."
