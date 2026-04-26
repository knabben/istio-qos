# Feature Specification: Kind Development Environment Setup

**Feature Branch**: `001-kind-istio-setup`
**Created**: 2026-04-26
**Status**: Draft
**Input**: User description: "the application must have an initial testing development with kind,
must have installation scripts in a REC folder to bootstrap the cluster with a local registry
and install Istio in the sequence. There should be documentation on README.md on how to use
those scripts to make the procedure easy."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Bootstrap Kind Cluster with Local Registry (Priority: P1)

A developer contributing to `mesh-priority-controller` needs a reproducible local Kubernetes
environment to test the controller end-to-end without relying on a shared cluster or cloud
infrastructure. Running a single script from the `hack/` directory creates a `kind` cluster
with a local container registry configured and reachable from inside the cluster.

**Why this priority**: Without a local cluster and registry, developers cannot build, push,
and deploy the controller locally. This is the foundational step for all local testing.

**Independent Test**: Can be fully tested by running the bootstrap script alone and verifying
that `kubectl get nodes` reports a healthy node and that pushing a container image to the local
registry succeeds from the host.

**Acceptance Scenarios**:

1. **Given** a developer has Docker and `kind` installed, **When** they run `hack/bootstrap.sh`,
   **Then** a `kind` cluster is created, a local container registry is started, and the
   registry is accessible from inside the cluster.
2. **Given** the cluster is running, **When** the developer pushes an image to the local
   registry, **Then** the image is pullable from a pod running inside the cluster.
3. **Given** the script is run a second time on an already-created cluster, **When** the
   developer re-runs `hack/bootstrap.sh`, **Then** the script detects the existing cluster
   and registry and exits cleanly without error.

---

### User Story 2 - Install Istio into the Cluster (Priority: P2)

After the kind cluster is running, a developer needs Istio installed and ready so that
`DestinationRule` and `VirtualService` resources work correctly for testing tier-based
traffic routing. A single script from the `hack/` directory installs Istio into the cluster
in a configuration compatible with the controller's mesh requirements.

**Why this priority**: Istio is the mesh layer that consumes the tier labels the controller
produces. Without Istio, end-to-end traffic routing tests cannot be exercised.

**Independent Test**: Can be fully tested by running the Istio install script after the
cluster is up and verifying that all Istio control plane components reach Running state.

**Acceptance Scenarios**:

1. **Given** a kind cluster created by `hack/bootstrap.sh` is running, **When** the developer
   runs `hack/install-istio.sh`, **Then** the Istio control plane is installed and all
   components reach Ready state within a reasonable timeout.
2. **Given** Istio is installed, **When** a namespace is labeled for sidecar injection,
   **Then** pods in that namespace receive an Envoy sidecar automatically.
3. **Given** the install script is run against a cluster that already has Istio installed,
   **When** the developer re-runs `hack/install-istio.sh`, **Then** the script detects the
   existing installation and exits without re-installing or breaking existing state.

---

### User Story 3 - Follow README Documentation for Full Setup (Priority: P3)

A developer new to the project needs clear, step-by-step documentation in `README.md` that
explains how to use the scripts in `hack/` to go from zero to a fully configured local
development environment (kind cluster + registry + Istio) without prior context about the
project.

**Why this priority**: Without documentation the scripts are opaque. Good documentation
lowers the barrier to contribution and ensures the setup is reproducible across team members.

**Independent Test**: Can be fully tested by having a developer unfamiliar with the project
follow the README instructions from scratch and reach a working local environment with no
out-of-band assistance.

**Acceptance Scenarios**:

1. **Given** a developer reads `README.md`, **When** they follow the documented prerequisites
   and script execution order, **Then** they can complete the full setup (cluster + registry
   + Istio) without needing to consult external references.
2. **Given** the README documents prerequisites, **When** a developer checks if their machine
   meets the requirements, **Then** they can verify all required tools are installed before
   running any script.
3. **Given** the README documents the expected outcome of each script, **When** a developer
   runs each script in sequence, **Then** they can confirm each step completed correctly
   using the verification commands documented in the README.

---

### Edge Cases

- What happens when Docker is not running when a script is executed?
- What happens if the required port for the local registry is already bound on the host?
- What happens if a prior partial setup left stale kind resources that conflict with a fresh
  bootstrap?
- What happens if network access to download Istio manifests is unavailable?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The project MUST include a `hack/bootstrap.sh` script that creates a `kind`
  cluster and a local container registry configured to be reachable from within the cluster.
- **FR-002**: The project MUST include a `hack/install-istio.sh` script that installs the
  full Istio service mesh (control plane + data plane sidecar injection) into an existing
  kind cluster. The installed mesh MUST support `DestinationRule`, `VirtualService`, and
  automatic sidecar injection, which are required for tier-based traffic routing.
- **FR-003**: Both scripts MUST be idempotent: running them multiple times on an
  already-configured environment MUST NOT produce errors or corrupt existing state.
- **FR-004**: Both scripts MUST validate prerequisites (Docker running, required CLI tools
  present) and emit a clear, human-readable error message if a prerequisite is absent,
  rather than failing mid-execution with a cryptic error.
- **FR-005**: Scripts MUST exit with a non-zero status code on any failure so that
  developers and CI pipelines can detect failures programmatically.
- **FR-006**: The project MUST include a `README.md` at the repository root documenting:
  prerequisites, execution order of scripts, expected outcome of each step, and verification
  commands to confirm each step completed correctly.
- **FR-007**: The `README.md` MUST document the exact commands a developer must run to
  complete the full setup sequence from an empty machine to a running kind cluster with
  local registry and Istio.

### Key Entities

- **Kind Cluster**: A local Kubernetes cluster running inside Docker, used as the test target
  for the controller and its mesh configuration.
- **Local Registry**: A container image registry running on the host, connected to the kind
  cluster so that locally-built controller images are pullable from inside the cluster.
- **Istio Installation**: The Istio service mesh control plane deployed inside the kind
  cluster, providing the `DestinationRule` and `VirtualService` support needed for tier-based
  routing tests.
- **`hack/` Directory**: The repository directory containing all setup and installation scripts
  for the local development environment.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer following `README.md` from scratch can reach a fully operational
  local environment (kind cluster + registry + Istio) in under 15 minutes on a machine that
  meets the documented prerequisites.
- **SC-002**: Running the bootstrap and Istio install scripts a second time on an
  already-configured environment completes without errors in under 60 seconds.
- **SC-003**: 100% of prerequisite-validation checks emit human-readable error messages when
  a prerequisite is absent — no script fails silently or produces a cryptic error.
- **SC-004**: All Istio system components reach Ready state within 5 minutes of the install
  script completing on a standard developer machine.

## Assumptions

- "cursor" in the original description is a typo for "cluster".
- "really.me" in the original description is a typo for "README.md".
- "REC folder" refers to a `hack/` directory at the repository root containing shell scripts.
- Scripts target Linux and macOS with Bash; Windows support is out of scope for v1.
- Docker is a hard prerequisite and must be running before any script is executed.
- The Istio version installed by the script will be pinned (documented in the script and
  README) for reproducibility across developer machines.
- The local registry will bind to a default port; configuring a non-default port is out
  of scope for v1.
- Network access to download kind, istioctl, and Istio manifests is assumed on the
  developer's machine during setup.
