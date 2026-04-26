# Quickstart: Pod Tier Label Controller

**Feature**: 002-pod-label-controller
**Date**: 2026-04-26

## Prerequisites

- Kind cluster running with Istio installed (see `hack/bootstrap.sh` and `hack/install-istio.sh`)
- `kubectl` configured for context `kind-istio-qos`
- `kubebuilder` v4 installed
- `go` 1.22+

---

## Step 1: Scaffold the Project

```bash
kubebuilder init --domain knabben.github.com \
  --repo github.com/knabben/istio-qos

kubebuilder create api --group mesh --version v1alpha1 \
  --kind PodLabelerPolicy --namespaced=false \
  --resource --controller
```

---

## Step 2: Run Unit + Integration Tests

```bash
make envtest    # downloads envtest binaries
make test       # runs go test ./... with KUBEBUILDER_ASSETS set
```

All tests MUST pass with `-race`. Coverage gate: reconciler package ≥ 80%.

---

## Step 3: Deploy to Kind

```bash
make docker-build IMG=localhost:5000/mesh-priority-controller:dev
docker push localhost:5000/mesh-priority-controller:dev
make deploy IMG=localhost:5000/mesh-priority-controller:dev
kubectl -n mesh-priority-system get pods
```

---

## Step 4: Apply the Reference Example

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

---

## Step 5: Verify Tier Labels

```bash
kubectl get pods -L tier
# NAME                   READY   TIER
# tier-app-xxx-aaa       1/1     high
# tier-app-xxx-bbb       1/1     high
# tier-app-xxx-ccc       1/1     standard
# tier-app-xxx-ddd       1/1     standard
```

Or use the automated test script:

```bash
bash hack/test-policy.sh
```

---

## Step 6: Test Traffic Routing

Send a premium request (should land on a `tier=high` pod):

```bash
kubectl exec -it <any-pod> -- curl -H "user-type: premium" http://tier-app-svc/
```

Send a standard request:

```bash
kubectl exec -it <any-pod> -- curl http://tier-app-svc/
```

Check Istio access logs to confirm routing:

```bash
kubectl logs -l app=tier-app -c istio-proxy | grep "user-type"
```

---

## Step 7: Verify Leader Election

```bash
kubectl -n mesh-priority-system get lease mesh-priority-controller.knabben.dev -o yaml
# holderIdentity should match exactly one pod name
```

---

## Teardown

```bash
make undeploy
bash hack/teardown.sh
```
