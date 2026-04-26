# Contract: PodReconciler (Act I Baseline)

**Feature**: 003-podlabeler-bug-baseline
**Date**: 2026-04-26

This contract documents the **actual behavior** of the Act I reconciler — including the
three deliberate bugs. It is the authoritative reference for the bug-exposure test suite:
each test asserts the CORRECT behavior that contradicts one of the documented bugs.

---

## Reconcile Function Signature

```go
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
```

### Input

| Parameter | Type | Description |
|-----------|------|-------------|
| `ctx` | `context.Context` | Request context (carries logger via `log.FromContext`) |
| `req.NamespacedName` | `types.NamespacedName` | Pod namespace + name to reconcile |

### Output

| Return | Type | When |
|--------|------|------|
| `ctrl.Result{}` | zero value | Normal completion (pod skipped or labeled) |
| `error` | non-nil | Any failure including **NotFound** (Bug 2) and write failure |

---

## Behavior Contract (Act I — Buggy)

### Step 1: Fetch Pod from Cache

```
r.Get(ctx, req.NamespacedName, pod)
```

| Outcome | Act I behavior | Correct behavior |
|---------|----------------|------------------|
| Pod found in cache | Continue to Step 2 | Same |
| Pod not found in cache (transient lag) | **Returns error** ← Bug 2 | Return `ctrl.Result{}, nil` |
| Pod not found (deleted) | **Returns error** ← Bug 2 | Return `ctrl.Result{}, nil` |
| Other API error | Returns error | Returns error |

**Bug 2 contract**: `r.Get()` returning `NotFound` MUST be treated as a terminal error in
Act I. The test asserts `err == nil`; the Act I code returns the error. Test FAILS. ✓

---

### Step 2: Skip Conditions (Correct in Act I)

| Condition | Behavior |
|-----------|----------|
| `pod.DeletionTimestamp != nil` | Return `ctrl.Result{}, nil` |
| `len(pod.Spec.Containers) == 0` | Return `ctrl.Result{}, nil` |

---

### Step 3: List All PodLabelerPolicies

```
r.List(ctx, policies)
```

Cluster-scoped list — no namespace filter. Errors are returned as-is.

---

### Step 4: Compute Desired Labels

Walk all policies. For each where `imageMatches(primaryImage, policy.Spec.ImagePattern)`:
merge `policy.Spec.Labels` into `desired` map. If no match, return without writing.

---

### Step 5: Write Labels

```
r.Update(ctx, pod)   // full PUT — Bug 1
```

| Scenario | Act I behavior | Correct behavior |
|----------|----------------|------------------|
| Sequential single reconcile | Labels applied correctly | Same |
| Concurrent reconcile (two goroutines) | **One write clobbers the other** ← Bug 1 | Both writes survive (SSA field ownership) |
| Kubelet status update races write | **409 Conflict propagated** ← Bug 1 | Retry with SSA resolves conflict |

**Bug 1 contract**: `r.Update()` uses `resourceVersion` from the informer cache snapshot.
Concurrent writes to the same pod with different `resourceVersion`s conflict. The losing
write is silently discarded (if the winner's version matches) or fails with 409 (if a
third writer intervened). Test asserts both label keys survive concurrent writes. Act I
loses one. Test FAILS. ✓

---

## Manager Configuration Contract (Bug 3)

```go
ctrl.Options{
    LeaderElection: false,   // Bug 3 — never creates a Lease
}
```

| Observable | Act I | Correct |
|------------|-------|---------|
| `Lease` in `podlabeler-system` namespace | **Does not exist** ← Bug 3 | Exists, held by active replica |
| Both replicas reconcile simultaneously | Yes | No — only leader reconciles |
| Label oscillation with 2 replicas | Yes (amplified by Bug 1) | No |

**Bug 3 contract**: `LeaderElection: false` means no `Lease` object is created. Test lists
Leases in the namespace and asserts `len(items) == 1`. Act I returns 0. Test FAILS. ✓

---

## imageMatches Function Contract (Correct in Act I)

```go
func imageMatches(image, pattern string) bool
```

Registry prefix stripped before matching: `registry.io/team/app1:v1` → `app1:v1`.

| Pattern | Example image | Matches? |
|---------|---------------|----------|
| `app1:latest` | `app1:latest` | Yes (exact) |
| `app1:latest` | `app1:v2` | No |
| `app1:*` | `app1:v2` | Yes (wildcard tag) |
| `app1:*` | `app2:v1` | No |
| `app1` | `app1:v1` | Yes (prefix) |
| `app1` | `app1-extra:v1` | Yes (prefix) |
| `app1` | `app2:v1` | No |

---

## Bug-Exposure Test Summary

| Test | Asserts | Fails on Act I because |
|------|---------|------------------------|
| `TestBug1_LostUpdate` | Pod has both labels after concurrent reconcile | One label clobbered |
| `TestBug2_StaleCache` | Reconcile returns `nil` for unknown pod | Returns `NotFound` error |
| `TestBug3_NoLease` | Manager creates a `Lease` on startup | No Lease created |

---

## envtest Setup Contract

```go
env := &envtest.Environment{
    CRDDirectoryPaths: []string{"../manifests"},
}
cfg, _ := env.Start()
defer env.Stop()
```

The `crd.yaml` in `manifests/` is loaded automatically. No kubebuilder setup-envtest
binary is required for CRD loading. The etcd and kube-apiserver binaries must be on
`$PATH` or in `$KUBEBUILDER_ASSETS` (populated by `make setup-envtest`).
