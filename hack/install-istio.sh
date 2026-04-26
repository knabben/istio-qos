#!/usr/bin/env bash
set -euo pipefail

# Default configuration — override via environment variables
CLUSTER_NAME="${CLUSTER_NAME:-istio-qos}"
ISTIO_VERSION="${ISTIO_VERSION:-1.24.2}"
ISTIO_PROFILE="${ISTIO_PROFILE:-demo}"

# Timeout (seconds) to wait for Istio pods to become ready
READY_TIMEOUT="${READY_TIMEOUT:-300}"

log()  { echo "[install-istio] $*"; }
err()  { echo "[install-istio] ERROR: $*" >&2; }

# ---------------------------------------------------------------------------
# Prerequisite validation
# ---------------------------------------------------------------------------
check_prerequisites() {
  local ok=true

  if ! command -v kubectl >/dev/null 2>&1; then
    err "'kubectl' not found in PATH. Install: https://kubernetes.io/docs/tasks/tools/"
    ok=false
  fi

  if ! kubectl cluster-info --context "kind-${CLUSTER_NAME}" >/dev/null 2>&1; then
    err "Cannot reach cluster 'kind-${CLUSTER_NAME}'."
    err "Run 'bash rec/bootstrap.sh' first, then retry."
    ok=false
  fi

  if ! command -v istioctl >/dev/null 2>&1; then
    err "'istioctl' not found in PATH."
    err "Download: https://istio.io/latest/docs/setup/install/istioctl/"
    err "  curl -L https://istio.io/downloadIstio | ISTIO_VERSION=${ISTIO_VERSION} sh -"
    ok=false
  fi

  if [[ "$ok" != "true" ]]; then
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# Idempotency check
# ---------------------------------------------------------------------------
istio_already_installed() {
  kubectl get namespace istio-system --context "kind-${CLUSTER_NAME}" >/dev/null 2>&1
}

# ---------------------------------------------------------------------------
# Install Istio
# ---------------------------------------------------------------------------
install_istio() {
  log "Installing Istio ${ISTIO_VERSION} (profile: ${ISTIO_PROFILE}) ..."
  if ! istioctl install \
      --set profile="${ISTIO_PROFILE}" \
      --context "kind-${CLUSTER_NAME}" \
      --skip-confirmation; then
    err "istioctl install failed."
    exit 2
  fi
}

# ---------------------------------------------------------------------------
# Wait for readiness
# ---------------------------------------------------------------------------
wait_for_ready() {
  log "Waiting for Istio pods to become ready (timeout: ${READY_TIMEOUT}s) ..."
  local elapsed=0
  local interval=10

  until kubectl wait pod \
      --all \
      --for=condition=Ready \
      --namespace=istio-system \
      --context="kind-${CLUSTER_NAME}" \
      --timeout="${interval}s" >/dev/null 2>&1; do
    elapsed=$(( elapsed + interval ))
    if (( elapsed >= READY_TIMEOUT )); then
      err "Timeout after ${READY_TIMEOUT}s waiting for Istio pods."
      err "Check pod status: kubectl get pods -n istio-system"
      exit 3
    fi
    log "  ... still waiting (${elapsed}s elapsed)"
  done
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  check_prerequisites

  if istio_already_installed; then
    log "Istio already installed in 'istio-system' — skipping."
    exit 0
  fi

  install_istio
  wait_for_ready

  log "Done. Istio ${ISTIO_VERSION} is ready."
  log ""
  log "Verify with:"
  log "  kubectl get pods -n istio-system"
  log "  kubectl get crd | grep istio.io"
  log ""
  log "Enable sidecar injection in a namespace:"
  log "  kubectl label namespace <ns> istio-injection=enabled"
}

main "$@"
