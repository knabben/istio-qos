---

description: "Task list for Kind Development Environment Setup"
---

# Tasks: Kind Development Environment Setup

**Input**: Design documents from `specs/001-kind-istio-setup/`
**Prerequisites**: plan.md ✅ spec.md ✅ research.md ✅ data-model.md ✅ contracts/ ✅

**Tests**: No automated test tasks generated — this is a shell script / documentation
feature. Verification is manual (run scripts, check exit codes and output).

**Organization**: Tasks are grouped by user story to enable independent implementation
and verification of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- All tasks include exact file paths

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Repository structure initialization — must complete before any user story.

- [ ] T001 Create `rec/` directory at repository root (`mkdir rec/`)

**Checkpoint**: `rec/` directory exists → all user story phases can begin in parallel.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Note**: No additional foundational tasks required beyond Phase 1. Each user story
targets a separate file and can be implemented in parallel after T001. The test-time
dependency (cluster must exist to test Istio install) is a runtime dependency, not an
implementation dependency.

**Checkpoint**: T001 complete → US1, US2, and US3 phases can all start in parallel.

---

## Phase 3: User Story 1 — Bootstrap Kind Cluster + Registry (Priority: P1) 🎯 MVP

**Goal**: Create `rec/bootstrap.sh` — an idempotent script that creates a kind cluster
and a local container registry (port 5000) connected via containerd mirror config.

**Independent Test**: Run `bash rec/bootstrap.sh`; verify `kubectl get nodes` shows a
Ready node and `curl http://localhost:5000/v2/_catalog` returns JSON.

### Implementation for User Story 1

- [ ] T002 [US1] Create `rec/bootstrap.sh` with shebang (`#!/usr/bin/env bash`),
  `set -euo pipefail`, and top-of-file default variables:
  `CLUSTER_NAME`, `REGISTRY_PORT`, `REGISTRY_NAME` per `contracts/scripts.md`
- [ ] T003 [US1] Add prerequisite validation to `rec/bootstrap.sh`: check Docker is
  running (`docker info`), `kind` is in `$PATH`, `kubectl` is in `$PATH`; if any check
  fails, print `[bootstrap] ERROR: <message>` to stderr and exit 1
- [ ] T004 [US1] Add registry creation logic to `rec/bootstrap.sh`: check if
  `$REGISTRY_NAME` container already exists (`docker ps -a --filter name=...`); if not,
  run `docker run -d -p ${REGISTRY_PORT}:5000 --name $REGISTRY_NAME registry:2`; print
  `[bootstrap]` prefixed progress to stdout
- [ ] T005 [US1] Add kind cluster creation logic to `rec/bootstrap.sh`: check if
  `$CLUSTER_NAME` exists (`kind get clusters | grep -q`); if not, generate kind config
  YAML inline (here-doc) with `containerdConfigPatches` mirror for `localhost:$REGISTRY_PORT`
  and pipe to `kind create cluster --name $CLUSTER_NAME --config -`
- [ ] T006 [US1] Add cluster-registry networking step to `rec/bootstrap.sh`: connect
  `$REGISTRY_NAME` container to the kind Docker network so in-cluster pods can resolve
  it by name; guard with a check that the network link doesn't already exist
- [ ] T007 [US1] Add success summary to `rec/bootstrap.sh`: print cluster name, registry
  address (`localhost:$REGISTRY_PORT`), and a verification hint (`kubectl get nodes`) to
  stdout; make script executable (`chmod +x rec/bootstrap.sh`)

**Checkpoint**: `rec/bootstrap.sh` exists, is executable, passes idempotency check
(second run exits 0 with no changes), and creates a healthy cluster + registry.

---

## Phase 4: User Story 2 — Install Istio Service Mesh (Priority: P2)

**Goal**: Create `rec/install-istio.sh` — an idempotent script that installs the full
Istio service mesh (`demo` profile) into an existing kind cluster.

**Independent Test**: With the US1 cluster running, execute `bash rec/install-istio.sh`;
verify `kubectl get pods -n istio-system` shows all pods Running/Ready.

### Implementation for User Story 2

- [ ] T008 [P] [US2] Create `rec/install-istio.sh` with shebang (`#!/usr/bin/env bash`),
  `set -euo pipefail`, and top-of-file default variables: `CLUSTER_NAME`,
  `ISTIO_VERSION` (pinned value), `ISTIO_PROFILE=demo` per `contracts/scripts.md`
- [ ] T009 [US2] Add prerequisite validation to `rec/install-istio.sh`: check `kubectl`
  is in `$PATH` and can reach the cluster (`kubectl cluster-info`), check `istioctl` is
  in `$PATH`; if `istioctl` missing, print `[install-istio] ERROR:` plus download URL
  to stderr and exit 1 (do not auto-download)
- [ ] T010 [US2] Add idempotency check to `rec/install-istio.sh`: if `istio-system`
  namespace already exists (`kubectl get namespace istio-system 2>/dev/null`), print
  `[install-istio] Istio already installed — skipping.` to stdout and exit 0
- [ ] T011 [US2] Add Istio installation step to `rec/install-istio.sh`: run
  `istioctl install --set profile=$ISTIO_PROFILE --skip-confirmation`; exit 2 with an
  `[install-istio] ERROR:` message to stderr on failure
