# Feature Specification: Pod Tier Label Controller

**Feature Branch**: `002-pod-label-controller`
**Created**: 2026-04-26
**Status**: Draft
**Input**: User description: "using kubebuilder create the first implementation of the
controller to manage labels on pods — no lost updates (server-side apply), cache-safe
reads, leader election required; example scenario with DestinationRule + VirtualService
routing premium traffic by tier label."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Controller Applies Tier Labels to Matching Pods (Priority: P1)

A platform engineer deploys a `PodLabelerPolicy` custom resource that specifies one or
more container image patterns and a target tier (`high` or `standard`). The controller
watches all pods in the cluster and, for each pod whose container images match a policy,
applies the label `tier: <value>` to that pod. Pods that match no policy receive no tier
label (or have an existing one removed if the policy is deleted).

**Why this priority**: This is the core function of the controller. Without it, no
downstream Istio routing by tier is possible.

**Independent Test**: Deploy a `PodLabelerPolicy` matching image `nginx:*` with
`tier: standard`; create a pod running `nginx:latest`; verify the pod gains the label
`tier=standard` within one reconcile cycle. Delete the policy; verify the label is
removed from the pod.

**Acceptance Scenarios**:

1. **Given** a `PodLabelerPolicy` exists with `imagePattern: nginx:*` and `tier: high`,
   **When** a pod running `nginx:1.25` is created, **Then** the controller applies
   `tier=high` to the pod within one reconcile cycle.
2. **Given** a pod already carries `tier=standard` and a new policy matches it with
   `tier=high`, **When** the policy is applied, **Then** the controller updates the label
   to `tier=high` in a single atomic write (no intermediate stale state).
3. **Given** a pod carries `tier=high` and the matching `PodLabelerPolicy` is deleted,
   **When** the controller reconciles, **Then** the `tier` label is removed from the pod.
4. **Given** a pod matches no `PodLabelerPolicy`, **When** the controller reconciles,
   **Then** no `tier` label is written to the pod, and any pre-existing `tier` label
   applied by the controller is removed.

---

### User Story 2 - No Lost Updates Under Concurrent Writes (Priority: P1)

The controller MUST never overwrite concurrent changes made by other actors (other
controllers, operators, or a parallel reconcile). All pod label writes use server-side
apply with field ownership, ensuring conflicts are detected and retried rather than
silently dropped.

**Why this priority**: A lost update on a tier label causes the wrong traffic tier to
persist until the next reconcile, routing premium users to standard pods. This is
classified as a correctness violation, not a performance issue.

**Independent Test**: Simulate a concurrent annotation on the pod between the controller's
read and write. Verify the controller's label write succeeds via server-side apply and
does not discard the concurrent change on unrelated fields.

**Acceptance Scenarios**:

1. **Given** a pod is being reconciled and a concurrent actor modifies an unrelated
   annotation, **When** the controller applies the tier label, **Then** the concurrent
   annotation is preserved and the tier label is correctly set.
2. **Given** two controller replicas are running (before leader election settles),
   **When** both attempt to label the same pod, **Then** exactly one write wins and the
   pod ends up with the correct tier label — no conflict error surfaces to the operator.
3. **Given** the API server returns a conflict error on a label write, **When** the
   controller handles the error, **Then** it requeues and retries the pod — it MUST NOT
   silently discard the conflict.

---

### User Story 3 - Controller Runs with Leader Election (Priority: P1)

The controller deployment supports multiple replicas for high availability. Only the
elected leader reconciles pods at any given time. Standby replicas take over within
the Kubernetes lease renewal window if the leader fails.

**Why this priority**: Without leader election, multiple replicas would race to label
pods, causing conflict storms and wasted API server calls at fleet scale.

**Independent Test**: Deploy two replicas of the controller; confirm via logs that only
one is actively reconciling pods; kill the active replica; confirm the standby takes
over and resumes labeling within the lease duration.

**Acceptance Scenarios**:

1. **Given** two controller replicas are deployed, **When** both start, **Then** only one
   acquires the leader lease and begins reconciling; the other waits in standby.
