# Quickstart: Kind Development Environment Setup

**Feature**: 001-kind-istio-setup
**Date**: 2026-04-26

This guide walks through setting up a complete local development environment for
`mesh-priority-controller` using kind, a local container registry, and Istio.

## Prerequisites

Install the following tools before running any script:

| Tool        | Purpose                                   | Install                                  |
|-------------|-------------------------------------------|------------------------------------------|
| `docker`    | Run kind nodes and the local registry     | https://docs.docker.com/get-docker/      |
| `kind`      | Create local Kubernetes clusters          | `go install sigs.k8s.io/kind@latest`    |
| `kubectl`   | Interact with the cluster                 | https://kubernetes.io/docs/tasks/tools/ |
| `istioctl`  | Install the Istio service mesh            | https://istio.io/latest/docs/setup/install/istioctl/ |

Verify all tools are available:

```bash
docker info
kind version
kubectl version --client
istioctl version --remote=false
```

---

## Step 1: Bootstrap the Kind Cluster and Registry

```bash
bash hack/bootstrap.sh
```

Expected output (excerpt):
```
[bootstrap] Starting local registry on localhost:5000 ...
[bootstrap] Creating kind cluster 'istio-qos' ...
[bootstrap] Connecting registry to kind network ...
[bootstrap] Done. Cluster 'istio-qos' is ready.
```

### Verify Step 1

```bash
# Cluster nodes are Ready
kubectl get nodes

# Registry is reachable
curl -s http://localhost:5000/v2/_catalog
```

---

## Step 2: Install Istio

```bash
bash hack/install-istio.sh
```

Expected output (excerpt):
```
[install-istio] Installing Istio demo profile (version X.Y.Z) ...
[install-istio] Waiting for Istio pods to become ready ...
[install-istio] Done. Istio is ready in namespace istio-system.
```

### Verify Step 2

```bash
# All Istio pods are Running
kubectl get pods -n istio-system

# Istio CRDs are installed
kubectl get crd | grep istio.io
```

By default `hack/install-istio.sh` also installs the observability add-ons automatically
(see Step 3 below). Pass `SKIP_ADDONS=true` to skip them.

---

## Step 3: Verify the Observability Stack

After `hack/install-istio.sh` completes, Prometheus, Grafana, Jaeger, and Kiali are all
deployed in `istio-system`:

```bash
kubectl get deployments -n istio-system
```

Expected output includes:
```
NAME                   READY   UP-TO-DATE   AVAILABLE
grafana                1/1     1            1
istiod                 1/1     1            1
jaeger                 1/1     1            1
kiali                  1/1     1            1
prometheus             1/1     1            1
```

### Access the Dashboards

Each command opens the respective UI in your browser via `istioctl` port-forward.
Keep the terminal open while using the dashboard.

| Dashboard  | Command                           | URL                       |
|------------|-----------------------------------|---------------------------|
| Kiali      | `istioctl dashboard kiali`        | http://localhost:20001    |
| Grafana    | `istioctl dashboard grafana`      | http://localhost:3000     |
| Jaeger     | `istioctl dashboard jaeger`       | http://localhost:16686    |
| Prometheus | `istioctl dashboard prometheus`   | http://localhost:9090     |

**Kiali** is the primary tool for watching tier-label routing live. After deploying the
`config/samples/` from feature 002, open the Kiali graph view and enable **traffic
animation** to see requests flow between `high-priority-pods` and `standard-pods` subsets.

### What to look for in Kiali

1. Navigate to **Graph → Namespace: default**.
2. Set the display to show **Request rate** and **Traffic animation**.
3. Send traffic with and without the `user-type: premium` header — the highlighted paths
   will switch between the high and standard subsets accordingly.
4. The **`tier` label** appears on each workload node in the sidebar under **Labels**.

---

## Step 5: Test the Local Registry

Build and push a test image, then confirm it is pullable inside the cluster:

```bash
# Build and push to local registry
docker build -t localhost:5000/mesh-priority-controller:dev .
docker push localhost:5000/mesh-priority-controller:dev

# Run a pod that pulls from the local registry
kubectl run test-pull \
  --image=localhost:5000/mesh-priority-controller:dev \
  --restart=Never \
  --command -- sleep 30

kubectl get pod test-pull
kubectl delete pod test-pull
```

---

## Teardown

To remove the local environment when you are done:

```bash
bash hack/teardown.sh
```

This deletes the kind cluster and stops the registry container. All local state is lost.

---

## Defaults and Customization

| Variable        | Default         | Override example                                    |
|-----------------|-----------------|-----------------------------------------------------|
| `CLUSTER_NAME`  | `istio-qos`     | `CLUSTER_NAME=my-cluster bash hack/bootstrap.sh`    |
| `REGISTRY_PORT` | `5000`          | `REGISTRY_PORT=5001 bash hack/bootstrap.sh`         |
| `ISTIO_VERSION` | (pinned in file)| `ISTIO_VERSION=1.25.0 bash hack/install-istio.sh`      |
| `ISTIO_PROFILE` | `demo`          | `ISTIO_PROFILE=minimal bash hack/install-istio.sh`     |
| `SKIP_ADDONS`   | (unset)         | `SKIP_ADDONS=true bash hack/install-istio.sh`          |

---

## Troubleshooting

**Port 5000 already in use**:
```bash
lsof -i :5000
# Kill the process or use REGISTRY_PORT=5001 bash hack/bootstrap.sh
```

**Docker not running**:
```bash
# Start Docker Desktop or:
sudo systemctl start docker
```

**Istio pods stuck in Pending**:
```bash
kubectl describe pod -n istio-system <pod-name>
# Typically a resource constraint on a low-memory machine
```
