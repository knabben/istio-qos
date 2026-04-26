# Research: Kind Development Environment Setup

**Feature**: 001-kind-istio-setup
**Date**: 2026-04-26

## Decision 1: Kind + Local Registry Configuration

**Decision**: Use the official kind local-registry pattern — a Docker container named
`kind-registry` on host port **5000**, wired into the kind cluster via
`containerdConfigPatches` that set `localhost:5000` as a containerd mirror.

**Rationale**: This is the canonical approach documented by the kind project. It uses
containerd's native mirror configuration, avoiding Docker socket complexity. The cluster
can pull images from `localhost:5000/<image>:<tag>` inside pods transparently.

**Alternatives considered**:
- Network-alias approach: fragile across Docker versions; not idiomatic.
- `kind load docker-image`: no registry involved; slower for iterative builds.

**Key config snippet** (kind cluster YAML):
```yaml
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:5000"]
        endpoint = ["http://kind-registry:5000"]
```

---

## Decision 2: Istio Version

**Decision**: Pin `ISTIO_VERSION` as a variable at the top of `hack/install-istio.sh`,
defaulting to the current stable release at time of writing. The version MUST be explicitly
documented in both the script and `README.md` for reproducibility.

**Rationale**: Istio releases frequently. Pinning the version prevents upgrade surprises
across developer machines. Using a shell variable makes it easy to bump in one place.

**Alternatives considered**:
- Always install "latest": non-reproducible; different developers get different behavior.
- Bake version into a separate config file: extra complexity with no benefit for a single script.

---

## Decision 3: Istio Installation Method

**Decision**: Use `istioctl install --set profile=demo --skip-confirmation` for scripted
local setups. The **`demo` profile** is chosen over `minimal` (control plane only) or
`default` (production-oriented).

**Rationale**: `demo` profile enables all Istio features relevant to the controller's
testing needs, including `DestinationRule`, `VirtualService`, and automatic sidecar
injection. `istioctl` provides built-in pre-flight validation, does not require an
in-cluster operator, and needs only the binary to be present. The Istio Operator was
formally deprecated in Istio 1.23+ and MUST NOT be used.

**Alternatives considered**:
- Helm: weaker config validation; requires Helm as an additional prerequisite.
- Istio Operator: deprecated since 1.23; not suitable for new setups.

---

## Decision 4: Script Idempotency Patterns

**Decision**: Each resource creation is guarded by a pre-check:
- **Kind cluster**: `kind get clusters | grep -q "^<cluster-name>$"` → skip if found.
- **Registry container**: `docker ps -a --filter "name=kind-registry" --format "{{.Names}}" | grep -q kind-registry` → skip if found.
- **Istio installation**: `kubectl get namespace istio-system 2>/dev/null` → skip if found.
- **Cluster-registry link**: Check for the existing `containerd` mirror config in the cluster.

**Rationale**: Idempotent scripts allow developers to re-run them safely after partial
failures without needing to clean up first.

**Alternatives considered**:
- Marker files (`.bootstrapped`): Fragile; doesn't detect partial states.
- Always delete and recreate: Destructive; loses any in-cluster state.

---

## Decision 5: Script Structure

**Decision**: Two separate scripts — `hack/bootstrap.sh` (cluster + registry) and
`hack/install-istio.sh` (Istio only) — plus a `hack/teardown.sh` for cleanup.

**Rationale**: Separation allows developers to run only the step they need, matching
the two user stories in the spec. A teardown script is practical for resetting state
between test runs.

**Alternatives considered**:
- Single `hack/setup.sh` calling both: Harder to run steps independently; conflicts with
  the spec's idempotency requirement when only one step needs re-running.

---

## Decision 6: Observability Add-ons Selection

**Decision**: Install all four Istio official add-ons by default:
**Prometheus** → **Grafana** → **Jaeger** → **Kiali** (dependency order).

| Add-on     | Role | Why included |
|------------|------|-------------|
| Prometheus | Metrics collection | Required by Kiali; feeds Grafana dashboards |
| Grafana    | Metrics dashboards | Pre-built Istio panels (request rate, error rate, latency by tier) |
| Jaeger     | Distributed tracing | Trace individual requests across mesh hops |
| Kiali      | Mesh topology graph | Live service graph with tier labels, traffic rates, error percentages |

**Rationale**: The `mesh-priority-controller` exists to influence Istio routing via tier
labels. Without a live mesh topology view (Kiali) and metrics dashboards (Grafana), a
developer cannot verify that tier-based routing is working correctly. All four tools are
officially maintained by the Istio project and tested against each Istio release — no
third-party operators or Helm charts required.

**Kiali specifically**: Kiali is the only tool that shows the full picture — which pods
carry which tier label, how traffic is split across subsets, and whether the
`DestinationRule`/`VirtualService` config is valid. It is the primary debugging surface
for the controller.

**Alternatives considered**:
- Prometheus + Grafana only: Misses the topology view; Kiali is the most useful single
  tool for validating tier routing.
- Zipkin instead of Jaeger: Both work; Jaeger is the Istio default sample add-on.
- External Kiali operator (kiali-operator Helm chart): Heavier; for production clusters.
  Sample YAML is sufficient for local dev.

---

## Decision 7: Add-ons Installation Method and Bundling

**Decision**: Add-on installation is **bundled inside `hack/install-istio.sh`** at the end
of the Istio setup sequence. Add-ons are fetched directly from the official Istio GitHub
repository at the **same pinned `ISTIO_VERSION`** as the control plane.

```bash
# Installation loop inside hack/install-istio.sh (after Istio control plane is ready)
ADDONS_BASE="https://raw.githubusercontent.com/istio/istio/${ISTIO_VERSION}/samples/addons"
for addon in prometheus grafana jaeger kiali; do
  if ! kubectl get deployment "$addon" -n istio-system &>/dev/null; then
    kubectl apply -f "${ADDONS_BASE}/${addon}.yaml"
  fi
done
kubectl rollout status deployment/kiali -n istio-system --timeout=180s
```

A `SKIP_ADDONS=true` env-var guard allows skipping on resource-constrained machines.

**Rationale**:
- Bundling avoids a separate script step; most developers always want the observability
  stack. The `SKIP_ADDONS` escape hatch handles the minority case.
- Fetching from GitHub at the pinned version ensures the add-ons are tested against the
  exact Istio version being installed (Kiali in particular has version coupling with Istio).
- Using `kubectl apply` with an existence check (`kubectl get deployment`) achieves
  idempotency without `--prune` risks.

**Alternatives considered**:
- Separate `hack/install-monitoring.sh`: Extra step developers must remember to run;
  contradicts the "by default" requirement.
- Bundling add-on YAMLs in the repo: Increases repo size; they are already versioned in
  the Istio release.