- [ ] T012 [US2] Add readiness wait to `rec/install-istio.sh`: poll
  `kubectl get pods -n istio-system` until all pods show `Running` or `Completed`, with
  a 300-second timeout; exit 3 with `[install-istio] ERROR: timeout` to stderr on
  timeout; print success summary and sidecar-injection hint to stdout on success; make
  script executable (`chmod +x rec/install-istio.sh`)

**Checkpoint**: `rec/install-istio.sh` exists, is executable, installs Istio into a kind
cluster, and is idempotent (second run exits 0 immediately).

---

## Phase 5: User Story 3 — README Documentation (Priority: P3)

**Goal**: Create `README.md` at the repository root with the complete step-by-step guide
that allows a new developer to set up the full local environment without external help.

**Independent Test**: Ask a developer unfamiliar with the project to follow only `README.md`
and reach a working kind cluster with Istio in under 15 minutes.

### Implementation for User Story 3

- [ ] T013 [P] [US3] Create `README.md` at repository root with: project title and one-
  paragraph overview of `mesh-priority-controller`, a **Prerequisites** section with a
  table listing Docker, kind, kubectl, istioctl — each with version notes and install links
- [ ] T014 [US3] Add **Setup sequence** section to `README.md` with numbered steps:
  (1) clone repo, (2) run `bash rec/bootstrap.sh`, (3) run `bash rec/install-istio.sh`;
  include the exact commands and expected output excerpts for each step
- [ ] T015 [US3] Add **Verification** section to `README.md` with the exact commands to
  confirm each step succeeded: `kubectl get nodes`, `curl http://localhost:5000/v2/_catalog`,
  `kubectl get pods -n istio-system`
- [ ] T016 [US3] Add **Teardown**, **Customization** (env var override table from
  `contracts/scripts.md`), and **Troubleshooting** sections to `README.md` covering
  port conflicts, Docker not running, and Istio pods stuck in Pending

**Checkpoint**: `README.md` exists at repo root with all sections; a reader can complete
the full setup by following it alone.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Teardown script, final executable permissions check, and end-to-end
integration verification.

- [ ] T017 [P] Create `rec/teardown.sh` with shebang, `set -euo pipefail`, CLUSTER_NAME
  and REGISTRY_NAME defaults; delete kind cluster (`kind delete cluster --name $CLUSTER_NAME`
  — no-op if not found); stop and remove registry container (`docker rm -f $REGISTRY_NAME`
  — no-op if not found); print summary to stdout; make executable (`chmod +x rec/teardown.sh`)
- [ ] T018 Verify all scripts in `rec/` are executable (`ls -la rec/`); confirm shebang
  line is present in each file; add `rec/` to `.gitattributes` with `text eol=lf` to
  prevent line-ending corruption on Windows checkouts
- [ ] T019 [P] Manual end-to-end integration run: execute `bash rec/bootstrap.sh` →
  `bash rec/install-istio.sh` → verify verification commands → `bash rec/teardown.sh`;
  confirm all scripts exit 0 and outputs match the documentation in `README.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: None defined — all US phases unblock after Phase 1
- **US1 (Phase 3)**: Depends on Phase 1 (T001). Tasks T002→T003→T004→T005→T006→T007 are sequential (same file)
- **US2 (Phase 4)**: Depends on Phase 1 only — can start in parallel with US1. Tasks T008→T009→T010→T011→T012 are sequential (same file)
- **US3 (Phase 5)**: Depends on Phase 1 only — can start in parallel with US1 and US2. T013→T014→T015→T016 are sequential (same file)
- **Polish (Phase 6)**: T017 can start after Phase 1. T018 depends on T007+T012+T017. T019 depends on T018

### User Story Dependencies

- **US1 (P1)**: No runtime dependency on other stories — implement independently
- **US2 (P2)**: Script can be written independently; **testing requires a running US1 cluster**
- **US3 (P3)**: Can be written fully independently — documentation is self-contained

### Parallel Opportunities

After T001:
- Developer A can start US1 (T002→T007)
- Developer B can start US2 (T008→T012) in parallel
- Developer C can start US3 (T013→T016) in parallel
- T017 (teardown script) can be written at any time in parallel

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. T001 — Create `rec/` directory
2. T002–T007 — Implement `rec/bootstrap.sh`
3. **STOP and VALIDATE**: `bash rec/bootstrap.sh` → `kubectl get nodes` → re-run → exit 0

### Incremental Delivery

1. T001 → T002–T007 → Validate US1 (cluster + registry works)
2. T008–T012 → Validate US2 (Istio installs into the cluster)
3. T013–T016 → Validate US3 (README guides a new developer end-to-end)
4. T017–T019 → Polish (teardown, permissions, full integration run)

### Parallel Team Strategy

With three developers after T001:
- Dev A: US1 scripts (T002–T007)
- Dev B: US2 scripts (T008–T012) + T017 teardown
- Dev C: US3 README (T013–T016)

---

## Notes

- `[P]` tasks operate on different files and have no incomplete task dependencies
- `[US]` label maps each task to its spec user story for traceability
- T008, T013, T017 are all marked `[P]` — they can start as soon as T001 completes
- US2 testing requires a running cluster from US1 — coordinate integration testing
- Commit after each phase checkpoint
- Run `bash rec/bootstrap.sh` twice before marking T007 done — idempotency is required
