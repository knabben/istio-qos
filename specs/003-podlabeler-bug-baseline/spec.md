# Feature Specification: PodLabeler Bug Baseline (Act I)

**Feature Branch**: `003-podlabeler-bug-baseline`
**Created**: 2026-04-26
**Status**: Draft
**Input**: Extracted from bugs/files.zip — the Act I broken controller baseline

## Overview

This specification describes the **Act I baseline**: a hand-written Kubernetes controller
(no scaffolding, no code generation) that watches Pods cluster-wide and applies labels
derived from `PodLabelerPolicy` custom resources. The controller is **intentionally
implemented with three distributed-systems bugs**. Its primary purpose is to serve as a
teaching demonstration — the buggy code plus an envtest suite that exposes each failure.

The controller itself is usable: it labels pods correctly under ideal (sequential,
single-replica) conditions. Under real operating conditions the three bugs surface and
cause silent data loss, dropped reconciliation requests, and mesh configuration storms.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Core Pod Labeling (Priority: P1)

An operator deploys `PodLabelerPolicy` resources that map container image patterns to
labels. When a Pod starts whose primary container image matches a policy's `imagePattern`,
the controller applies the policy's labels to that pod. Istio's `DestinationRule` uses
the `tier` label to route premium traffic to the high-tier pod subset and standard
traffic to the standard-tier subset.

**Why this priority**: Without the core labeling loop the controller has no value.
All three bugs live inside or around this loop, so it is the anchor for all bug exposure.

**Independent Test**: Deploy one `PodLabelerPolicy` with `imagePattern: app1:*` and
`labels: {tier: high}`. Create a pod with image `registry.io/team/app1:v1`. After
reconciliation, the pod must carry `tier=high`.

**Acceptance Scenarios**:

1. **Given** a `PodLabelerPolicy` with `imagePattern: app1:*` and `labels: {tier: high}`,
   **When** a pod whose primary container image is `registry.io/team/app1:v2` is created,
   **Then** the pod's labels include `tier=high` after the reconciler runs.

2. **Given** a `PodLabelerPolicy` with `imagePattern: app2:latest`,
   **When** a pod whose primary container image is `registry.io/team/app1:v2` is created,
   **Then** the pod's labels are unchanged (no match, no write).

3. **Given** two policies matching different image patterns,
   **When** a pod matches only one policy,
   **Then** only that policy's labels are applied — no labels from the non-matching policy.

4. **Given** a pod with zero containers,
   **When** the reconciler runs,
   **Then** no label write is attempted and the reconciler returns without error.

---

### User Story 2 — Bug 1: Lost Update Under Concurrent Writes (Priority: P2)

The reconciler applies labels using a full object replacement (`Update`) that embeds the
pod's `resourceVersion` from the informer cache. When two goroutines (or two replicas)
reconcile the same pod concurrently, both read the same `resourceVersion`, both issue
their writes, and one write silently clobbers the other's changes. A racing kubelet
status update produces the same effect: a 409 Conflict that the reconciler does not
retry, so the label is simply not written.

**Why this priority**: Label loss is invisible at the API level — the write returns 200
for the winner and 409 (unhandled) for the loser. Premium traffic silently routes to the
wrong pod subset. This is the most damaging bug in a multi-replica deployment.

**Independent Test**: The envtest suite sends two concurrent reconcile requests for the
same pod, each carrying a distinct label to write. After both complete, the pod must
carry both labels. **This test FAILS on the Act I baseline**: only one label survives.

**Acceptance Scenarios**:

1. **Given** a pod that matches two policies (different labels),
   **When** two reconcile calls execute concurrently against that pod,
   **Then** after both complete, the pod carries all expected labels from both policies.
   *(Fails on Act I — one write clobbers the other.)*

2. **Given** a reconcile write that races a kubelet status update for the same pod,
   **When** the API server returns a 409 Conflict,
   **Then** the reconciler retries and the label is eventually applied.
   *(Fails on Act I — the 409 is propagated, the label is dropped.)*

---

### User Story 3 — Bug 2: Stale Cache Read Treated as Terminal Error (Priority: P2)

The informer cache is eventually consistent. When a Pod Create event fires and the
reconciler fetches the pod, the cache may not yet reflect the new object. The `Get`
call returns a `NotFound` error. The Act I baseline propagates this error as terminal:
the workqueue applies exponential backoff and the request is eventually dropped. The pod
is never labeled.

**Why this priority**: Every newly created pod that matches a policy goes unlabeled until
the next reconcile event (policy change or manual trigger). In a busy cluster this means
new pods briefly join the wrong Istio subset, creating transient routing errors.

**Independent Test**: The envtest suite triggers a reconcile for a pod name that is not
yet visible in the cache. The reconciler must return a nil error (the miss is transient
and benign). **This test FAILS on the Act I baseline**: the reconciler returns the
`NotFound` error, causing backoff and eventual request drop.

**Acceptance Scenarios**:

1. **Given** a Pod Create event has fired,
   **When** the reconciler runs before the cache has synced the new pod,
   **Then** the reconciler returns without error and without logging a misleading failure.
   *(Fails on Act I — returns the NotFound error.)*

2. **Given** a pod that has been deleted before the reconciler runs,
   **When** the reconciler receives its stale queue entry,
   **Then** the reconciler returns without error (pod no longer exists — nothing to do).
   *(Fails on Act I — same code path, same error propagation.)*

---

### User Story 4 — Bug 3: No Leader Election with Multiple Replicas (Priority: P3)

