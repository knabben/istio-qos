---

description: "Task list for Live Kubernetes Cluster Debugging via kmcp MCP (004)"
---

# Tasks: Live Kubernetes Cluster Debugging via kmcp MCP

**Input**: Design documents from `specs/004-kmcp-k8s-debug/`
**Prerequisites**: plan.md ✅ spec.md ✅ research.md ✅ data-model.md ✅ quickstart.md ✅

**Deliverables**:
1. `kmcp-server/` — Python FastMCP server with 6 read-only Kubernetes tools + RBAC manifests
2. `act2/` — Fixed controller (all 3 bugs corrected) with envtest suite: 4 tests PASS
3. `.claude/skills/debug-bug*.md` — Per-bug skills: detect (MCP) → fix (code) → deploy → verify (test)

**Demo cycle per bug**:
```
go test act1/ → FAIL  →  MCP tools observe cluster  →  /debug-bugN  →  go test act2/ → PASS
```

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create all directory trees before any file-level work begins.

- [ ] T001 Create `kmcp-server/` directory tree: `kmcp-server/`, `kmcp-server/rbac/`
- [ ] T002 [P] Create `act2/` directory tree: `act2/`, `act2/api/v1alpha1/`, `act2/controller/`, `act2/manifests/istio/`

**Checkpoint**: Both `kmcp-server/` and `act2/` directories exist at repo root.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Module setup, shared file copies, and MCP config — required before any user story can be implemented.

- [ ] T003 [P] Create `kmcp-server/requirements.txt` with `fastmcp>=2.0.0`, `kubernetes>=31.0.0`, `pytest>=8.0.0`, `pytest-mock>=3.12.0`
- [ ] T004 Create `act2/go.mod` with module `github.com/knabben/istio-poc`, `go 1.22`, and the same dependency set as `act1/go.mod` (controller-runtime, k8s.io/api, k8s.io/apimachinery, k8s.io/client-go, testify, setup-envtest)
- [ ] T005 [P] Copy `act1/api/v1alpha1/types.go` → `act2/api/v1alpha1/types.go` verbatim (no changes)
- [ ] T006 [P] Copy `act1/api/v1alpha1/register.go` → `act2/api/v1alpha1/register.go` verbatim (no changes)
- [ ] T007 [P] Copy all `act1/manifests/` files → `act2/manifests/` verbatim: `crd.yaml`, `rbac.yaml`, `sample-policies.yaml`, `sample-workload.yaml`, `istio/destinationrule.yaml`, `istio/virtualservice.yaml` (act2-specific controller.yaml is written later in US3)
- [ ] T008 Run `go mod tidy` in `act2/` to generate `act2/go.sum`
- [ ] T009 Run `go build ./...` in `act2/` (api/ only at this stage) — verifies module and type registration compile cleanly
- [ ] T010 Add MCP server registration to `.claude/settings.json` under `mcpServers.podlabeler-debug` (command: `python`, args: `["{repo_root}/kmcp-server/server.py"]`, type: `stdio`)

**Checkpoint**: `act2/` compiles; `.claude/settings.json` has the MCP entry; `kmcp-server/requirements.txt` exists.

---

## Phase 3: User Story 1 — Configure kmcp MCP Server with Cluster Access (Priority: P1) 🎯 MVP

**Goal**: A working FastMCP server that gives Claude Code read-only visibility into the kind cluster via 6 Kubernetes tools.

**Independent Test**: Start the MCP server (`python kmcp-server/server.py`), ask Claude Code to "list all pods in the default namespace" — Claude calls `list_pods()` and returns live cluster data.

### Implementation for User Story 1

