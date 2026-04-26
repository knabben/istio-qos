#!/usr/bin/env bash
set -euo pipefail

# Default configuration — override via environment variables
CLUSTER_NAME="${CLUSTER_NAME:-istio-qos}"
REGISTRY_PORT="${REGISTRY_PORT:-5000}"
REGISTRY_NAME="${REGISTRY_NAME:-kind-registry}"

log()  { echo "[bootstrap] $*"; }
err()  { echo "[bootstrap] ERROR: $*" >&2; }

# ---------------------------------------------------------------------------
# Prerequisite validation
# ---------------------------------------------------------------------------
check_prerequisites() {
  local ok=true

  if ! docker info >/dev/null 2>&1; then
    err "Docker is not running. Start Docker and retry."
    ok=false
  fi

  if ! command -v kind >/dev/null 2>&1; then
    err "'kind' not found in PATH. Install: https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
    ok=false
  fi

  if ! command -v kubectl >/dev/null 2>&1; then
    err "'kubectl' not found in PATH. Install: https://kubernetes.io/docs/tasks/tools/"
    ok=false
  fi

  if [[ "$ok" != "true" ]]; then
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# Registry
# ---------------------------------------------------------------------------
start_registry() {
  if docker ps -a --filter "name=^${REGISTRY_NAME}$" --format '{{.Names}}' \
      | grep -q "^${REGISTRY_NAME}$"; then
    log "Registry '${REGISTRY_NAME}' already exists — skipping."
    return
  fi

  log "Starting local registry '${REGISTRY_NAME}' on port ${REGISTRY_PORT} ..."
  docker run -d \
    --name "${REGISTRY_NAME}" \
    --restart=always \
    -p "${REGISTRY_PORT}:5000" \
    registry:2
}

# ---------------------------------------------------------------------------
# Kind cluster
# ---------------------------------------------------------------------------
create_cluster() {
  if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    log "Cluster '${CLUSTER_NAME}' already exists — skipping."
    return
  fi

  log "Creating kind cluster '${CLUSTER_NAME}' ..."
  kind create cluster --name "${CLUSTER_NAME}" --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${REGISTRY_PORT}"]
        endpoint = ["http://${REGISTRY_NAME}:5000"]
EOF
}

# ---------------------------------------------------------------------------
# Connect registry to the kind Docker network
# ---------------------------------------------------------------------------
connect_registry_to_cluster() {
  local network="kind"

  if docker network inspect "${network}" \
      --format '{{range .Containers}}{{.Name}} {{end}}' 2>/dev/null \
      | grep -qw "${REGISTRY_NAME}"; then
    log "Registry already connected to '${network}' network — skipping."
    return
  fi

  log "Connecting registry to '${network}' network ..."
  docker network connect "${network}" "${REGISTRY_NAME}" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  check_prerequisites
  start_registry
  create_cluster
  connect_registry_to_cluster

  log "Done."
  log "  Cluster : ${CLUSTER_NAME}"
  log "  Registry: localhost:${REGISTRY_PORT}"
  log ""
  log "Verify with:"
  log "  kubectl get nodes"
  log "  curl http://localhost:${REGISTRY_PORT}/v2/_catalog"
}

main "$@"