2. **Given** the leading replica crashes, **When** the lease expires, **Then** the standby
   replica acquires the lease and resumes reconciliation without manual intervention.
3. **Given** the deployment is configured for local development, **When** an operator
   attempts to disable leader election, **Then** the controller MUST refuse to start with
   a configuration error — leader election is not optional.

---

### User Story 4 - Example Scenario: Istio Traffic Split by Tier (Priority: P2)

An operator can deploy a reference example that demonstrates the full routing pipeline:
four pods behind a Kubernetes Service, a `DestinationRule` defining two subsets
(`high-priority-pods` selecting `tier=high` and `standard-pods` selecting `tier=standard`),
and a `VirtualService` that routes requests with the header `user-type: premium` to the
high subset and all other requests to the standard subset. The controller's labels are
the only mechanism that assigns pods to subsets.

**Why this priority**: The example proves end-to-end correctness of the label-to-routing
pipeline and gives operators a working reference for their own deployments.

**Independent Test**: Apply the example manifests; label two pods `tier=high` via policy
and two as `tier=standard`; send a request with `user-type: premium` and confirm it lands
on a high-tier pod; send a request without the header and confirm it lands on a
standard-tier pod.

**Acceptance Scenarios**:

1. **Given** four pods are running and the controller has applied tier labels (2 high,
   2 standard), **When** a request with header `user-type: premium` reaches the
   VirtualService, **Then** the request is routed exclusively to pods with `tier=high`.
2. **Given** the same four pods, **When** a request without `user-type: premium` reaches
   the VirtualService, **Then** the request is routed exclusively to pods with
   `tier=standard`.
3. **Given** a pod's tier label changes from `standard` to `high` (policy update),
   **When** the next request arrives, **Then** Istio routes the request according to the
   new label without requiring a VirtualService or DestinationRule change.

---

### Edge Cases

- What happens when a pod matches more than one `PodLabelerPolicy` with conflicting tiers?
- What happens when the API server is temporarily unreachable during a label write?
- What happens when a `PodLabelerPolicy` has a malformed or empty `imagePattern`?
- What happens when the controller restarts mid-reconcile and the pod is partially labeled?
- What happens when the cache returns a stale pod that no longer exists?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The controller MUST watch `PodLabelerPolicy` custom resources cluster-wide
  and reconcile all pods whose container images match the policy's `imagePattern` field.
- **FR-002**: The controller MUST apply the label `tier: <value>` to each matching pod
  using server-side apply, claiming field ownership, so concurrent writes by other actors
  are detected rather than silently overwritten.
- **FR-003**: The controller MUST remove the `tier` label from pods that no longer match
  any `PodLabelerPolicy`, using the same server-side apply mechanism.
- **FR-004**: ALL pod and policy reads inside the reconcile loop MUST come from the
  informer cache. Direct API server reads are prohibited inside Reconcile.
- **FR-005**: When the cache returns `NotFound` for a pod or policy, the controller MUST
  treat this as a transient condition, log it at debug level, and requeue — it MUST NOT
  treat `NotFound` as a terminal error.
- **FR-006**: Leader election MUST be enabled and non-configurable. The controller MUST
  refuse to start if leader election is disabled.
- **FR-007**: When a pod matches more than one `PodLabelerPolicy` with the same tier, the
  controller applies that tier. When policies conflict (different tiers for the same pod),
  the controller MUST apply a deterministic tie-break (e.g., alphabetical policy name
  order) and record the conflict as a warning event on the pod.
- **FR-008**: The controller MUST expose a reference example under `config/samples/`
  containing: a `PodLabelerPolicy`, a Deployment with four pods, a Kubernetes Service, a
  `DestinationRule` with `high-priority-pods` (selector `tier=high`) and `standard-pods`
  (selector `tier=standard`) subsets, and a `VirtualService` routing `user-type: premium`
  header traffic to the high subset and all other traffic to the standard subset.
- **FR-009**: The controller MUST emit a Kubernetes Event on each pod when a tier label
  is applied or removed, recording the policy name that triggered the change.
