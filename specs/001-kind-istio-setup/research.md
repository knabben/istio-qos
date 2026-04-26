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

**Decision**: Pin `ISTIO_VERSION` as a variable at the top of `rec/install-istio.sh`,
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

**Decision**: Two separate scripts — `rec/bootstrap.sh` (cluster + registry) and
`rec/install-istio.sh` (Istio only) — plus a `rec/teardown.sh` for cleanup.

**Rationale**: Separation allows developers to run only the step they need, matching
the two user stories in the spec. A teardown script is practical for resetting state
between test runs.

**Alternatives considered**:
- Single `rec/setup.sh` calling both: Harder to run steps independently; conflicts with
  the spec's idempotency requirement when only one step needs re-running.
