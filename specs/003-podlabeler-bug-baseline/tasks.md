---

description: "Task list for PodLabeler Bug Baseline (Act I)"
---

# Tasks: PodLabeler Bug Baseline (Act I)

**Input**: Design documents from `specs/003-podlabeler-bug-baseline/`
**Prerequisites**: plan.md ✅ spec.md ✅ research.md ✅ data-model.md ✅ contracts/ ✅

**Tests**: Test tasks are included — the spec explicitly requires (FR-010) an envtest suite
with three tests each written to FAIL against the Act I implementation. Test authoring IS
the primary deliverable of US2–US4.

**Organization**: Tasks are grouped by user story to enable independent implementation
and verification of each bug-exposure test.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Extract bugs/files.zip into a proper Go module at `bugs/podlabeler-act1/`.
All implementation files come verbatim from the zip — no logic changes permitted.

- [x] T001 Create directory tree: `act1/`, `act1/api/v1alpha1/`, `act1/controller/`, `act1/manifests/istio/`
- [x] T002 [P] Extract `main.go` from `bugs/files.zip` into `act1/main.go` verbatim (import path updated to `github.com/knabben/istio-poc`)
- [x] T003 [P] Extract `api/v1alpha1/types.go` from `bugs/files.zip` into `act1/api/v1alpha1/types.go` verbatim
- [x] T004 [P] Extract `api/v1alpha1/register.go` from `bugs/files.zip` into `act1/api/v1alpha1/register.go` verbatim
- [x] T005 [P] Extract `controller/reconciler.go` from `bugs/files.zip` into `act1/controller/reconciler.go` verbatim (import path updated to `github.com/knabben/istio-poc`)
- [x] T006 [P] Extract all manifests from `bugs/files.zip`: `crd.yaml`, `rbac.yaml`, `controller.yaml`, `sample-policies.yaml`, `sample-workload.yaml`, `istio/destinationrule.yaml`, `istio/virtualservice.yaml` into `act1/manifests/`
- [x] T007 Create `act1/go.mod` with module `github.com/knabben/istio-poc`, go 1.22; deps include original + testify + setup-envtest

**Checkpoint**: All source files exist at `bugs/podlabeler-act1/`. No file has been modified from the zip content.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Prove the code compiles, set up envtest infrastructure, and create the test
skeleton that all bug-exposure tests share. Must complete before any US phase.

- [x] T008 Run `go mod tidy` in `act1/` to generate `go.sum` with resolved dependencies
- [x] T009 Run `go build ./...` in `act1/` and confirm zero errors — verifies the bug baseline compiles cleanly (bugs are logic errors, not build errors)
- [x] T010 Create `act1/Makefile` with targets: `setup-envtest`, `build`, `vet`, `test-bugs`, `test-bugs-assert-failures`
- [x] T011 Run `make setup-envtest` in `act1/` — binaries at `/home/amimk/.local/share/kubebuilder-envtest/k8s/1.35.0-linux-amd64/`
- [x] T012 Create `act1/controller/reconciler_test.go` with TestMain (envtest startup, scheme, k8sClient, cfg) + 4 tests

**Checkpoint**: `go test ./controller/... -run TestMain -v` exits without panic. envtest starts and CRD loads.

---

## Phase 3: User Story 1 — Core Pod Labeling (Priority: P1) 🎯 MVP

**Goal**: Verify the happy-path labeling works under sequential, single-replica conditions.
This is the one test expected to PASS on Act I.

**Independent Test**: Run `go test ./controller/... -run TestCoreLabeling -v`; pod receives `tier=high` label.

### Implementation for User Story 1

- [x] T013 [US1] Write `TestCoreLabeling` in `act1/controller/reconciler_test.go`
- [x] T014 [US1] Verified: `TestCoreLabeling` PASSES — pod receives `tier=high` after Reconcile

**Checkpoint**: `TestCoreLabeling` passes. The controller correctly labels pods when no concurrency is involved.

---

## Phase 4: User Story 2 — Bug 1 Lost Update (Priority: P2)

**Goal**: Write and confirm the failing test that exposes Bug 1 (lost update via `r.Update()`
under concurrent reconciliation).

**Independent Test**: Run `go test ./controller/... -run TestBug1_LostUpdate -v`; test FAILS with
"expected both labels, got only one."

### Implementation for User Story 2

- [x] T015 [US2] Write `TestBug1_LostUpdate` — concurrent Updates with same resourceVersion, assert both labels present
- [x] T016 [US2] Verified: FAILS — `map[policy-b:applied]` only (policy-a lost to 409 Conflict)

**Checkpoint**: `TestBug1_LostUpdate` fails on Act I. Failure message clearly shows label loss.

---

## Phase 5: User Story 3 — Bug 2 Stale Cache (Priority: P2)

**Goal**: Write and confirm the failing test that exposes Bug 2 (NotFound from informer cache
propagated as a terminal error).

**Independent Test**: Run `go test ./controller/... -run TestBug2_StaleCache -v`; test FAILS with
"expected nil error, got: pods 'nonexistent-pod' not found."

### Implementation for User Story 3

