# Quickstart: PodLabeler Bug Baseline (Act I)

**Feature**: 003-podlabeler-bug-baseline
**Date**: 2026-04-26

This guide covers two modes: running the bug-exposure test suite (envtest, no cluster
required) and deploying the full Act I demo to a kind cluster to see the live symptoms.

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.22+ | Build and test |
| kubectl | any | Apply manifests to kind cluster |
| kind | any | Create local cluster for live demo (optional) |
| Docker | any | Build image for kind demo (optional) |

---

## Step 1: Extract the Act I Code

The implementation lives at `bugs/podlabeler-act1/` (extracted from `bugs/files.zip`).

```bash
cd bugs/podlabeler-act1
go mod download
```

---

## Step 2: Run the Bug-Exposure Test Suite

The three tests prove each bug exists. **All three are expected to FAIL on Act I.**

```bash
# Install envtest binaries (first time only)
make setup-envtest

# Run bug-exposure tests — expect 3 failures
make test-bugs
```

Expected output:

```
--- FAIL: TestBug1_LostUpdate (0.45s)
    reconciler_test.go:87: expected pod to carry both labels {policy-a:applied policy-b:applied}
        got: map[policy-b:applied]
--- FAIL: TestBug2_StaleCache (0.12s)
    reconciler_test.go:134: expected nil error for missing pod, got:
        pods "nonexistent-pod" not found
--- FAIL: TestBug3_NoLease (2.08s)
    reconciler_test.go:178: expected 1 Lease in namespace podlabeler-system, got 0
FAIL
```

This is the correct output for Act I. All three failures confirm the bugs are present.

---

## Step 3: Verify the Failure Count

The Makefile target `test-bugs-assert-failures` counts failures and asserts exactly 3:

```bash
make test-bugs-assert-failures
# Exits 0 if exactly 3 tests failed, non-zero otherwise
```

---

## Step 4 (Optional): Deploy to a kind Cluster — Live Demo

To see the live symptoms (label oscillation, no Lease):

```bash
# Create cluster and load image
kind create cluster --name podlabeler-demo
docker build -t podlabeler:act1 .
kind load docker-image podlabeler:act1 --name podlabeler-demo

# Apply everything
kubectl apply -f manifests/crd.yaml
kubectl apply -f manifests/rbac.yaml
kubectl apply -f manifests/controller.yaml    # replicas: 2, LeaderElection: false
kubectl apply -f manifests/sample-workload.yaml
kubectl apply -f manifests/sample-policies.yaml
kubectl apply -f manifests/istio/             # if Istio is installed

# Watch Bug 3 symptoms: no Lease, both replicas active
kubectl get lease -n podlabeler-system        # returns: No resources found

# Watch Bug 1 + Bug 3 amplified: label oscillation
kubectl get pods -n my-service -L tier --watch
# tier column will flicker between values as two replicas race
```

---

## Makefile Targets

| Target | What it does |
|--------|-------------|
| `make setup-envtest` | Download kube-apiserver + etcd binaries for envtest |
| `make test-bugs` | Run the 3 bug-exposure tests (expects all to FAIL) |
| `make test-bugs-assert-failures` | Assert exactly 3 tests failed |
| `make build` | `go build ./...` — verifies code compiles |
| `make vet` | `go vet ./...` — verifies no type errors |

---

## What to Expect in Act II

In Act II, three changes are applied to the same code:

1. **Bug 1 fix**: `r.Update(pod)` → `r.Patch(pod, client.Apply, client.FieldOwner("podlabeler"), client.ForceOwnership)`
2. **Bug 2 fix**: `return ctrl.Result{}, err` → `if apierrors.IsNotFound(err) { return ctrl.Result{}, nil }`
3. **Bug 3 fix**: `LeaderElection: false` → `LeaderElection: true, LeaderElectionID: "podlabeler.knabben.dev"`

After each fix, the corresponding test flips from FAIL to PASS.
The final Act II state: all three tests pass; the core labeling test (US1) still passes.
