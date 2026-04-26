# Reconciler Contracts: Pod Tier Label Controller

**Feature**: 002-pod-label-controller
**Date**: 2026-04-26

These contracts define the behavioural guarantees of the reconciler — its inputs, outputs,
invariants, and error-handling commitments. They drive integration test cases.

---

## Reconcile(ctx, Request) → (Result, error)

**Input**: A `reconcile.Request` carrying a `types.NamespacedName` identifying a Pod.

**Pre-conditions**:
- The manager's informer cache is synced.
- Leader election lease is held by this replica.

### Contract Table

| Scenario | Expected output | Principle |
|----------|----------------|-----------|
| Pod not found in cache (transient) | Requeue after backoff; no error returned | FR-005 |
| Pod found; matches exactly one policy; label absent | Apply `tier=<value>`; emit `TierLabelApplied` event; return no error | FR-001, FR-002 |
| Pod found; matches one policy; label already correct | No write; return no error (diff-gate) | II (Label Stability) |
| Pod found; matches one policy; label wrong value | Apply correct `tier=<value>`; emit event | FR-002 |
| Pod found; matches no policy; label present | Apply patch removing `tier`; emit `TierLabelRemoved` | FR-003 |
| Pod found; matches no policy; label absent | No-op; return no error | FR-001 |
| Pod found; matches two policies with same tier | Apply that tier; no conflict event | FR-007 |
| Pod found; matches two policies with different tiers | Apply tier from alphabetically-first policy name; emit `TierConflict` Warning event | FR-007 |
| Policy list from cache returns error (not NotFound) | Return error; controller-runtime requeues with backoff | IV (fail-closed) |
| Server-side apply returns conflict error | Return error; controller-runtime requeues | FR-002 |
| Server-side apply returns any non-conflict error | Return error; controller-runtime requeues | FR-002 |

### Invariants (MUST hold after every successful reconcile)

1. **Exactly-one-label**: A pod that matches ≥1 policy carries exactly one `tier` label.
2. **No-stale-label**: A pod that matches zero policies carries no `tier` label owned by
   `mesh-priority-controller`.
3. **No-redundant-write**: If the pod already carries the correct `tier` value, zero API
   server calls are made to mutate the pod.
4. **Field-ownership**: The `tier` label field is always owned by field manager
   `mesh-priority-controller`. No other field is touched.

---

## PodLabelerPolicy Reconcile (secondary — enqueues pods)

When a `PodLabelerPolicy` event fires, the handler enqueues reconcile requests for all
pods in the cluster. The policy itself is not mutated by this handler.

### Contract

| Scenario | Expected action |
|----------|----------------|
| Policy created | All pods enqueued |
| Policy updated (imagePattern or tier changed) | All pods enqueued |
| Policy deleted | All pods enqueued (pods with this policy's tier will have label removed if no other policy matches) |

---

## CRD Schema Contract: PodLabelerPolicy

| Field | Type | Validation | Error on violation |
|-------|------|------------|-------------------|
| `spec.imagePattern` | `string` | `minLength: 1` | CRD validation webhook (v1: status condition) |
| `spec.tier` | `string` enum | `enum: [high, standard]` | CRD CEL rule reject at admission |

---

## RBAC Requirements

The controller ServiceAccount MUST have the following ClusterRole rules:

| Resource | Verbs |
|----------|-------|
| `pods` | `get`, `list`, `watch`, `patch` |
| `podlabelerpolicies` | `get`, `list`, `watch` |
| `podlabelerpolicies/status` | `update`, `patch` |
| `events` | `create`, `patch` |
| `leases` (coordination.k8s.io) | `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` |

---

## Health Endpoints

| Endpoint | Purpose | Expected response |
|----------|---------|-------------------|
| `/healthz` | Liveness probe | `200 OK` when manager is running |
| `/readyz` | Readiness probe | `200 OK` when informer cache is synced |

---

## Metrics Exposed

| Metric name | Labels | Description |
|-------------|--------|-------------|
| `mesh_priority_labels_applied_total` | `tier`, `namespace` | Label writes (new or updated) |
| `mesh_priority_labels_skipped_total` | `namespace` | No-op skips (label already correct) |
| `mesh_priority_reconcile_errors_total` | `error_category` | Reconcile failures by category |
| `mesh_priority_policy_evaluations_total` | `result` | Policy match evaluations |