- [ ] T011 [US1] [P] Create `kmcp-server/rbac/role.yaml` — `ClusterRole` named `podlabeler-debug-reader` with `get/list/watch` on: `pods`, `events` (core), `pods/log` (get only), `leases` (coordination.k8s.io), `podlabelerpolicies` (labeling.knabben.dev) — no write verbs
- [ ] T012 [US1] [P] Create `kmcp-server/rbac/rolebinding.yaml` — `ClusterRoleBinding` binding `podlabeler-debug-reader` to the current user (`kubectl config current-context` user)
- [ ] T013 [US1] Create `kmcp-server/server.py` — FastMCP server named `podlabeler-debug` with all 6 tool functions: `list_pods(namespace)`, `get_pod(name, namespace)`, `list_pod_logs(pod_name, namespace, container, tail_lines)`, `list_events(namespace, field_selector)`, `list_leases(namespace)`, `list_podlabelerpolicies(namespace)` — each uses the `kubernetes` Python client to read cluster state and returns JSON-serializable output
- [ ] T014 [US1] Create `kmcp-server/Makefile` with targets: `setup` (pip install -r requirements.txt), `install-rbac` (kubectl apply role + rolebinding), `start` (python server.py), `stop` (pkill -f server.py), `test` (pytest tests/ -v)
- [ ] T015 [US1] [P] Create `kmcp-server/tests/__init__.py` (empty) to mark the tests package
- [ ] T016 [US1] Create `kmcp-server/tests/test_server.py` — one pytest function per tool using `pytest-mock` to patch `kubernetes.client.CoreV1Api` / `CoordinationV1Api` / custom-objects API: (a) `test_list_pods` — mock returns a PodList with 2 pods, assert tool returns JSON with correct pod names and labels; (b) `test_get_pod` — mock returns a single Pod, assert labels and phase are present; (c) `test_list_pod_logs` — mock returns log string, assert lines are returned; (d) `test_list_events` — mock returns EventList, assert event message/reason fields present; (e) `test_list_leases` — mock returns empty LeaseList, assert output indicates zero leases; (f) `test_list_podlabelerpolicies` — mock returns empty custom-object list, assert graceful empty response; each test also asserts the tool raises no exception when the mock client is provided
- [ ] T017 [US1] Run `pytest tests/ -v` in `kmcp-server/` — all 6 tool tests pass

**Checkpoint**: `make setup install-rbac start` in `kmcp-server/`; `make test` exits 0 with 6 PASSED; Claude Code can call `list_pods` and receive pod data.

---

## Phase 4: User Story 2 — Start Cluster and Deploy Broken Controller (Priority: P2)

**Goal**: The Act I buggy controller is running in the kind cluster with the sample policies and workload pods, making all three bugs observable via the MCP tools.

**Independent Test**: After this phase, `kubectl get pods -n default` shows controller pods running; `list_pod_logs` in Claude Code returns controller log lines mentioning reconciliation.

### Implementation for User Story 2

- [ ] T018 [US2] Create `act1/Dockerfile` — multi-stage build: `FROM golang:1.22 AS builder` (go build -o /podlabeler .) + `FROM gcr.io/distroless/static:nonroot AS runner` (COPY + ENTRYPOINT); image tag target: `podlabeler:act1`
- [ ] T019 [US2] Add `docker-build`, `kind-load`, `cluster-deploy`, `cluster-teardown` targets to `act1/Makefile` — `docker-build` builds `podlabeler:act1`; `kind-load` runs `kind load docker-image podlabeler:act1`; `cluster-deploy` applies `manifests/crd.yaml manifests/rbac.yaml manifests/controller.yaml manifests/sample-policies.yaml manifests/sample-workload.yaml`; `cluster-teardown` deletes those resources
- [ ] T020 [US2] Verify `make docker-build kind-load cluster-deploy` in `act1/` — confirm controller pods reach Running state and policies are listed by `kubectl get podlabelerpolicies`

**Checkpoint**: MCP tool `list_pod_logs(pod_name="<controller-pod>")` returns reconciliation log lines; `list_leases()` returns empty list (Bug 3 observable).

---

## Phase 5: User Story 3 — Diagnose and Fix All Three Bugs via Skills (Priority: P3)

**Goal**: For each of the three bugs: a Claude Code skill that (1) runs the failing test to confirm the bug, (2) uses MCP tools to observe live cluster evidence, (3) points to the exact defect location, (4) applies the fix to `act2/`, (5) builds and deploys the fixed image, and (6) re-runs the test confirming it PASSES.

**Independent Test**: Running `/debug-podlabeler-all` in Claude Code produces a final report: "TestBug1_LostUpdate PASS, TestBug2_StaleCache PASS, TestBug3_NoLease PASS — all 4 envtest tests pass in act2/".

### Act II Fixed Controller (act2/ source files)

