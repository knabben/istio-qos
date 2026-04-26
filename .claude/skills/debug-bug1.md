# Debug Bug 1 — Lost Update (r.Update race)

## Overview

Bug 1 is a lost-update race in `act1/controller/reconciler.go`. The reconciler calls
`r.Update(ctx, pod)` which sends a full PUT carrying the pod's in-memory
`resourceVersion`. When two replicas (or two rapid reconcile cycles) both read the
pod at the same `resourceVersion`, whichever PUT arrives second gets a 409 Conflict.
The error is returned to the workqueue without retry, silently dropping one set of
labels. In the Istio mesh this causes flicker: the tier label disappears, Envoy
re-queries xDS, and the control plane floods with push notifications.

## Step 1 — Detect with MCP tools

Run the following MCP queries in order:

```
list_pods namespace=podlabeler-system          # confirm controller is running
list_pod_logs pod_name=<controller-pod> namespace=podlabeler-system tail_lines=200
```

Look for log lines matching:
- `"failed to update pod"` or `"conflict"` or `"the object has been modified"`
- Two rapid label-set writes to the same pod within milliseconds

Then inspect the labelled pods:
```
list_pods namespace=default
```

If `tier` labels are absent or inconsistent across pods that should match the same
PodLabelerPolicy, Bug 1 is confirmed.

Codebase pointer — the defective line is:
```
act1/controller/reconciler.go : line 83  →  r.Update(ctx, pod)
```

## Step 2 — Apply the fix

The fix replaces `r.Update()` with a server-side apply patch. Open the file and
replace the update block (approximately lines 79-86) with:

```go
patch := &corev1.Pod{
    TypeMeta: metav1.TypeMeta{
        APIVersion: "v1",
        Kind:       "Pod",
    },
    ObjectMeta: metav1.ObjectMeta{
        Name:      pod.Name,
        Namespace: pod.Namespace,
        Labels:    desired,
    },
}
if err := r.Patch(ctx, patch, client.Apply,
    client.ForceOwnership, client.FieldOwner("podlabeler")); err != nil {
    logger.Error(err, "failed to patch pod labels")
    return ctrl.Result{}, err
}
```

Also add the diff-gate above the patch (skip write if labels already correct):

```go
if labelsAlreadyApplied(pod.Labels, desired) {
    logger.Info("labels already correct, skipping write", "labels", desired)
    return ctrl.Result{}, nil
}
```

The reference implementation is at:
```
act2/controller/reconciler.go : lines 84-113
```

## Step 3 — Rebuild and redeploy

```bash
cd act1
make docker-build          # IMAGE_TAG=podlabeler:act1
make kind-load             # loads into kind cluster istio-qos
kubectl rollout restart deployment podlabeler-controller -n podlabeler-system
kubectl rollout status deployment podlabeler-controller -n podlabeler-system
```

## Step 4 — Verify test passes

```bash
cd act1
make test-bugs             # KUBEBUILDER_ASSETS must be set
```

Expected output — `TestBug1_LostUpdate` changes from FAIL to PASS.

Alternatively, run the act2 suite (which targets the pre-fixed code):
```bash
cd act2
make test
```

Expected: `TestBug1_Fixed --- PASS`.
