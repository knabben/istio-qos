# Tasks: Pod Tier Label Controller

**Input**: Design documents from `specs/002-pod-label-controller/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/reconciler.md ✅

**Tests**: Included — spec §IV mandates three integration tests with exact function names.

**Organization**: Tasks grouped by user story. US1/US2/US3 are all P1 and sequential within their phases; US4 is P2 and independent.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to
- Exact file paths are given for every task

---

## Phase 1: Setup (kubebuilder Scaffold)

**Purpose**: Bootstrap the kubebuilder project structure and add missing dependencies.

- [ ] T001 Run `kubebuilder init --domain knabben.github.com --repo github.com/knabben/istio-qos` in repo root (generates go.mod, cmd/main.go, Makefile, Dockerfile, PROJECT)
- [ ] T002 Run `kubebuilder create api --group mesh --version v1alpha1 --kind PodLabelerPolicy --namespaced=false --resource --controller` (generates api/v1alpha1/ types + internal/controller/ stub)
- [ ] T003 [P] Add `github.com/gobwas/glob` to go.mod via `go get github.com/gobwas/glob` and run `go mod tidy`
- [ ] T004 [P] Configure `.golangci.yml` at repo root with `govet`, `errcheck`, `staticcheck`, `unused`, `misspell` linters enabled

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: CRD types, matcher package, envtest suite bootstrap, and metrics registration. All user stories depend on this phase.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T005 Define PodLabelerPolicy spec (`imagePattern string`, `tier string`) and status (`matchedPods int32`, `conditions []metav1.Condition`) with kubebuilder markers (`+kubebuilder:validation:Enum=high;standard`, `+kubebuilder:resource:scope=Cluster`) in `api/v1alpha1/podlabelerpolicy_types.go`
- [ ] T006 Run `make generate manifests` to produce CRD YAML (`config/crd/bases/`), deepcopy code, and RBAC role stubs
- [ ] T007 [P] Implement `internal/matcher/matcher.go`: `Compile(pattern string) (Matcher, error)` wrapping `gobwas/glob`, and `MatchesAnyImage(images []string) bool`; no Kubernetes imports
- [ ] T008 [P] Write unit tests in `internal/matcher/matcher_test.go` covering: exact match, wildcard `*`, registry prefix `*/image:*`, no-match, empty pattern error; table-driven with `testing.T`
- [ ] T009 Bootstrap Ginkgo v2 + envtest suite in `internal/controller/suite_test.go`: `envtest.Environment{CRDDirectoryPaths: ["../../config/crd/bases"]}`, scheme registration for `corev1` and `meshv1alpha1`, `BeforeSuite`/`AfterSuite` hooks
- [ ] T010 Register four Prometheus counters at package init in `internal/controller/metrics.go`: `mesh_priority_labels_applied_total{tier,namespace}`, `mesh_priority_labels_skipped_total{namespace}`, `mesh_priority_reconcile_errors_total{error_category}`, `mesh_priority_policy_evaluations_total{result}`; use `prometheus.MustRegister`

**Checkpoint**: Foundational ready — `make generate` passes, `go build ./...` passes, matcher unit tests pass.

---

## Phase 3: User Story 1 — Controller Applies Tier Labels to Matching Pods (Priority: P1) 🎯 MVP

**Goal**: Pods matching a `PodLabelerPolicy` receive `tier: high|standard` via SSA; pods that stop matching have the label removed.

**Independent Test**: Create a `PodLabelerPolicy` matching `nginx:*` → `tier: standard`; create a pod running `nginx:latest`; verify pod gains `tier=standard`; delete the policy; verify label removed.

### Tests for User Story 1

- [ ] T011 [P] [US1] Write envtest integration test `TestReconcile_LabelApplied` in `internal/controller/podlabelerpolicy_controller_test.go`: create PodLabelerPolicy + Pod, trigger reconcile, assert `pod.Labels["tier"] == policy.Spec.Tier`
- [ ] T012 [P] [US1] Write envtest integration test `TestReconcile_LabelRemoved` in `internal/controller/podlabelerpolicy_controller_test.go`: delete policy, trigger reconcile, assert `tier` label absent from pod

### Implementation for User Story 1

- [ ] T013 [US1] Implement `Reconcile(ctx, req)` in `internal/controller/podlabelerpolicy_controller.go`:
  1. Fetch pod from cache; if `apierrors.IsNotFound` → `return ctrl.Result{}, nil`
  2. List all `PodLabelerPolicy` from cache
  3. For each policy, use `matcher.Compile(policy.Spec.ImagePattern)` and match each container image
  4. Collect matching policies; sort by `policy.Name`; use first; if tiers differ emit `TierConflict` Warning event
  5. Diff-gate: if computed tier equals current `pod.Labels["tier"]` → skip write, increment `mesh_priority_labels_skipped_total`
  6. Construct minimal SSA patch: `&corev1.Pod{TypeMeta: ..., ObjectMeta: {Name, Namespace, Labels: {"tier": tier}}}` (or omit `tier` for removal)
  7. Call `r.Patch(ctx, patch, client.Apply, client.FieldOwner("mesh-priority-controller"), client.ForceOwnership)`
  8. Increment `mesh_priority_labels_applied_total`; return `ctrl.Result{}, err`
- [ ] T014 [US1] Register cross-type watches in `SetupWithManager()` in `internal/controller/podlabelerpolicy_controller.go`:
  - Primary: `For(&corev1.Pod{})` (enqueue pod's NamespacedName on pod events)
  - Secondary: `Watches(&meshv1alpha1.PodLabelerPolicy{}, handler.EnqueueRequestsFromMapFunc(mapPolicyToPods))` where `mapPolicyToPods` lists all pods cluster-wide and enqueues each
- [ ] T015 [US1] Emit Kubernetes Events in `internal/controller/podlabelerpolicy_controller.go`: `r.Recorder.Event(pod, corev1.EventTypeNormal, "TierLabelApplied", msg)`, `"TierLabelRemoved"`, and `corev1.EventTypeWarning, "TierConflict"` with message including policy name, old tier, new tier
- [ ] T016 [US1] Add structured `Info`-level log entries in `internal/controller/podlabelerpolicy_controller.go` for every label mutation: fields `pod`, `namespace`, `old_tier` (or `<none>`), `new_tier`, `policy`
- [ ] T017 [US1] Increment `mesh_priority_policy_evaluations_total` per policy evaluation and `mesh_priority_reconcile_errors_total{error_category}` on any error return in `internal/controller/podlabelerpolicy_controller.go`

**Checkpoint**: `make test` passes; a pod running an nginx image receives `tier=standard` when a matching policy exists, and the label is removed when the policy is deleted.

---

## Phase 4: User Story 2 — No Lost Updates Under Concurrent Writes (Priority: P1)

**Goal**: All pod label writes use SSA with field ownership; `NotFound` from cache is handled as a transient non-error condition.

**Independent Test**: Simulate a concurrent unrelated annotation change on the pod between reconcile read and write; confirm the annotation is preserved and the tier label is correctly set.

### Tests for User Story 2

- [ ] T018 [US2] Write `TestReconcile_NoLostUpdates` in `internal/controller/podlabelerpolicy_controller_test.go`: using envtest, inject a concurrent annotation on the pod after cache read (via direct client write); run reconcile; assert annotation is still present AND `tier` label is set; assert NO `client.Update` was called on Pod (use a fake recorder or wrap client)
- [ ] T019 [US2] Write `TestReconcile_CacheNotFound` in `internal/controller/podlabelerpolicy_controller_test.go`: request reconcile for a non-existent pod name; assert `ctrl.Result{} == result` and `err == nil`; assert no panic and no log at Error level

### Implementation for User Story 2

- [ ] T020 [US2] Verify and harden `apierrors.IsNotFound` branch in `internal/controller/podlabelerpolicy_controller.go`: log at Debug level (`logger.V(1).Info("pod not found, skipping")`), return `ctrl.Result{}, nil` — no error propagation, no requeue timer
- [ ] T021 [US2] Add Makefile target `vet-no-forbidden` using `grep -rn "\.Update(ctx, \&.*Pod\|MergeFrom" internal/controller/` to enforce no forbidden write patterns at CI time in `Makefile`

**Checkpoint**: `TestReconcile_NoLostUpdates` and `TestReconcile_CacheNotFound` both pass with `-race`.

---

## Phase 5: User Story 3 — Controller Runs with Leader Election (Priority: P1)

**Goal**: Manager is configured with mandatory, non-configurable leader election; startup validation rejects `LeaderElection: false`.

**Independent Test**: Start manager with `LeaderElection: false` programmatically; assert it exits before calling `mgr.Start`; confirm via test.

### Tests for User Story 3

- [ ] T022 [US3] Write `TestReconcile_LeaderElection` in `internal/controller/podlabelerpolicy_controller_test.go`: call a `validateManagerOptions(opts ctrl.Options) error` function with `opts.LeaderElection = false`; assert non-nil error returned; assert error message contains "leader election"

### Implementation for User Story 3

- [ ] T023 [US3] Add `validateManagerOptions(opts ctrl.Options) error` function in `cmd/main.go` returning an error if `!opts.LeaderElection`; call it before `mgr.Start(ctx)` and `log.Error` + `os.Exit(1)` on failure
- [ ] T024 [US3] Configure `ctrl.Options` in `cmd/main.go` with hardcoded values:
  ```
  LeaderElection:              true,
  LeaderElectionID:            "mesh-priority-controller.knabben.github.com",
  LeaderElectionReleaseOnCancel: true,
  HealthProbeBindAddress:      ":8081",
  MetricsBindAddress:          ":8080",
  ```
  Register `/healthz` liveness and `/readyz` readiness probes via `mgr.AddHealthzCheck` / `mgr.AddReadyzCheck`

**Checkpoint**: `TestReconcile_LeaderElection` passes; manager binary exits non-zero when started with `LeaderElection: false`; `go test -race ./...` passes for controller package.

---

## Phase 6: User Story 4 — Istio Traffic Split by Tier Example (Priority: P2)

**Goal**: `config/samples/` contains a complete deployable example: 4-pod Deployment, Service, two PodLabelerPolicies, DestinationRule with two subsets, VirtualService routing premium header to high subset.

**Independent Test**: Apply all samples to kind cluster; confirm controller labels 2 pods `tier=high` and 2 pods `tier=standard`; `curl -H "user-type: premium"` routes to high-tier pods only.

### Implementation for User Story 4

- [ ] T025 [P] [US4] Create `config/samples/mesh_v1alpha1_podlabelerpolicy_high.yaml`: `imagePattern: "*/tier-app-high:*"`, `tier: high`, name `policy-high`
- [ ] T026 [P] [US4] Create `config/samples/mesh_v1alpha1_podlabelerpolicy_standard.yaml`: `imagePattern: "*/tier-app-standard:*"`, `tier: standard`, name `policy-standard`
- [ ] T027 [P] [US4] Create `config/samples/deployment.yaml`: Deployment with 4 replicas — initContainers or separate containers selecting two image tags (`localhost:5000/tier-app-high:latest` for 2 pods, `localhost:5000/tier-app-standard:latest` for 2 pods); label `app: tier-app`
- [ ] T028 [P] [US4] Create `config/samples/service.yaml`: ClusterIP Service `tier-app-svc` selecting `app: tier-app`, port 80
- [ ] T029 [US4] Create `config/samples/destinationrule.yaml`: `networking.istio.io/v1beta1` DestinationRule for host `tier-app-svc` with subsets `high-priority-pods` (labels: `tier: high`) and `standard-pods` (labels: `tier: standard`)
- [ ] T030 [US4] Create `config/samples/virtualservice.yaml`: VirtualService for `tier-app-svc` — match `headers.user-type.exact: premium` → route to `high-priority-pods` subset; default route → `standard-pods` subset

**Checkpoint**: `kubectl apply -f config/samples/` succeeds on kind cluster; after controller reconciles, `kubectl get pods -L tier` shows 2 high and 2 standard pods.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Deployment manifest, RBAC, Dockerfile, CI validation, final test run.

- [ ] T031 [P] Create `config/manager/manager.yaml` Deployment: namespace `mesh-priority-system`, `replicas: 2`, `securityContext.runAsNonRoot: true`, `securityContext.readOnlyRootFilesystem: true`, `resources.requests: {cpu: 50m, memory: 64Mi}`, `resources.limits: {cpu: 200m, memory: 128Mi}`
- [ ] T032 [P] Update `config/rbac/role.yaml` ClusterRole with exact verbs per contracts/reconciler.md: `pods: get,list,watch,patch`; `podlabelerpolicies: get,list,watch`; `podlabelerpolicies/status: update,patch`; `events: create,patch`; `leases: get,list,watch,create,update,patch,delete`
- [ ] T033 [P] Update `Dockerfile` for multi-stage build: builder stage `golang:1.22`, runtime stage `gcr.io/distroless/static:nonroot`, `USER 65532:65532`
- [ ] T034 Verify `hack/test-policy.sh` matches the CRD API group `mesh.knabben.github.com/v1alpha1` and runs cleanly against kind cluster (bash syntax check: `bash -n hack/test-policy.sh`)
- [ ] T035 Run `make test` (`go test -race ./...` with `KUBEBUILDER_ASSETS` set) and confirm reconciler package coverage ≥ 80%; fix any coverage gaps
- [ ] T036 [P] Run `golangci-lint run ./...` and resolve any lint violations in `internal/` and `cmd/`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 completion — **blocks all user stories**
- **US1 (Phase 3)**: Depends on Phase 2 — implements core reconciler; tests required first
- **US2 (Phase 4)**: Depends on Phase 3 (SSA write path must exist before testing no-lost-updates)
- **US3 (Phase 5)**: Depends on Phase 2 (manager options); independent of US1/US2 except for cmd/main.go
- **US4 (Phase 6)**: Depends on Phase 2 (CRD types must be defined); independent of US1/US2/US3 for YAML authoring
- **Polish (Phase 7)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: After Phase 2 — core path
- **US2 (P1)**: After US1 — tests verify the SSA path established in US1
- **US3 (P1)**: After Phase 2 — independent of US1 reconcile logic; only touches `cmd/main.go`
- **US4 (P2)**: After Phase 2 — YAML files only; no Go code dependency

### Within Each User Story

- Tests (T011, T012, T018, T019, T022) should be written before their implementation tasks to drive TDD
- Reconciler implementation tasks (T013–T017) must follow in order (fetch → list → match → diff → patch → event → log → metrics)
- SetupWithManager (T014) must be complete before integration tests can run against envtest

---

## Parallel Example: Phase 2 Foundational

```bash
# These can run simultaneously:
Task T007: "Implement internal/matcher/matcher.go"
Task T008: "Write unit tests in internal/matcher/matcher_test.go"
Task T009: "Bootstrap envtest Ginkgo suite in internal/controller/suite_test.go"
Task T010: "Register Prometheus metrics in internal/controller/metrics.go"

