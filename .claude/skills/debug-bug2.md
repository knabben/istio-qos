# Debug Bug 2 — Stale Cache / NotFound error propagation

## Overview

Bug 2 is in `act1/controller/reconciler.go`. When the reconciler calls `r.Get()` for
a pod that no longer exists (deleted pod, or informer cache lag), the Kubernetes client
returns a `NotFound` error. The buggy code propagates this error back to the workqueue,
which applies exponential backoff and eventually drops the event. The correct behavior
is to treat `NotFound` as a benign, transient condition and return `nil` so the
workqueue discards the request cleanly.

The symptom in production: pods that are created and quickly reconciled may never
receive their tier label. The controller log shows repeated `"failed to get pod"`
messages for pods that were already relabelled and deleted.

## Step 1 — Detect with MCP tools

```
list_pod_logs pod_name=<controller-pod> namespace=podlabeler-system tail_lines=300
```

Look for:
- `"failed to get pod"` followed by `"not found"` or `"404"`
- Repeated reconcile errors for the same pod name after it has been deleted
- Workqueue depth growing without draining

Then check events:
```
list_events namespace=podlabeler-system
```

Look for `Warning` events from the controller referencing a pod that no longer exists.

Codebase pointer — the defective block is:
```
act1/controller/reconciler.go : lines 38-41
```

Buggy code:
```go
if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
    return ctrl.Result{}, err  // Bug 2: NotFound propagated as error
}
```

## Step 2 — Apply the fix

Replace the Get block with a NotFound guard:

```go
if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
    if apierrors.IsNotFound(err) {
        return ctrl.Result{}, nil  // benign: pod deleted or cache lagging
    }
    return ctrl.Result{}, err
}
```

Add the import if not present:
```go
apierrors "k8s.io/apimachinery/pkg/api/errors"
```

The reference implementation is at:
```
act2/controller/reconciler.go : lines 38-48
```

## Step 3 — Rebuild and redeploy

```bash
cd act1
make docker-build
make kind-load
kubectl rollout restart deployment podlabeler-controller -n podlabeler-system
kubectl rollout status deployment podlabeler-controller -n podlabeler-system
```

## Step 4 — Verify test passes

```bash
cd act1
make test-bugs
```

Expected: `TestBug2_StaleCache` changes from FAIL to PASS.

Or with the act2 suite:
```bash
cd act2
make test
```

Expected: `TestBug2_Fixed --- PASS`.
