#!/usr/bin/env bash
set -euo pipefail

# Default configuration — override via environment variables
CLUSTER_NAME="${CLUSTER_NAME:-istio-qos}"
ISTIO_VERSION="${ISTIO_VERSION:-1.29.0}"
ISTIO_PROFILE="${ISTIO_PROFILE:-demo}"
READY_TIMEOUT="${READY_TIMEOUT:-300}"
# Set SKIP_ADDONS=true to bypass Prometheus/Grafana/Jaeger/Kiali installation
SKIP_ADDONS="${SKIP_ADDONS:-false}"

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
    err "Run 'bash hack/bootstrap.sh' first, then retry."
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
# Idempotency check — returns 0 if istio-system already exists
# ---------------------------------------------------------------------------
istio_already_installed() {
  kubectl get namespace istio-system --context "kind-${CLUSTER_NAME}" >/dev/null 2>&1
}

# ---------------------------------------------------------------------------
# Install Istio control plane
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
# Wait for Istio control-plane pods to reach Ready
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
# Install observability add-ons: Prometheus → Grafana → Jaeger → Kiali
# Each add-on is checked individually for idempotency.
# ---------------------------------------------------------------------------
install_addons() {
  local base="https://raw.githubusercontent.com/istio/istio/${ISTIO_VERSION}/samples/addons"
  local ctx="kind-${CLUSTER_NAME}"

  log "Installing observability add-ons (Prometheus → Grafana → Jaeger → Kiali) ..."

  for addon in prometheus grafana jaeger kiali; do
    if kubectl get deployment "${addon}" \
        -n istio-system --context "${ctx}" &>/dev/null; then
      log "  ${addon}: already installed — skipping."
    else
      log "  Installing ${addon} ..."
      kubectl apply -f "${base}/${addon}.yaml" --context "${ctx}"
    fi
  done

  log "Waiting for Kiali to be ready (timeout: 180s) ..."
  kubectl rollout status deployment/kiali \
    -n istio-system \
    --context "${ctx}" \
    --timeout=180s
}

# ---------------------------------------------------------------------------
# Print access summary
# ---------------------------------------------------------------------------
print_summary() {
  log ""
  log "Setup complete. Istio ${ISTIO_VERSION} is ready."
  log ""
  log "Verify control plane:"
  log "  kubectl get pods -n istio-system"
  log "  kubectl get crd | grep istio.io"

  if [[ "${SKIP_ADDONS}" != "true" ]]; then
    log ""
    log "Access observability dashboards (each opens a browser tab via port-forward):"
    log "  istioctl dashboard kiali      # Service graph + tier routing  (port 20001)"
    log "  istioctl dashboard grafana    # Istio metrics dashboards       (port 3000)"
    log "  istioctl dashboard jaeger     # Distributed tracing            (port 16686)"
    log "  istioctl dashboard prometheus # Metrics query UI               (port 9090)"
  fi

  log ""
  log "Enable sidecar injection in a namespace:"
  log "  kubectl label namespace <ns> istio-injection=enabled"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  check_prerequisites

  if istio_already_installed; then
    log "Istio already installed in 'istio-system' — skipping control plane install."
  else
    install_istio
    wait_for_ready
  fi

  if [[ "${SKIP_ADDONS}" == "true" ]]; then
    log "Skipping observability add-ons (SKIP_ADDONS=true)."
  else
    install_addons
  fi

  print_summary
}

main "$@"
