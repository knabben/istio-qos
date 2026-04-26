# Debug Bug 3 — Missing Leader Election

## Overview

Bug 3 is in `act1/main.go`. The controller-runtime manager is created with
`LeaderElection: false` (the default). When the Deployment has `replicas: 2`, both
pods start a reconcile loop simultaneously. Each sees the same pod events, each
computes the same desired labels, and each calls `r.Update()` (Bug 1). This doubles
the 409-Conflict rate and creates a continuous label-flicker loop. Every label change
triggers an Istio xDS push, so the control plane is flooded even when no real
configuration changed.

The fix is to enable leader election: only one replica holds the Lease and reconciles
at a time. The standby replica takes over only if the leader crashes.

## Step 1 — Detect with MCP tools

Check whether a leader-election Lease exists:
```
list_leases namespace=kube-system
```

With Bug 3 active (`LeaderElection: false`) no Lease is created — the output will be
empty or show only system leases unrelated to podlabeler.

Check how many controller replicas are running:
```
list_pods namespace=podlabeler-system
```

If you see two pods both in Running state, two reconcilers are active simultaneously.

Confirm the race by tailing logs from both pods in parallel:
```
list_pod_logs pod_name=<controller-pod-1> namespace=podlabeler-system tail_lines=100
list_pod_logs pod_name=<controller-pod-2> namespace=podlabeler-system tail_lines=100
```

Look for interleaved `"applied labels"` messages from both pods for the same target pod.

Codebase pointer — the defective manager setup is:
```
act1/main.go : lines 51-65  (ctrl.NewManager call — no LeaderElection field)
```

## Step 2 — Apply the fix

In `act1/main.go`, add the leader-election options to `ctrl.NewManager()`:

```go
mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    Scheme:                 scheme,
    Metrics:                metricsserver.Options{BindAddress: metricsAddr},
    HealthProbeBindAddress: probeAddr,

    // Fix 3: enable leader election so only one replica reconciles at a time.
    LeaderElection:                true,
    LeaderElectionID:              "podlabeler.knabben.dev",
    LeaderElectionNamespace:       "kube-system",
    LeaderElectionReleaseOnCancel: true,
})
```

Also reduce `replicas` to 1 in `act1/manifests/controller.yaml` — with leader election
a single replica is sufficient for HA (the standby would restart automatically):

```yaml
spec:
  replicas: 1
```

The reference implementation is at:
```
act2/main.go         : lines 51-65
act2/manifests/controller.yaml : line 13  (replicas: 1)
```

## Step 3 — Rebuild and redeploy

```bash
cd act1
make docker-build
make kind-load
kubectl apply -f manifests/controller.yaml   # picks up replicas: 1 + new image
kubectl rollout status deployment podlabeler-controller -n podlabeler-system
```

After rollout verify the Lease is created:
```
list_leases namespace=kube-system
```

Expected: one Lease named `podlabeler.knabben.dev` in `kube-system`.

## Step 4 — Verify test passes

```bash
cd act1
make test-bugs
```

Expected: `TestBug3_NoLease` changes from FAIL to PASS.

Or with the act2 suite:
```bash
cd act2
make test
```

Expected: `TestBug3_Fixed --- PASS`.
