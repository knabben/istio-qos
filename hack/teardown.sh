#!/usr/bin/env bash
set -euo pipefail

# Default configuration — override via environment variables
CLUSTER_NAME="${CLUSTER_NAME:-istio-qos}"
REGISTRY_NAME="${REGISTRY_NAME:-kind-registry}"

log()  { echo "[teardown] $*"; }

# ---------------------------------------------------------------------------
# Delete kind cluster
# ---------------------------------------------------------------------------
delete_cluster() {
  if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    log "Cluster '${CLUSTER_NAME}' not found — skipping."
    return
  fi

  log "Deleting kind cluster '${CLUSTER_NAME}' ..."
  kind delete cluster --name "${CLUSTER_NAME}"
}

# ---------------------------------------------------------------------------
# Stop and remove registry container
# ---------------------------------------------------------------------------
remove_registry() {
  if ! docker ps -a --filter "name=^${REGISTRY_NAME}$" --format '{{.Names}}' \
      | grep -q "^${REGISTRY_NAME}$"; then
    log "Registry '${REGISTRY_NAME}' not found — skipping."
    return
  fi

  log "Removing registry container '${REGISTRY_NAME}' ..."
  docker rm -f "${REGISTRY_NAME}" >/dev/null
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  delete_cluster
  remove_registry

  log "Done. Local environment torn down."
}

main "$@"
