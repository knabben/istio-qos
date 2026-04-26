# Script Contracts: Kind Development Environment Setup

**Feature**: 001-kind-istio-setup
**Date**: 2026-04-26

These contracts define the public interface of each script — arguments, environment
variables, exit codes, and stdout/stderr conventions. They are the external-facing
contract that `README.md` and CI pipelines rely on.

---

## rec/bootstrap.sh

**Purpose**: Create a kind cluster with a local container registry connected to it.

### Environment Variables (Optional Overrides)

| Variable        | Default       | Description                                    |
|-----------------|---------------|------------------------------------------------|
| `CLUSTER_NAME`  | `istio-qos`   | Name of the kind cluster to create             |
| `REGISTRY_PORT` | `5000`        | Host port for the local container registry     |
| `REGISTRY_NAME` | `kind-registry` | Docker container name for the registry       |

### Positional Arguments

None.

### Behaviour

1. Validates prerequisites: Docker is running, `kind` binary is in `$PATH`,
   `kubectl` binary is in `$PATH`.
2. Checks if registry container `$REGISTRY_NAME` already exists; skips creation if so.
3. Starts registry container on `$REGISTRY_PORT`.
4. Checks if kind cluster `$CLUSTER_NAME` already exists; skips creation if so.
5. Creates kind cluster with containerd mirror config pointing to the registry.
6. Connects the registry container to the kind Docker network.
7. Prints a success summary to stdout.

### Exit Codes

| Code | Meaning                                              |
|------|------------------------------------------------------|
| `0`  | Success (or all resources already existed — no-op)  |
| `1`  | Prerequisite check failed (with human-readable message to stderr) |
| `2`  | kind cluster or registry creation failed            |

### Stdout / Stderr

- Progress messages: stdout, prefixed with `[bootstrap]`
- Error messages: stderr, human-readable, prefixed with `[bootstrap] ERROR:`

### Idempotency Guarantee

Running `rec/bootstrap.sh` a second time on an already-configured environment
exits with code `0` and makes no changes.

---

## rec/install-istio.sh

**Purpose**: Install the Istio service mesh into an existing kind cluster.

### Environment Variables (Optional Overrides)

| Variable         | Default          | Description                                       |
|------------------|------------------|---------------------------------------------------|
| `CLUSTER_NAME`   | `istio-qos`      | Name of the kind cluster to install Istio into    |
| `ISTIO_VERSION`  | (pinned in file) | Istio version to install (e.g. `1.24.2`)          |
| `ISTIO_PROFILE`  | `demo`           | istioctl install profile                          |

### Positional Arguments

None.

### Behaviour

1. Validates prerequisites: `kubectl` is in `$PATH`, kind cluster `$CLUSTER_NAME`
   is reachable, `istioctl` is in `$PATH` or downloaded on demand.
2. Checks if `istio-system` namespace already exists; skips installation if so.
3. Runs `istioctl install --set profile=$ISTIO_PROFILE --skip-confirmation`.
4. Waits for all Istio control plane pods in `istio-system` to reach `Ready` state
   (timeout: 5 minutes).
5. Prints a success summary to stdout.

### Exit Codes

| Code | Meaning                                                 |
|------|---------------------------------------------------------|
| `0`  | Success (or Istio already installed — no-op)            |
| `1`  | Prerequisite check failed (with human-readable message) |
| `2`  | istioctl installation failed                            |
| `3`  | Timeout waiting for Istio pods to become ready          |

### Stdout / Stderr

- Progress messages: stdout, prefixed with `[install-istio]`
- Error messages: stderr, human-readable, prefixed with `[install-istio] ERROR:`

### Idempotency Guarantee

Running `rec/install-istio.sh` a second time exits with code `0` and makes no changes
if Istio is already installed.

---

## rec/teardown.sh

**Purpose**: Delete the kind cluster and stop the local registry container.

### Environment Variables (Optional Overrides)

| Variable        | Default         | Description                                |
|-----------------|-----------------|--------------------------------------------|
| `CLUSTER_NAME`  | `istio-qos`     | Name of the kind cluster to delete         |
| `REGISTRY_NAME` | `kind-registry` | Docker container name for the registry     |

### Positional Arguments

None.

### Behaviour

1. Deletes kind cluster `$CLUSTER_NAME` (no-op if not found).
2. Stops and removes registry container `$REGISTRY_NAME` (no-op if not found).
3. Prints a success summary to stdout.

### Exit Codes

| Code | Meaning                                           |
|------|---------------------------------------------------|
| `0`  | Success (cluster and/or registry removed or absent) |
| `1`  | Deletion failed                                   |

### Idempotency Guarantee

Running `rec/teardown.sh` when no cluster or registry exists exits with code `0`.
