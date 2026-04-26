# mesh-priority-controller

A Kubernetes controller that labels pods with a tier classification
(`tier: high | standard`) derived from `PodLabelerPolicy` custom resources matching on
container image patterns. The labels are consumed by Istio `DestinationRule` subsets,
which a `VirtualService` uses to route premium customer traffic to the high tier and
standard traffic to the standard tier.

The controller is the **source of truth for traffic-tier classification** in the mesh.

---

## Prerequisites

Install the following tools before running any script:

| Tool | Purpose | Install |
|------|---------|---------|
| Docker | Run kind nodes and the local registry | https://docs.docker.com/get-docker/ |
| `kind` | Create local Kubernetes clusters | https://kind.sigs.k8s.io/docs/user/quick-start/#installation |
| `kubectl` | Interact with the cluster | https://kubernetes.io/docs/tasks/tools/ |
| `istioctl` v1.24.2 | Install the Istio service mesh | https://istio.io/latest/docs/setup/install/istioctl/ |

Verify all tools are available:

```bash
docker info
kind version
kubectl version --client
istioctl version --remote=false
```

---

## Local Development Setup

### Step 1 — Bootstrap the Kind Cluster and Local Registry

```bash
bash hack/bootstrap.sh
```

Expected output:

```
[bootstrap] Starting local registry 'kind-registry' on port 5000 ...
[bootstrap] Creating kind cluster 'istio-qos' ...
[bootstrap] Connecting registry to 'kind' network ...
[bootstrap] Done.
[bootstrap]   Cluster : istio-qos
[bootstrap]   Registry: localhost:5000
```

**Verify:**

```bash
kubectl get nodes
# NAME                      STATUS   ROLES           AGE
# istio-qos-control-plane   Ready    control-plane   ...

curl http://localhost:5000/v2/_catalog
# {"repositories":[]}
```

---

### Step 2 — Install Istio Service Mesh

```bash
bash hack/install-istio.sh
```

Expected output:

```
[install-istio] Installing Istio 1.24.2 (profile: demo) ...
[install-istio] Waiting for Istio pods to become ready (timeout: 300s) ...
[install-istio] Done. Istio 1.24.2 is ready.
```

**Verify:**

```bash
kubectl get pods -n istio-system
# All pods should show Running or Completed status

kubectl get crd | grep istio.io
# Lists DestinationRule, VirtualService, PeerAuthentication, etc.
```

---

### Step 3 — Use the Local Registry

Build and push the controller image, then deploy it into the cluster:

```bash
# Build and push to the local registry
docker build -t localhost:5000/mesh-priority-controller:dev .
docker push localhost:5000/mesh-priority-controller:dev

# Pods inside the cluster can pull from localhost:5000
kubectl run test-pull \
  --image=localhost:5000/mesh-priority-controller:dev \
  --restart=Never \
  --command -- sleep 30

kubectl get pod test-pull
kubectl delete pod test-pull
```

---

## Script Reference

All scripts live in the `hack/` directory. Each script is idempotent — running it a
second time on an already-configured environment exits 0 and makes no changes.

| Script | Purpose | Exit codes |
|--------|---------|------------|
| `hack/bootstrap.sh` | Create kind cluster + local registry | 0 success, 1 prereq fail, 2 create fail |
| `hack/install-istio.sh` | Install Istio service mesh | 0 success, 1 prereq fail, 2 install fail, 3 timeout |
| `hack/teardown.sh` | Delete cluster and stop registry | 0 success, 1 delete fail |

### Customization via Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CLUSTER_NAME` | `istio-qos` | Kind cluster name |
| `REGISTRY_PORT` | `5000` | Host port for the local registry |
| `REGISTRY_NAME` | `kind-registry` | Docker container name for the registry |
| `ISTIO_VERSION` | `1.24.2` | Istio version to install |
| `ISTIO_PROFILE` | `demo` | istioctl install profile |
| `READY_TIMEOUT` | `300` | Seconds to wait for Istio pods to become ready |

Example override:

```bash
CLUSTER_NAME=my-cluster REGISTRY_PORT=5001 bash hack/bootstrap.sh
```

---

## Teardown

To remove the local environment when you are done:

```bash
bash hack/teardown.sh
```

This deletes the kind cluster and stops the registry container. **All in-cluster state
is lost.**

---

## Troubleshooting

**Port 5000 already in use:**

```bash
lsof -i :5000
# Kill the conflicting process, or use:
REGISTRY_PORT=5001 bash hack/bootstrap.sh
```

**Docker not running:**

```bash
sudo systemctl start docker   # Linux
# Or start Docker Desktop on macOS
```

**Istio pods stuck in Pending:**

```bash
kubectl describe pod -n istio-system <pod-name>
# Usually a resource constraint — ensure your machine has at least 4 GB RAM free
```

**`istioctl` not found:**

```bash
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.24.2 sh -
export PATH="$PWD/istio-1.24.2/bin:$PATH"
```

---

## Enable Sidecar Injection

To have Istio automatically inject the Envoy proxy into pods in a namespace:

```bash
kubectl label namespace <your-namespace> istio-injection=enabled
```

Pods created after labeling will receive the sidecar automatically.