- [x] T017 [US3] Write `TestBug2_StaleCache` — Reconcile for nonexistent pod, assert nil error
- [x] T018 [US3] Verified: FAILS — `pods "nonexistent-pod" not found` returned as terminal error

**Checkpoint**: `TestBug2_StaleCache` fails on Act I. Failure message shows NotFound propagated as error.

---

## Phase 6: User Story 4 — Bug 3 No Lease (Priority: P3)

**Goal**: Write and confirm the failing test that exposes Bug 3 (LeaderElection: false means
no Lease object is created by the manager).

**Independent Test**: Run `go test ./controller/... -run TestBug3_NoLease -v`; test FAILS with
"expected 1 Lease, got 0."

### Implementation for User Story 4

- [x] T019 [US4] Write `TestBug3_NoLease` — start manager with LeaderElection:false, list Leases, assert len==1
- [x] T020 [US4] Verified: FAILS — `"[]" should have 1 item(s), but has 0`

**Checkpoint**: `TestBug3_NoLease` fails on Act I. Failure message shows Lease list is empty.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: CI integration, Makefile completion, and end-to-end verification that exactly
three tests fail and one passes.

- [x] T021 [P] Add `test-bugs` target to `act1/Makefile`
- [x] T022 [P] Add `test-bugs-assert-failures` target to `act1/Makefile`; verified: exits 0
- [x] T023 [P] Create `act1/scripts/run-bug-tests.sh` with `--assert` mode + `.github/workflows/act1-bug-baseline.yml`
- [x] T024 Run `bash scripts/run-bug-tests.sh --assert` — PASSES: 3 FAIL, 1 PASS
- [x] T025 [P] `go vet ./...` — 0 issues confirmed
- [x] T026 Remove `bugs/` directory — done

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately. T002–T007 are all parallel after T001.
- **Foundational (Phase 2)**: Depends on Phase 1 complete. T008→T009→T010→T011→T012 are sequential (T008 needs go.mod, T009 needs go.sum, T010 is independent, T011 needs Makefile, T012 can start after T011).
- **US1 (Phase 3)**: Depends on T012 (test skeleton). T013→T014 sequential.
- **US2 (Phase 4)**: Depends on T012 (test skeleton). Can start in parallel with US1 after T012. T015→T016 sequential.
- **US3 (Phase 5)**: Depends on T012 (test skeleton). Can start in parallel with US1, US2. T017→T018 sequential.
- **US4 (Phase 6)**: Depends on T012 (test skeleton). Can start in parallel with US1, US2, US3. T019→T020 sequential.
- **Polish (Phase 7)**: Depends on US1–US4 complete. T021–T023 parallel. T024 after T021–T022. T025 independent.

### User Story Dependencies

- **US1 (P1)**: Independent after T012 — write and run happy-path test
- **US2 (P2)**: Independent after T012 — write and confirm Bug 1 test failure
- **US3 (P2)**: Independent after T012 — write and confirm Bug 2 test failure
- **US4 (P3)**: Independent after T012 — write and confirm Bug 3 test failure

### Parallel Opportunities

After T001 (directory creation):
- T002–T007 all extract different files — fully parallel

After T012 (test skeleton):
- Developer A: US1 (T013–T014) — happy-path test
- Developer B: US2 (T015–T016) — Bug 1 concurrent write test
- Developer C: US3 (T017–T018) — Bug 2 stale cache test
- Developer D: US4 (T019–T020) — Bug 3 Lease test

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 (T001–T007) → Extract all source files
2. Phase 2 (T008–T012) → Prove compilation + envtest skeleton
3. Phase 3 (T013–T014) → Write and run happy-path test
4. **STOP and VALIDATE**: `TestCoreLabeling` passes → MVP confirmed

### Incremental Bug Delivery

1. T001–T012 → Foundation
2. T013–T014 → US1 happy path passes ✓
3. T015–T016 → Bug 1 test fails ✓ (FAIL = expected)
4. T017–T018 → Bug 2 test fails ✓ (FAIL = expected)
5. T019–T020 → Bug 3 test fails ✓ (FAIL = expected)
6. T021–T025 → Polish and assert exactly 3 failures

### Parallel Team Strategy

After T012 (all foundational complete):
- Dev A: US1 happy path (T013–T014)
- Dev B: US2 Bug 1 test (T015–T016)
- Dev C: US3 Bug 2 test (T017–T018)
- Dev D: US4 Bug 3 test (T019–T020)

All four test functions go into the same file (`reconciler_test.go`); coordinate with
short-lived branches or sequential commits to avoid merge conflicts on that file.

---

## Notes

- `[P]` tasks target different files and have no incomplete task dependencies
- Phase 1 T002–T007 are fully parallel — six files from one zip
- The single test file (`reconciler_test.go`) means US1–US4 test authoring is sequential
  on that file unless team coordinates carefully
- "FAIL = expected" — the three bug tests FAILING is the correct outcome, not a problem
- Do not modify `reconciler.go` or `main.go` — the bugs must remain to make the tests fail
- T024 (`make test-bugs-assert-failures`) is the final integration gate
