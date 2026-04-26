# 002 — Pod Tier Label Controller

A Kubernetes controller that labels pods with `tier: high | standard` derived from
`PodLabelerPolicy` custom resources. The labels drive Istio `DestinationRule` subset
selection, routing premium traffic to high-tier pods.

## Spec documents

| Document | Purpose |
|----------|---------|
| [spec.md](spec.md) | Feature requirements, user stories, §I-VIII non-negotiable constraints |
| [plan.md](plan.md) | Implementation plan: tech stack, project structure, forbidden patterns |
| [research.md](research.md) | Architecture decisions (kubebuilder v4, SSA, gobwas/glob, envtest) |
| [data-model.md](data-model.md) | CRD schema, state transitions, DestinationRule/VirtualService examples |
| [contracts/reconciler.md](contracts/reconciler.md) | 13-scenario reconcile contract table, RBAC, metrics |
| [quickstart.md](quickstart.md) | End-to-end deploy and verify walkthrough |
| [tasks.md](tasks.md) | 36 implementation tasks across 7 phases |

---

## Prerequisites

| Tool | Minimum version | Install |
|------|----------------|---------|
| Go | 1.22 | `go install` or [go.dev](https://go.dev/dl) |
| kubebuilder | v4 | `go install sigs.k8s.io/kubebuilder/v4/cmd/kubebuilder@latest` |
| kind | any | `go install sigs.k8s.io/kind@latest` |
| kubectl | 1.28+ | package manager or [kubernetes.io](https://kubernetes.io/docs/tasks/tools/) |
| istioctl | 1.24.2 | `curl -L https://istio.io/downloadIstio \| ISTIO_VERSION=1.24.2 sh -` |
| Docker | any | [docker.com](https://docs.docker.com/get-docker/) |
| golangci-lint | 1.57+ | `go install github.com/golangci-lint/golangci-lint/cmd/golangci-lint@latest` |

The kind cluster and Istio must already be running (see [feature 001](../001-kind-istio-setup/)).
If not, run:

```bash
bash hack/bootstrap.sh      # create kind cluster + local registry
bash hack/install-istio.sh  # install Istio 1.24.2 demo profile
```

---

## Install (scaffold + build)

```bash
# 1. Scaffold kubebuilder project (run once, in repo root)
kubebuilder init --domain knabben.github.com \
  --repo github.com/knabben/istio-qos

kubebuilder create api --group mesh --version v1alpha1 \
  --kind PodLabelerPolicy --namespaced=false \
  --resource --controller

# 2. Add glob dependency
go get github.com/gobwas/glob
go mod tidy

# 3. Generate CRD manifests and deepcopy code
make generate manifests

# 4. Build the controller binary
make build

# 5. Build and push container image to local registry
make docker-build IMG=localhost:5000/mesh-priority-controller:dev
docker push localhost:5000/mesh-priority-controller:dev
```

---

## Deploy to kind

```bash
# Install CRD and deploy controller
make deploy IMG=localhost:5000/mesh-priority-controller:dev

# Verify controller pods are running (2 replicas, one holds leader lease)
kubectl -n mesh-priority-system get pods
kubectl -n mesh-priority-system get lease mesh-priority-controller.knabben.github.com -o yaml
```

---

## Test

### Unit + integration tests (no live cluster required)

```bash
# Download envtest binaries
make envtest

# Run all tests with race detector
make test   # equivalent: go test -race ./... with KUBEBUILDER_ASSETS set

# Coverage report for reconciler package
go test -race -coverprofile=coverage.out ./internal/controller/...
go tool cover -func=coverage.out | grep -E "^total|controller"
```

The three integration tests required by spec §IV must be present and pass:

| Test | File | Verifies |
|------|------|---------|
| `TestReconcile_NoLostUpdates` | `internal/controller/podlabelerpolicy_controller_test.go` | SSA write preserves concurrent unrelated field changes |
| `TestReconcile_CacheNotFound` | `internal/controller/podlabelerpolicy_controller_test.go` | `NotFound` from cache → `ctrl.Result{}, nil`, no error |
| `TestReconcile_LeaderElection` | `internal/controller/podlabelerpolicy_controller_test.go` | `LeaderElection: false` → startup validation rejects |

### Lint

```bash
golangci-lint run ./...
```

### Policy behavior test (requires live kind cluster + deployed controller)

```bash
bash hack/test-policy.sh
```

This script:
1. Creates a `PodLabelerPolicy` matching `nginx:*` → `tier: standard`
2. Creates a test pod running `nginx:latest`
3. Polls until the pod receives `tier=standard` (60 s timeout) — exits 1 on timeout
4. Deletes the policy
5. Polls until the `tier` label is removed from the pod — exits 1 on timeout
6. Cleans up; exits 0 on success

---

## Validate end-to-end (Istio routing)

Apply the reference example to the kind cluster:

```bash
kubectl apply -f config/samples/
```

This creates:
- `PodLabelerPolicy` `policy-high` (pattern `*/tier-app-high:*` → `tier: high`)
- `PodLabelerPolicy` `policy-standard` (pattern `*/tier-app-standard:*` → `tier: standard`)
- Deployment with 4 pods (2 high-image, 2 standard-image)
- Service `tier-app-svc`
- `DestinationRule` with `high-priority-pods` and `standard-pods` subsets
- `VirtualService` routing `user-type: premium` → high subset, default → standard subset

**Verify tier labels:**

```bash
kubectl get pods -L tier
# NAME                   READY   STATUS    TIER
# tier-app-xxx-aaa       1/1     Running   high
# tier-app-xxx-bbb       1/1     Running   high
# tier-app-xxx-ccc       1/1     Running   standard
# tier-app-xxx-ddd       1/1     Running   standard
```

**Verify traffic routing:**

```bash
# Premium request → should land on a tier=high pod
kubectl exec -it <any-pod> -- curl -H "user-type: premium" http://tier-app-svc/

# Standard request → should land on a tier=standard pod
kubectl exec -it <any-pod> -- curl http://tier-app-svc/

# Confirm via Istio access logs
kubectl logs -l app=tier-app -c istio-proxy | grep "user-type"
```

**Verify leader election:**

```bash
kubectl -n mesh-priority-system get lease mesh-priority-controller.knabben.github.com -o yaml
# holderIdentity should match exactly one pod name
```

---

## Teardown

```bash
make undeploy       # remove controller and CRD from cluster
bash hack/teardown.sh  # delete kind cluster and local registry
```

---

## Non-negotiable constraints (spec §I-VIII)

See [spec.md § Non-Negotiable Engineering Constraints](spec.md) for the full list.
Key points:

- All pod label writes MUST use `client.Apply` + `client.FieldOwner("mesh-priority-controller")` — no `client.Update()` or `MergePatch` on labels
- `NotFound` from cache → `return ctrl.Result{}, nil` — never return a `NotFound` error
- `LeaderElection: true` is hardcoded; the controller exits non-zero if set to `false`
- Coverage gate: reconciler package ≥ 80%
- All three integration tests must exist with exact function names
