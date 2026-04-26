# Data Model: Kind Development Environment Setup

**Feature**: 001-kind-istio-setup
**Date**: 2026-04-26

## Entities

### Kind Cluster

The local Kubernetes cluster created and managed by the `kind` tool.

| Attribute        | Value / Description                                      |
|------------------|----------------------------------------------------------|
| Name             | `istio-qos` (configurable via `CLUSTER_NAME` variable)  |
| Kubernetes ver.  | Controlled by kind node image; pinned in `bootstrap.sh` |
| Nodes            | 1 control-plane node (single-node dev setup)            |
| CNI              | kind default (kindnet)                                   |
| Registry mirror  | `localhost:5000` → `http://kind-registry:5000`          |

**Lifecycle**:
- Created by `hack/bootstrap.sh`
- Destroyed by `hack/teardown.sh`
- Re-entrant: `bootstrap.sh` skips creation if cluster already exists

---

### Local Container Registry

A Docker container running a plain HTTP registry on the host, connected to the kind cluster's
containerd runtime.

| Attribute        | Value / Description                                         |
|------------------|-------------------------------------------------------------|
| Container name   | `kind-registry`                                             |
| Image            | `registry:2`                                                |
| Host port        | `5000`                                                      |
| Push address     | `localhost:5000/<image>:<tag>`                              |
| Pull address     | `localhost:5000/<image>:<tag>` (inside cluster pods)        |
| Network          | Connected to the `kind` Docker network for cluster access   |

**Lifecycle**:
- Started by `hack/bootstrap.sh`
- Stopped/removed by `hack/teardown.sh`
- Re-entrant: `bootstrap.sh` skips creation if container already exists

---

### Istio Installation

The Istio service mesh control plane and data plane configuration deployed inside the kind
cluster.

| Attribute             | Value / Description                                       |
|-----------------------|-----------------------------------------------------------|
| Install profile       | `demo`                                                    |
| Version variable      | `ISTIO_VERSION` (pinned at top of `install-istio.sh`)     |
| Install namespace     | `istio-system`                                            |
| Sidecar injection     | Automatic, per namespace label                            |
| Key CRDs installed    | `DestinationRule`, `VirtualService`, `PeerAuthentication` |
| Ingress gateway       | Included (demo profile)                                   |

**Lifecycle**:
- Installed by `hack/install-istio.sh` into an existing kind cluster
- Uninstalled as part of cluster teardown (`hack/teardown.sh` deletes the cluster)
- Re-entrant: `install-istio.sh` skips installation if `istio-system` namespace exists

---

### `hack/` Directory

The repository directory containing all setup scripts.

| File                   | Purpose                                              |
|------------------------|------------------------------------------------------|
| `hack/bootstrap.sh`     | Create kind cluster + local registry (P1)            |
| `hack/install-istio.sh` | Install Istio service mesh into the cluster (P2)     |
| `hack/teardown.sh`      | Delete the kind cluster and stop the registry        |

---

### README.md

The repository root documentation file.

| Section                  | Content                                               |
|--------------------------|-------------------------------------------------------|
| Prerequisites            | Docker, kind, kubectl, istioctl — with version notes  |
| Setup sequence           | Step-by-step commands in order                        |
| Script reference         | What each script does and expected output             |
| Verification commands    | Commands to confirm each step completed correctly     |
| Teardown                 | How to clean up the local environment                 |

---

## Validation Rules

- `CLUSTER_NAME` must be a valid kind cluster name (lowercase alphanumeric + hyphens).
- Registry port `5000` must be free on the host before `bootstrap.sh` runs; the script
  validates this and emits a human-readable error if the port is occupied.
- `ISTIO_VERSION` must be set to a non-empty string; the script validates this before
  attempting download.

## State Transitions

```
[Host: Docker running]
        │
        ▼ hack/bootstrap.sh
[kind cluster created + registry running]
        │
        ▼ hack/install-istio.sh
[Istio mesh installed and ready]
        │
        ▼ (develop / test controller)
        │
        ▼ hack/teardown.sh
[kind cluster deleted + registry stopped]
```
