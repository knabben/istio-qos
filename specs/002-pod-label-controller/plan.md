# Implementation Plan: Pod Tier Label Controller

**Branch**: `002-pod-label-controller` | **Date**: 2026-04-26 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/002-pod-label-controller/spec.md`

## Summary

Implement a Kubernetes controller using kubebuilder v4 that watches `PodLabelerPolicy` CRDs
and applies `tier: high | standard` labels to pods via server-side apply with field ownership.
The controller drives Istio DestinationRule subset selection for premium/standard traffic
routing. All reads come from the informer cache; writes use `client.Apply` with a fixed
field manager. Leader election is mandatory and non-configurable.

## Technical Context

**Language/Version**: Go 1.22+
**Primary Dependencies**: kubebuilder v4, controller-runtime v0.19+, client-go, `github.com/gobwas/glob` for image pattern matching
**Storage**: Kubernetes API server (via informer cache for reads, server-side apply for writes)
**Testing**: `go test ./...` with `-race`; `sigs.k8s.io/controller-runtime/pkg/envtest`; Ginkgo v2 suite
**Target Platform**: Kubernetes cluster (kind for dev, any conformant cluster for prod)
**Project Type**: Kubernetes controller (kubebuilder scaffold)
**Performance Goals**: Label applied within 5 s of policy creation under normal load; no unbounded LIST calls during reconcile bursts
**Constraints**: `NotFound` → `ctrl.Result{}, nil` (no error return); all writes via `client.Apply`; `LeaderElection: true` hardcoded; coverage ≥ 80% for reconciler package
**Scale/Scope**: Cluster-wide pod watch; fan-out all pods on any policy event (v1 scope; field index deferred to v2)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|----------|
| I. Label Correctness | PASS | FR-001–FR-003 mandate correct label per policy match; FR-007 defines deterministic tie-break |
| II. Label Stability | PASS | FR-002 server-side apply with diff-gate (invariant III in reconciler contract) prevents redundant writes |
| III. Policy-Driven Classification | PASS | FR-001 requires all tier assignment from `PodLabelerPolicy` resources; no hardcoded tiers |
| IV. Fleet-Safe Engineering | PASS | envtest integration tests required (SC-006); fail-closed on cache error (FR-005); leader election non-configurable (FR-006) |
| V. Observability | PASS | FR-009 Kubernetes Events on every label mutation; FR-010 health endpoints; Prometheus metrics in reconciler contract |

**Post-Phase-1 re-check**: All invariants satisfied. No constitution violations introduced.

## Project Structure

### Documentation (this feature)

```text
specs/002-pod-label-controller/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0: tech decisions
├── data-model.md        # Phase 1: CRD schema, state transitions
├── quickstart.md        # Phase 1: deploy & verify steps
├── contracts/
│   └── reconciler.md    # Phase 1: reconcile contract table, RBAC, metrics
└── tasks.md             # Phase 2 output (/speckit-tasks command)
```

### Source Code (repository root)

```text
.
├── api/
│   └── v1alpha1/
│       ├── podlabelerpolicy_types.go      # CRD types + validation markers
│       └── zz_generated.deepcopy.go       # generated
├── cmd/
│   └── main.go                            # Manager setup, leader election, health probes
├── config/
│   ├── crd/bases/                         # Generated CRD manifests
│   ├── rbac/                              # ClusterRole, ClusterRoleBinding, ServiceAccount
│   ├── manager/
│   │   ├── manager.yaml                   # Deployment (2 replicas, security context)
│   │   └── kustomization.yaml
│   └── samples/
│       ├── mesh_v1alpha1_podlabelerpolicy_high.yaml
│       ├── mesh_v1alpha1_podlabelerpolicy_standard.yaml
│       ├── deployment.yaml                # 4-pod deployment (2 high-image, 2 standard-image)
│       ├── service.yaml
│       ├── destinationrule.yaml           # high-priority-pods + standard-pods subsets
│       └── virtualservice.yaml            # user-type: premium → high, default → standard
├── hack/
│   ├── bootstrap.sh                       # Kind cluster + local registry (feature 001)
│   ├── install-istio.sh                   # Istio install (feature 001)
│   ├── teardown.sh                        # Teardown (feature 001)
│   └── test-policy.sh                     # NEW: apply policy, assert tier label, cleanup
├── internal/
│   ├── controller/
│   │   ├── podlabelerpolicy_controller.go # Primary reconciler
│   │   ├── podlabelerpolicy_controller_test.go
│   │   └── suite_test.go                  # Ginkgo + envtest bootstrap
│   └── matcher/
│       ├── matcher.go                     # Glob image pattern matching (pure, no k8s dep)
│       └── matcher_test.go
├── Dockerfile
├── Makefile
└── go.mod
```

**Structure Decision**: Standard kubebuilder v4 layout. `internal/matcher` isolates glob matching for pure unit testing without Kubernetes machinery. `config/samples/` provides the full Istio routing reference example.

## Phase 0: Research Summary

See [research.md](research.md) for full decision rationale.

| Decision | Choice | Key Rationale |
|----------|--------|--------------|
| CRD scaffold | kubebuilder v4, `--namespaced=false` | Cluster-scoped; active release line |
| Label writes | `client.Apply` + `client.FieldOwner("mesh-priority-controller")` | Field ownership; conflict detected by API server, not client |
| Image matching | `github.com/gobwas/glob` | Handles `/` in registry prefixes; DFA-compiled; well-maintained |
| Cross-type watch | `handler.EnqueueRequestsFromMapFunc` (all pods on policy event) | Simple, correct, idempotent reconciler makes redundant enqueues harmless |
| Integration tests | `controller-runtime/pkg/envtest` | No live cluster; reproducible in CI |
| Leader election | Hardcoded `true`; `LeaderElectionID: "mesh-priority-controller.knabben.github.com"` | Prevents accidental disabling |

## Phase 1: Design Summary

See [data-model.md](data-model.md), [contracts/reconciler.md](contracts/reconciler.md), [quickstart.md](quickstart.md).

**CRD**: `PodLabelerPolicy` — cluster-scoped, `spec.imagePattern` (glob string), `spec.tier` (enum: high|standard), `status.matchedPods` (int32), `status.conditions`.

**Reconcile Algorithm**:
1. Fetch pod from cache; if `NotFound` → `return ctrl.Result{}, nil`
2. List all `PodLabelerPolicy` resources from cache
3. For each policy, compile glob and match against each container image
4. Resolve conflicts: sort matching policies by name; first entry wins; emit Warning event if tiers differ
5. Diff-gate: compare computed tier against pod's current `tier` label — skip write if equal
6. Construct minimal SSA patch (pod with only `tier` label); call `r.Patch(ctx, patch, client.Apply, client.FieldOwner(...), client.ForceOwnership)`
7. Emit `TierLabelApplied` or `TierLabelRemoved` event; increment Prometheus counter

**RBAC**: ClusterRole with `pods: get, list, watch, patch`; `podlabelerpolicies: get, list, watch`; `podlabelerpolicies/status: update, patch`; `events: create, patch`; `leases: get, list, watch, create, update, patch, delete`.

## Implementation Notes

### Forbidden Patterns

The following patterns MUST NOT appear anywhere in the reconciler or controller package:

| Pattern | Reason |
|---------|--------|
| `client.Update(ctx, pod)` on Pod | Overwrites entire object; loses concurrent changes; violates SSA invariant |
| `client.Patch(ctx, pod, client.MergeFrom(...))` on labels | Strategic merge patch has no field ownership; silent overwrite |
| `LeaderElection: false` | Forbidden by FR-006 and Constitution §IV |
| `time.Sleep(...)` in Reconcile | Blocks reconcile goroutine; use `RequeueAfter` instead |
| `ctrl.Result{Requeue: true}` without `RequeueAfter` | Tight-loop requeue; use exponential backoff from controller-runtime |
| Direct API server reads (`r.Client.Get` bypassing cache) | Violates FR-004; use cache-backed client only |

### Required Integration Tests

Three tests MUST be present with these exact names:

| Test Name | Verifies |
|-----------|---------|
| `TestReconcile_NoLostUpdates` | SSA write preserves concurrent unrelated field changes; no MergePatch used |
| `TestReconcile_CacheNotFound` | `NotFound` from cache → `ctrl.Result{}, nil`; no error returned; no panic |
| `TestReconcile_LeaderElection` | Manager refuses to start when `LeaderElection: false` is set programmatically |

### Prometheus Metrics

| Metric | Type | Labels |
|--------|------|--------|
| `mesh_priority_labels_applied_total` | Counter | `tier`, `namespace` |
| `mesh_priority_labels_skipped_total` | Counter | `namespace` |
| `mesh_priority_reconcile_errors_total` | Counter | `error_category` |
| `mesh_priority_policy_evaluations_total` | Counter | `result` |

### Deployment Spec Constraints

- `replicas: 2`
- `securityContext.runAsNonRoot: true`
- `securityContext.readOnlyRootFilesystem: true`
- `resources.requests: {cpu: 50m, memory: 64Mi}`
- `resources.limits: {cpu: 200m, memory: 128Mi}`
- Namespace: `mesh-priority-system`
- LeaderElectionID: `mesh-priority-controller.knabben.github.com`

### Commit Conventions

All commits in this feature MUST use Conventional Commits format:
- `feat:` new behavior
- `fix:` bug corrections
- `test:` test-only changes
- `chore:` scaffolding, build, config
- `docs:` documentation only

## Complexity Tracking

No constitution violations. No complexity justification required.