- [ ] T021 [US3] Write `act2/controller/reconciler.go` — copy `act1/controller/reconciler.go` as base, then apply **Bug 2 fix** at line ~61: replace `return ctrl.Result{}, err` (after r.Get NotFound) with `if apierrors.IsNotFound(err) { return ctrl.Result{}, nil }` — add `k8s.io/apimachinery/pkg/api/errors` import
- [ ] T022 [US3] Apply **Bug 1 fix** to `act2/controller/reconciler.go` at line ~121: replace the `r.Update(ctx, pod)` block with a server-side apply `r.Patch(ctx, patch, client.Apply, client.ForceOwnership, client.FieldOwner("podlabeler"))` using a minimal SSA object that carries only the `desired` labels map — add `metav1` import for `TypeMeta`
- [ ] T023 [US3] Write `act2/main.go` — copy `act1/main.go` as base, then apply **Bug 3 fix**: replace `LeaderElection: false` with `LeaderElection: true, LeaderElectionID: "podlabeler.knabben.dev", LeaderElectionNamespace: "kube-system", LeaderElectionReleaseOnCancel: true` — update the log message to print `true`
- [ ] T024 [US3] Write `act2/manifests/controller.yaml` — copy `act1/manifests/controller.yaml`, change image to `podlabeler:act2`, set `replicas: 1`, add leader-election env vars if needed; `act2/manifests/` already has crd.yaml, rbac.yaml, sample-*.yaml from T007
- [ ] T025 [US3] Copy `act1/controller/reconciler_test.go` → `act2/controller/reconciler_test.go` verbatim (identical test file — all 4 tests should now PASS against the fixed reconciler)
- [ ] T026 [US3] Run `go mod tidy` and `go build ./...` in `act2/` — confirm zero errors with the fixed imports (`apierrors`, `metav1`)
- [ ] T027 [US3] Create `act2/Dockerfile` — identical structure to `act1/Dockerfile`, image tag target `podlabeler:act2`
- [ ] T028 [US3] Create `act2/Makefile` — same targets as `act1/Makefile` (setup-envtest, build, vet, test, docker-build, kind-load, cluster-deploy, cluster-teardown) with act2-specific image tag and manifests
- [ ] T029 [US3] Run `go test ./controller/... -v` in `act2/` with `KUBEBUILDER_ASSETS` set — confirm all 4 tests PASS (TestCoreLabeling, TestBug1_LostUpdate, TestBug2_StaleCache, TestBug3_NoLease)

### Claude Code Skills (the interactive debug+fix+verify workflow)

- [ ] T030 [US3] Write `.claude/skills/debug-bug1.md` — skill that: (1) runs `go test -run TestBug1_LostUpdate` in act1/ and shows FAIL output; (2) calls MCP `list_pods` and `list_events` to surface 409 Conflict and label loss evidence; (3) points to `act1/controller/reconciler.go:121` — `r.Update(ctx, pod)`; (4) shows the exact SSA patch already applied in `act2/controller/reconciler.go`; (5) runs `make docker-build kind-load cluster-deploy` in `act2/`; (6) runs `go test -run TestBug1_LostUpdate` in act2/ → PASS
- [ ] T031 [US3] Write `.claude/skills/debug-bug2.md` — skill that: (1) runs `go test -run TestBug2_StaleCache` in act1/ and shows FAIL output; (2) calls MCP `list_pod_logs` to surface `pods "X" not found` controller log lines; (3) points to `act1/controller/reconciler.go:61` — `return ctrl.Result{}, err`; (4) shows the exact `apierrors.IsNotFound` guard already applied in `act2/controller/reconciler.go`; (5) runs `make docker-build kind-load cluster-deploy` in `act2/`; (6) runs `go test -run TestBug2_StaleCache` in act2/ → PASS
- [ ] T032 [US3] Write `.claude/skills/debug-bug3.md` — skill that: (1) runs `go test -run TestBug3_NoLease` in act1/ and shows FAIL output; (2) calls MCP `list_leases` — empty list; (3) points to `act1/main.go:69` — `LeaderElection: false`; (4) shows the `LeaderElection: true` block already in `act2/main.go`; (5) runs `make docker-build kind-load cluster-deploy` in `act2/`; (6) runs `go test -run TestBug3_NoLease` in act2/ → PASS
- [ ] T033 [US3] Write `.claude/skills/debug-podlabeler-all.md` — combined skill that runs the detect→fix→verify cycle for all three bugs in sequence (Bug 2 first as simplest, then Bug 3, then Bug 1), ending with `go test ./controller/... -v` in `act2/` showing all 4 PASS

**Checkpoint**: `/debug-podlabeler-all` runs to completion; `go test ./controller/... -v` in `act2/` exits with 4 PASS, 0 FAIL.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Quality gates, CI integration, documentation, and final validation.

