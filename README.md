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
| `istioctl` v1.29.0 | Install the Istio service mesh | https://istio.io/latest/docs/setup/install/istioctl/ |

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
[install-istio] Installing Istio 1.29.0 (profile: demo) ...
[install-istio] Waiting for Istio pods to become ready (timeout: 300s) ...
[install-istio] Installing observability add-ons (Prometheus → Grafana → Jaeger → Kiali) ...
[install-istio]   Installing prometheus ...
[install-istio]   Installing grafana ...
[install-istio]   Installing jaeger ...
[install-istio]   Installing kiali ...
[install-istio] Waiting for Kiali to be ready (timeout: 180s) ...
[install-istio] Setup complete. Istio 1.29.0 is ready.
```

**Verify:**

```bash
kubectl get pods -n istio-system
# All pods — including grafana, jaeger, kiali, prometheus — should show Running

kubectl get crd | grep istio.io
# Lists DestinationRule, VirtualService, PeerAuthentication, etc.
```

---

### Step 3 — Access Observability Dashboards

The following commands each open a dashboard in your browser via `istioctl` port-forward.
Keep the terminal open while using the dashboard.

| Dashboard  | Command                           | URL                        | What it shows |
|------------|-----------------------------------|----------------------------|---------------|
| **Kiali**  | `istioctl dashboard kiali`        | http://localhost:20001     | Service mesh topology, tier routing, traffic animation |
| Grafana    | `istioctl dashboard grafana`      | http://localhost:3000      | Istio metrics: request rate, error rate, latency |
| Jaeger     | `istioctl dashboard jaeger`       | http://localhost:16686     | Distributed request traces across mesh hops |
| Prometheus | `istioctl dashboard prometheus`   | http://localhost:9090      | Raw Prometheus query UI |

**Kiali walkthrough** — after deploying the `config/samples/` resources:

1. Open Kiali → **Graph → Namespace: default**
2. Enable **Traffic Animation** from the Display menu
3. Send traffic with and without `user-type: premium` header — the active path switches
   between `high-priority-pods` and `standard-pods` subsets
4. Click any workload to see its `tier` label in the sidebar

---

### Step 5 — Use the Local Registry

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
| `ISTIO_VERSION` | `1.29.0` | Istio version to install |
| `ISTIO_PROFILE` | `demo` | istioctl install profile |
| `READY_TIMEOUT` | `300` | Seconds to wait for Istio pods to become ready |
| `SKIP_ADDONS` | (unset) | Set to `true` to skip Prometheus/Grafana/Jaeger/Kiali install |

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
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.29.0 sh -
export PATH="$PWD/istio-1.29.0/bin:$PATH"
```

**Kiali add-on fails to install (network timeout):**

```bash
# Retry the add-ons step only (Istio control plane already installed):
bash hack/install-istio.sh
# The script skips the control plane re-install and retries any missing add-ons.

# Or skip add-ons entirely on a resource-constrained machine:
SKIP_ADDONS=true bash hack/install-istio.sh
```

**Kiali shows no graph / "No namespaces found":**

```bash
# Label the namespace for sidecar injection so Kiali can observe traffic:
kubectl label namespace default istio-injection=enabled
# Restart existing pods to pick up the sidecar:
kubectl rollout restart deployment -n default
```

---

## Enable Sidecar Injection

To have Istio automatically inject the Envoy proxy into pods in a namespace:

```bash
kubectl label namespace <your-namespace> istio-injection=enabled
```

Pods created after labeling will receive the sidecar automatically.