# Then sequentially:
Task T005: "Define PodLabelerPolicy types in api/v1alpha1/"
Task T006: "Run make generate manifests"
```

## Parallel Example: User Story 4 (Phase 6)

```bash
# All config/samples YAML files can be authored simultaneously:
Task T025: "config/samples/mesh_v1alpha1_podlabelerpolicy_high.yaml"
Task T026: "config/samples/mesh_v1alpha1_podlabelerpolicy_standard.yaml"
Task T027: "config/samples/deployment.yaml"
Task T028: "config/samples/service.yaml"
# Then sequentially (reference the service host):
Task T029: "config/samples/destinationrule.yaml"
Task T030: "config/samples/virtualservice.yaml"
```

---

## Implementation Strategy

### MVP (US1 Only)

1. Complete Phase 1: Setup (kubebuilder scaffold)
2. Complete Phase 2: Foundational (types, matcher, envtest, metrics)
3. Complete Phase 3: US1 (reconciler labels pods; cross-type watch; events; logs)
4. **STOP and VALIDATE**: `make test` passes; manually apply a policy and pod in kind; confirm `tier` label appears
5. Demonstrate end-to-end label flow before adding correctness guarantees

### Incremental Delivery

1. Phase 1+2 → Foundation ready
2. Phase 3 (US1) → Labels applied → MVP demo
3. Phase 4 (US2) → Required tests pass → Correctness guaranteed
4. Phase 5 (US3) → Leader election hardened → Production-ready
5. Phase 6 (US4) → Istio routing example → Full demo scenario
6. Phase 7 → Polish → Merge-ready

### Parallel Opportunities Summary

- T003, T004 (Phase 1): parallel after T001+T002
- T007, T008, T009, T010 (Phase 2): all parallel; T005→T006 must precede them
- T011, T012 (Phase 3 tests): parallel with each other
- T025, T026, T027, T028 (Phase 6): all parallel
- T031, T032, T033, T036 (Phase 7): all parallel

---

## Notes

- All `client.Update(ctx, pod)` on Pod and `client.MergeFrom` on labels are forbidden per spec §I — the Makefile `vet-no-forbidden` target (T021) enforces this
- `TestReconcile_NoLostUpdates`, `TestReconcile_CacheNotFound`, `TestReconcile_LeaderElection` are required with exact names per spec §IV
- Coverage gate: reconciler package ≥ 80% per quickstart.md
- All commits must follow Conventional Commits format per spec §VIII
- `hack/test-policy.sh` (pre-created) is verified in T034 — do not recreate it