- [ ] T034 [P] Run `go vet ./...` in `act2/` — 0 issues
- [ ] T035 [P] Run `make test` in `act2/` one final time with fresh `KUBEBUILDER_ASSETS` — confirm 4 PASS
- [ ] T036 [P] Verify `python kmcp-server/server.py` starts without import errors and the 6 tools are listed; run `make test` in `kmcp-server/` — confirm 6 pytest PASSED
- [ ] T037 [P] Add `act2/README.md` documenting the three bug fixes with before/after code blocks and the `make docker-build kind-load cluster-deploy test` workflow
- [ ] T038 Create `.github/workflows/kmcp-server-tests.yml` — trigger on push/PR touching `kmcp-server/**` or the workflow file itself; steps: checkout → `actions/setup-python@v5` with python-version `"3.11"` → `pip install -r kmcp-server/requirements.txt` → `pytest kmcp-server/tests/ -v --tb=short` → fail CI if exit code ≠ 0; workflow must NOT require a live Kubernetes cluster (all tests use mocks)
- [ ] T039 Perform quickstart.md walkthrough end-to-end: MCP setup → cluster deploy → `/debug-bug1` → `/debug-bug2` → `/debug-bug3` → all tests PASS

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — T001 and T002 are parallel (different directories).
- **Foundational (Phase 2)**: Depends on Phase 1. T003–T007 are all parallel after T001/T002. T008 (go mod tidy) needs T004 (go.mod) + T005/T006 (api/ files). T009 (go build) needs T008. T010 (settings.json) is independent.
- **US1 (Phase 3)**: Depends on T003 (requirements.txt) and T010 (settings.json). T011/T012 parallel; T013 needs T011/T012; T014 independent of T013. T015 parallel with T014 (different file). T016 needs T013 (imports server tools) + T015 (package exists). T017 needs T016.
- **US2 (Phase 4)**: Depends on Phase 1 (act1/ exists). T018/T019 parallel; T020 needs both T018 and T019.
- **US3 (Phase 5)**: Depends on Phase 2 complete (act2/ scaffold). T021→T022 sequential (same file). T023 parallel with T021/T022 (different file). T024 parallel (different file). T025 parallel (different file). T026 needs T021/T022/T023. T027/T028 parallel. T029 needs T026 + T027 + T028. T030/T031/T032 need T029 complete (verified fixes). T033 needs T030/T031/T032.
- **Polish (Phase 6)**: Depends on US3 complete. T034/T035/T036/T037 fully parallel. T038 parallel (independent of act2/ code). T039 after all five.

### User Story Dependencies

- **US1 (P1)**: Independent after Phase 2 — MCP server can be built and tested without act2/ fixed code.
- **US2 (P2)**: Independent after Phase 1 — deploying act1 to cluster has no dependency on act2/ or MCP server.
- **US3 (P3)**: Depends on Phase 2 (act2/ scaffold) and benefits from US1 (MCP tools for cluster observation), but the act2/ code fixes and tests can be written and verified independently of the live cluster.

### Parallel Opportunities

After T001/T002 (directory creation):
- T003–T007 all write different files — fully parallel

After T009 (act2 compiles):
- T011/T012 (RBAC) — parallel
- T015 (tests/__init__.py) — parallel
- T018/T019 (Dockerfile, Makefile for act1) — parallel
- T021/T023/T024/T025 (act2 source files, different files) — parallel

After T016 (test_server.py):
- T017 (pytest) — must follow T016

After T029 (act2 tests PASS):
- T030/T031/T032 (one skill per bug) — fully parallel

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 (T001–T002) → Create directories
2. Phase 2 (T003–T010) → Module setup, file copies, MCP config
3. Phase 3 (T011–T014) → MCP server with 6 tools + RBAC
4. **STOP and VALIDATE**: Claude Code can query live cluster via `list_pods` — MCP works
5. Continue to US2/US3 once MCP is verified

### Incremental Delivery

1. T001–T017 → US1: MCP server working + Python tests pass ✓ (demo: Claude inspects cluster)
2. T018–T020 → US2: Broken controller deployed ✓ (demo: bugs visible in cluster)
3. T021–T033 → US3: Fixed controller + skills ✓ (demo: full detect→fix→verify cycle)
4. T034–T039 → Polish + CI ✓

### Parallel Team Strategy

After Phase 2 complete:
- Dev A: US1 (T011–T017) — Python MCP server + unit tests
- Dev B: US2 (T018–T020) — Docker + cluster deployment
- Dev C: US3 act2 code (T021–T029) — Fixed Go controller
- Dev D: US3 skills (T030–T033) — after T029 confirms fixes work

---

## Notes

- `[P]` tasks target different files and have no incomplete task dependencies
- T021→T022 are sequential: both modify `act2/controller/reconciler.go`; apply Bug 2 first (simpler), then Bug 1
- T030–T033 (skills) are written AFTER T029 (tests PASS) so skill content can reference verified PASS output
- T016 (test_server.py) uses `pytest-mock` to patch the kubernetes client — NO live cluster required; the GitHub Actions workflow (T038) runs on ubuntu-latest with no cluster setup
- T038 workflow triggers on `kmcp-server/**` path filter; it is separate from the existing `act1-bug-baseline.yml`
- The skills describe the DETECTION path (MCP tools) + point to already-applied fixes in act2/; they are instructions for Claude, not code
- `act2/` starts as a copy of act1 and is progressively fixed: api/ is verbatim, reconciler.go gets Bug 1+2 fixes, main.go gets Bug 3 fix
- act1/ source is never modified — it remains the bug baseline