- **FR-010**: The controller MUST expose a `/healthz` readiness and liveness endpoint so
  Kubernetes can determine when the controller is ready to serve.

### Key Entities

- **PodLabelerPolicy**: A cluster-scoped custom resource defining one or more container
  image patterns and the target tier (`high` or `standard`) to assign to matching pods.
- **Pod (managed)**: Any pod in the cluster whose container images match at least one
  `PodLabelerPolicy`. The controller owns the `tier` label on these pods.
- **Tier Label**: The label `tier: high | standard` applied to a pod by the controller.
  This label is the sole input to Istio `DestinationRule` subset selectors.
- **DestinationRule Subset**: An Istio construct that groups pods by label selector.
  `high-priority-pods` selects `tier=high`; `standard-pods` selects `tier=standard`.
- **VirtualService**: An Istio construct that routes traffic based on request attributes.
  Routes `user-type: premium` header traffic to the high subset; all other traffic to
  the standard subset.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A pod matching a newly created `PodLabelerPolicy` receives the correct
  `tier` label within 5 seconds of the policy being applied under normal cluster load.
- **SC-002**: When a `PodLabelerPolicy` is deleted, all pods that were labeled by that
  policy have the `tier` label removed within 10 seconds.
- **SC-003**: Under a simulated concurrent write to an unrelated pod field, 100% of
  tier label writes succeed without data loss (no silent conflict drops).
- **SC-004**: With two controller replicas deployed, leader failover completes and
  reconciliation resumes within the configured lease duration (default: 15 seconds).
- **SC-005**: The reference example routes 100% of `user-type: premium` requests to
  high-tier pods and 100% of standard requests to standard-tier pods, verifiable by
  inspecting Envoy access logs or Istio telemetry.
- **SC-006**: The controller passes all `go test ./...` unit and integration tests
  without requiring a live cluster (envtest-based).

## Assumptions

- The `PodLabelerPolicy` CRD is already designed; this feature implements the first
  version of the controller that acts on it (`v1alpha1`).
- Image pattern matching uses glob syntax (e.g., `nginx:*`, `*/myapp:v1.*`); regex is
  out of scope for v1.
- The controller is cluster-scoped (watches pods in all namespaces) for v1; per-namespace
  scoping is a future enhancement.
- Tie-break between conflicting policies uses alphabetical policy name order; a more
  sophisticated priority field is a future enhancement.
- The reference example assumes the kind dev environment from feature 001 is already
  running with Istio installed.
- The controller binary is built with the project's standard Go toolchain; no custom
  build system is required.

---

## Non-Negotiable Engineering Constraints

These constraints are enforced at code-review and CI level. Any code pattern that violates
them MUST be rejected, regardless of whether tests pass.

### I. Reconciliation Correctness

**Forbidden write patterns** — MUST NOT appear anywhere in the controller or reconciler:

| Forbidden pattern | Reason |
|-------------------|--------|
| `client.Update(ctx, pod)` on Pod metadata | Overwrites entire object; no conflict detection; violates SSA invariant |
| `client.Patch(ctx, pod, client.MergeFrom(...))` on labels | Strategic merge patch has no field ownership; silently overwrites concurrent changes |
| `client.Patch` with `types.JSONPatchType` on labels | Same as above — bypasses field ownership |

**Mandatory write pattern** for every pod label mutation:

```go
apply := &corev1.Pod{
    TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
    ObjectMeta: metav1.ObjectMeta{
        Name:      pod.Name,
        Namespace: pod.Namespace,
        Labels:    map[string]string{"tier": computedTier},
    },
}
err := r.Patch(ctx, apply, client.Apply,
    client.FieldOwner("mesh-priority-controller"),
    client.ForceOwnership)
```

For label removal, omit the `tier` key from Labels entirely.

### II. Cache-Safe Reads

- ALL reads inside `Reconcile` (pods, policies) MUST come from the informer cache via the
  cache-backed `r.Client`.
- Direct `r.APIReader` or `r.Client.Get` calls that bypass the cache are prohibited inside
  `Reconcile`.
- When the cache returns `apierrors.IsNotFound(err)` for a pod: treat as transient, do NOT
  log at Error level, return `ctrl.Result{}, nil` (no error, no panic).