The manager is deployed with `replicas: 2` and `LeaderElection: false`. Both replicas
reconcile every event independently. Combined with Bug 1 this produces continuous label
oscillation: each replica writes its version of the labels, the other overwrites it, and
the cycle repeats. Every label write triggers an Istio xDS push to all proxies, producing
a config-push storm that degrades the entire mesh under load.

**Why this priority**: The Lease absence is observable (no Lease object in the cluster).
The oscillation and config-push storm are observable via Kiali and Prometheus. This bug
requires a running cluster to demonstrate fully; the envtest test is narrower: verify a
Lease is created when the manager starts.

**Independent Test**: Start the manager and check the cluster for a `coordination.k8s.io/Lease`
object in the controller's namespace. **This test FAILS on the Act I baseline**: no Lease
is created because `LeaderElection: false`.

**Acceptance Scenarios**:

1. **Given** the manager starts with its default configuration,
   **When** the test checks for a `Lease` object in the controller namespace,
   **Then** no Lease is found.
   *(This is the expected FAILURE on Act I — proves the bug.)*

2. **Given** two manager replicas running simultaneously with `LeaderElection: false`,
   **When** both receive a reconcile event for the same pod,
   **Then** both write independently — demonstrating split-brain behavior.

---

### Edge Cases

- A pod with no containers must not trigger a label write.
- A pod already carrying the correct labels must not trigger an unnecessary write
  (idempotency check).
- A `PodLabelerPolicy` deleted while a reconcile is in progress must not cause a panic.
- A pod being deleted (`DeletionTimestamp` set) must be skipped — no label write.
- Multiple policies matching the same pod must all contribute their labels (merge, not
  replace).
- An `imagePattern` with a wildcard tag (`app:*`) must match any tag of that image.
- An `imagePattern` without a tag must match as a prefix (e.g., `app` matches `app:v1`
  and `app-extra:v1`).

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The controller MUST watch all Pods cluster-wide and reconcile each pod
  against all `PodLabelerPolicy` resources whenever either changes.
- **FR-002**: The controller MUST match a pod's primary container image (index 0) against
  each policy's `imagePattern`, stripping any registry prefix before matching.
- **FR-003**: The controller MUST apply all labels from all matching policies to the pod
  in a single write operation per reconcile cycle.
- **FR-004**: The controller MUST skip pods that are being deleted (`DeletionTimestamp` set).
- **FR-005**: The controller MUST skip pods with no containers.
- **FR-006**: The controller MUST use a full object replacement write (`Update`) — this is
  the intentional Bug 1 implementation that the test suite must expose.
- **FR-007**: The controller MUST propagate `NotFound` errors from cache reads as terminal
  errors — this is the intentional Bug 2 implementation that the test suite must expose.
- **FR-008**: The manager MUST start with `LeaderElection: false` and `replicas: 2` in
  the Deployment — this is the intentional Bug 3 configuration that the test suite must expose.
- **FR-009**: The CRD types, DeepCopy methods, and scheme registration MUST be written by
  hand (no code generation), mirroring the bugs/files.zip layout exactly.
- **FR-010**: The envtest test suite MUST include exactly three tests, one per bug, each
  written to FAIL against this implementation and PASS against the corrected Act II version.

### Key Entities

- **PodLabelerPolicy**: A cluster-scoped custom resource with `imagePattern` (string) and
  `labels` (map of string to string). Defines which labels to apply to pods whose primary
  container image matches the pattern.
- **Pod**: The standard Kubernetes workload unit. The controller reads pods from the
  informer cache and writes labels back via the API server.
- **Reconciler**: The controller loop that processes one pod per invocation. Contains
  Bug 1 (Update write) and Bug 2 (NotFound propagation).
- **Manager**: The controller-runtime manager that hosts the reconciler. Contains Bug 3
  (LeaderElection disabled).
- **Lease**: The Kubernetes coordination object that leader election creates. Absent in
  the Act I baseline; its absence is the observable symptom of Bug 3.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All three bug-exposure tests fail when run against the Act I baseline,
  confirming the bugs exist as documented.
- **SC-002**: The core labeling test (US1) passes: a pod matching a policy receives the
  correct labels within one reconcile cycle under sequential, single-replica conditions.
- **SC-003**: Each bug is exposed by exactly one isolated test — no test covers more than
  one bug, keeping the failure signal unambiguous.
- **SC-004**: The test suite runs to completion without panics, deadlocks, or
  infrastructure errors — only the three expected assertion failures occur.
- **SC-005**: The implementation compiles and passes `go vet` without errors, confirming
  the bugs are logic errors, not syntax or type errors.

---

## Assumptions

- The target audience is engineers attending a talk on spec-driven controller development.
  The "user" of this feature is the presenter demonstrating what happens when a controller
  is written without constitutional constraints.
- The implementation is a faithful reproduction of bugs/files.zip — no changes to the
  buggy logic are permitted. The spec describes what the code does, not what it should do.
- envtest (controller-runtime's integration test framework) is used for the test suite.
  No live cluster is required to run the three bug-exposure tests.
- The three bug tests use `//go:build` tags or a separate package so they can be run
  independently from any correct-code tests added in Act II.
- `imagePattern` matching follows the three-form convention from bugs/files.zip:
  exact match (`app:v1`), wildcard tag (`app:*`), prefix match (`app`). Registry prefixes
  are stripped before matching.
- The Istio `DestinationRule` and `VirtualService` manifests from bugs/files.zip are
  included as reference fixtures but are not exercised by the envtest suite (no Istio
  control plane in envtest).
- The Go module path mirrors the bugs/files.zip module: `github.com/knabben/istio-poc`.
