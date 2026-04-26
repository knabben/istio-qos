# Implementation Plan: PodLabeler Bug Baseline (Act I)

**Branch**: `003-podlabeler-bug-baseline` | **Date**: 2026-04-26 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/003-podlabeler-bug-baseline/spec.md`

## Summary

Extract the Act I baseline from `bugs/files.zip` into a proper Go module at
`act1/`, preserving every bug faithfully, and add an envtest integration test suite
(`controller/reconciler_test.go`) with exactly three tests — one per bug — each
written to assert correct behavior and therefore **FAIL** against the buggy
implementation. The module is `github.com/knabben/istio-poc`; no kubebuilder,
no code generation. The `bugs/` directory is removed after extraction.

## Technical Context

**Language/Version**: Go 1.22 (matches bugs/files.zip go.mod)
**Primary Dependencies**: `sigs.k8s.io/controller-runtime v0.17.3`, `k8s.io/api v0.29.3`,
`k8s.io/apimachinery v0.29.3`, `k8s.io/client-go v0.29.3`, `sigs.k8s.io/controller-runtime/pkg/envtest` (test only)
**Storage**: Kubernetes API server (envtest fake server for tests)
**Testing**: `controller-runtime/pkg/envtest` loaded from `manifests/crd.yaml`; no setup-envtest binary needed for unit tests; envtest binary used for integration tests
**Target Platform**: Linux (envtest), any Kubernetes cluster for manifests
**Project Type**: Standalone Go module; teaching baseline (not production controller)
**Performance Goals**: Not applicable — teaching demo; test suite must complete in < 60 s
**Constraints**: Must be a faithful copy of bugs/files.zip; zero changes to reconciler.go or main.go logic; DeepCopy by hand; no kubebuilder markers; no controller-gen
**Scale/Scope**: Single-node envtest for tests; 2-replica kind deployment for live demo

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `mesh-priority-controller` constitution v1.0.0:

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Label Correctness | **INTENTIONAL VIOLATION** | Bug 1 (lost update) causes incorrect labels under concurrency. Required by FR-006. |
| II. Label Stability | **INTENTIONAL VIOLATION** | Bug 3 (no leader election) + Bug 1 cause label oscillation. Required by FR-008. |
| III. Policy-Driven Classification | Pass | Labels are derived from PodLabelerPolicy CRs. Pattern matching is deterministic. |
| IV. Fleet-Safe Engineering | **INTENTIONAL VIOLATION** | No production-grade tests. Test suite is written to FAIL. Required by FR-010. |
| V. Observability | **INTENTIONAL VIOLATION** | No Prometheus metrics. Required by the "no scaffolding" constraint. |

**Complexity Justification**: All four violations are required by the spec (FR-006 through
FR-010). This implementation is a teaching artifact — the violations ARE the feature. The
spec explicitly requires the broken scenarios so Act II can demonstrate the fixes. No
production-grade alternative exists because the broken code is the deliverable.

**Operational Constraints**: This implementation does NOT respect API rate limiting, does
NOT implement leader election, and does NOT deduplicate events. All are intentional and
documented in the spec as Bug 1–3.

**Result**: Constitution violations are justified. Proceed.

## Project Structure

### Documentation (this feature)

```text
specs/003-podlabeler-bug-baseline/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── reconciler.md   # Reconciler behavior contract (including bug contracts)
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code

```text
act1/                                    # Extracted from bugs/files.zip; bugs/ removed after
├── go.mod                               # module github.com/knabben/istio-poc
├── go.sum
├── main.go                              # Manager — Bug 3 (LeaderElection: false)
├── api/
│   └── v1alpha1/
│       ├── types.go                     # PodLabelerPolicy + PodLabelerPolicyList
│       └── register.go                  # Scheme registration, hand-written
├── controller/
│   ├── reconciler.go                    # Reconciler — Bug 1 (Update) + Bug 2 (NotFound)
│   └── reconciler_test.go              # 3 bug-exposure envtest tests (all FAIL on Act I)
└── manifests/
    ├── crd.yaml                         # Hand-written PodLabelerPolicy CRD
    ├── rbac.yaml                        # SA + ClusterRole + ClusterRoleBinding
    ├── controller.yaml                  # Deployment, replicas: 2 (Bug 3)
    ├── sample-policies.yaml             # app1 → tier=high, app2 → tier=standard
    ├── sample-workload.yaml             # Two Deployments + ClusterIP Service
    └── istio/
        ├── destinationrule.yaml         # Subsets: high-priority-pods, standard-pods
        └── virtualservice.yaml          # user-type: premium → high, default → standard
```

**Structure Decision**: Standalone module at `act1/` — isolated from the main `istio-qos`
module to avoid dependency conflicts. The `bugs/` directory (containing only `files.zip`)
is removed after extraction is complete (T026).

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|--------------------------------------|
| Constitution Principles I, II, IV, V violated | The buggy behavior is the deliverable | Fixing the bugs would defeat the teaching purpose of Act I |
| Tests written to FAIL | Proves bugs exist; proves Act II fixes them | Tests that skip or pass would not demonstrate the failure |