- When the cache returns any other error: return the error so controller-runtime requeues
  with exponential backoff.

### III. Leader Election

- `ctrl.Options.LeaderElection` MUST be `true` — hardcoded, not configurable via flag or
  environment variable.
- `LeaderElectionID` MUST be `"mesh-priority-controller.knabben.github.com"`.
- `LeaderElectionReleaseOnCancel` MUST be `true`.
- The controller MUST refuse to start (exit non-zero) if `LeaderElection` is `false` — a
  startup validation check MUST assert this before `mgr.Start(ctx)`.

### IV. Required Integration Tests

The following three tests MUST exist with these exact function names. Tests that exercise
the same scenarios under different names do NOT satisfy the requirement.

| Test Name | Scenario Verified |
|-----------|------------------|
| `TestReconcile_NoLostUpdates` | SSA write preserves unrelated concurrent field changes; assert no `MergePatch` or `Update` call occurs on Pod |
| `TestReconcile_CacheNotFound` | Cache returns `NotFound` for requested pod → reconciler returns `ctrl.Result{}, nil`; no error; no panic |
| `TestReconcile_LeaderElection` | Manager configured with `LeaderElection: false` → startup validation rejects it before `mgr.Start` |

All three tests MUST run without a live cluster (envtest or fakes only) and MUST pass with
`go test -race ./...`.

### V. Observability

The controller MUST expose these Prometheus metrics. Metrics MUST be registered at package
init time (not inside Reconcile) using `prometheus.MustRegister`.

| Metric name | Type | Labels | Incremented when |
|-------------|------|--------|-----------------|
| `mesh_priority_labels_applied_total` | Counter | `tier`, `namespace` | A tier label is written (new or updated value) |
| `mesh_priority_labels_skipped_total` | Counter | `namespace` | Diff-gate fires — label already correct, no write |
| `mesh_priority_reconcile_errors_total` | Counter | `error_category` | Reconcile returns an error |
| `mesh_priority_policy_evaluations_total` | Counter | `result` | Each policy match evaluation completes |

Every label mutation MUST also emit a Kubernetes Event on the pod with:
- `reason`: `TierLabelApplied`, `TierLabelRemoved`, or `TierConflict`
- `type`: `Normal` for apply/remove, `Warning` for conflict
- `message`: includes policy name, old value (or `<none>`), new value

Every label mutation MUST be logged at `Info` level with structured fields:
`pod`, `namespace`, `old_tier` (or `<none>`), `new_tier`, `policy`.

### VI. Deployment Constraints

The controller Deployment manifest (`config/manager/manager.yaml`) MUST specify:

| Field | Required value |
|-------|---------------|
| `replicas` | `2` |
| `securityContext.runAsNonRoot` | `true` |
| `securityContext.readOnlyRootFilesystem` | `true` |
| `resources.requests.cpu` | `50m` |
| `resources.requests.memory` | `64Mi` |
| `resources.limits.cpu` | `200m` |
| `resources.limits.memory` | `128Mi` |
| Namespace | `mesh-priority-system` |

### VII. RBAC Requirements

The controller ServiceAccount MUST be bound to a ClusterRole with exactly these rules and
no broader permissions:

| Resource | API group | Verbs |
|----------|-----------|-------|
| `pods` | `""` (core) | `get`, `list`, `watch`, `patch` |
| `podlabelerpolicies` | `mesh.knabben.github.com` | `get`, `list`, `watch` |
| `podlabelerpolicies/status` | `mesh.knabben.github.com` | `update`, `patch` |
| `events` | `""` (core) | `create`, `patch` |
| `leases` | `coordination.k8s.io` | `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` |

### VIII. Commit Conventions

All commits touching this feature MUST use Conventional Commits format:

- `feat:` — new behavior visible to users or operators
- `fix:` — corrections to existing behavior
- `test:` — test-only changes
- `chore:` — scaffolding, build, config, generated files
- `docs:` — documentation only

Commit messages MUST be imperative mood, present tense, ≤72 characters on the subject line.
