# Feature Specification: Live Kubernetes Cluster Debugging via kmcp MCP

**Feature Branch**: `004-kmcp-k8s-debug`
**Created**: 2026-04-26
**Status**: Draft
**Input**: User description: "using kmcp a new MCP server can be created, the tool existent is to access the kubernetes cluster, review what kind of access via kubeconfig are necessary to debug the errors from podlabeler-bug, the procedure of this feature is starting the cluster, broken controller, and use the MCP and claude code to debug the 3 errors live"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Configure kmcp MCP Server with Cluster Access (Priority: P1)

A developer sets up the kmcp MCP server and connects it to their local Kubernetes cluster so that Claude Code gains live visibility into cluster resources. The developer reviews and applies the minimum kubeconfig permissions required for the MCP server to inspect pods, controller logs, events, and coordination objects.

**Why this priority**: Without a working MCP-to-cluster connection, no live debugging is possible. This is the foundational prerequisite for all subsequent stories.

**Independent Test**: Can be fully tested by starting the MCP server, connecting Claude Code to it, and verifying that Claude Code can list pods and read events in the `default` namespace — without deploying the broken controller.

**Acceptance Scenarios**:

1. **Given** a local cluster is running, **When** the kmcp MCP server is started with the documented kubeconfig configuration, **Then** Claude Code can query cluster resources (pods, events, leases, logs) through the MCP tools.
2. **Given** the MCP server is running, **When** Claude Code issues a cluster inspection request, **Then** the response arrives within 5 seconds and contains accurate live cluster data.
3. **Given** the kubeconfig is scoped to the documented minimum permissions, **When** the MCP server attempts an operation outside that scope, **Then** the request is denied and the error is surfaced to Claude Code.

---

### User Story 2 - Start Cluster and Deploy Broken Controller (Priority: P2)

A developer provisions a local Kubernetes cluster, installs Istio, and deploys the Act I podlabeler controller (the intentionally buggy baseline from `act1/`). Supporting resources — the CRD, RBAC, sample policies, and workload pods — are also applied so that the three bugs are reproducible in a live environment.

**Why this priority**: The broken controller must be running in the cluster before any live debugging can begin. This story creates the observable failure conditions.

**Independent Test**: Can be fully tested by verifying that the controller pods are running, the CRD is installed, and at least one sample pod exists — even before connecting Claude Code to the cluster.

**Acceptance Scenarios**:

1. **Given** a fresh local cluster, **When** the setup procedure is followed, **Then** the podlabeler controller is running with `replicas: 2` and the PodLabelerPolicy CRD is installed.
2. **Given** the controller is deployed with `LeaderElection: false`, **When** sample workload pods are created, **Then** both controller replicas begin reconciling the same pods simultaneously (Bug 3 precondition visible in logs).
3. **Given** a sample pod matching a policy image pattern exists, **When** the controller reconciles it, **Then** a label-loss event (Bug 1) or a NotFound error (Bug 2) is observable in controller logs or pod state.

---

### User Story 3 - Diagnose All Three Bugs via Claude Code and MCP (Priority: P3)

With the MCP server connected and the broken controller running, a developer uses Claude Code to interactively investigate each of the three known bugs. Claude Code queries live cluster state — pod labels, controller logs, lease objects, events — and identifies the observable symptom, root cause, and location in the code for each bug without requiring manual `kubectl` commands from the developer.

**Why this priority**: This is the primary value of the feature — demonstrating that an AI assistant with live cluster access can autonomously diagnose distributed-systems bugs that are otherwise hard to observe.

**Independent Test**: Can be fully tested by prompting Claude Code to "investigate why pods are not correctly labeled" and verifying it produces a diagnosis identifying all three bugs from cluster evidence alone.

**Acceptance Scenarios**:

1. **Given** the broken controller is running and the MCP is connected, **When** Claude Code is asked to investigate pod labeling behavior, **Then** it identifies the lost-update symptom (one label missing after concurrent writes) by inspecting pod label history and controller logs.
2. **Given** the controller is running, **When** Claude Code inspects error events and logs, **Then** it identifies the stale-cache bug (NotFound errors treated as terminal) and traces it to the reconciler's error propagation path.
3. **Given** the controller is deployed with `replicas: 2`, **When** Claude Code lists Lease objects in the cluster, **Then** it identifies the absence of a leader-election Lease and explains the consequence (both replicas reconcile independently).
4. **Given** a complete debugging session, **When** Claude Code has investigated all three symptoms, **Then** it produces a summary report naming each bug, its observable cluster evidence, and a one-line fix recommendation.

---

### Edge Cases

- What happens when the cluster is not reachable when Claude Code issues an MCP tool call? The MCP server must return a clear error rather than hanging.
- What happens when the kubeconfig token has expired mid-session? The failure must surface to Claude Code with a descriptive message.
- What happens if the controller crashes before Bug 2 is observable? The deployment should be restartable without re-running the full setup.
- What happens if both controller replicas update the same pod simultaneously before Claude Code can observe the intermediate state? Logs must be retained long enough for post-hoc diagnosis.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A local Kubernetes MCP server MUST be configurable using kmcp with the minimum kubeconfig permissions necessary to read pod state, labels, events, controller logs, and coordination leases.
- **FR-002**: The required kubeconfig RBAC permissions MUST be documented (list of API groups, resources, and verbs) so they can be applied to any cluster without granting cluster-admin.
- **FR-003**: A local cluster MUST be provisionable via a documented single-command or short script procedure that installs the CRD, RBAC, and deploys the buggy controller from `act1/manifests/`.
- **FR-004**: The MCP server MUST expose tools sufficient for Claude Code to: list pods and their labels, retrieve controller pod logs, list events in a namespace, and list coordination Lease objects.
- **FR-005**: Claude Code MUST be able to identify all three bug symptoms (label loss, NotFound error propagation, missing Lease) from live cluster data without requiring the developer to run any manual `kubectl` commands.
- **FR-006**: The end-to-end debugging session (cluster start → controller deploy → MCP connect → diagnose 3 bugs) MUST be completable within 20 minutes by following documented steps.
- **FR-007**: The MCP server configuration MUST be stored in the repository so any developer can reproduce the debugging environment without manual configuration.

### Key Entities

- **MCP Server**: The kmcp process that mediates between Claude Code and the Kubernetes API; configured with cluster endpoint and credentials.
- **kubeconfig**: Credential and permission file scoped to the minimum RBAC rules needed for read-only cluster inspection.
- **Buggy Controller**: The Act I podlabeler deployment (`act1/manifests/`) running with `replicas: 2` and `LeaderElection: false`.
- **Debugging Session**: An interactive Claude Code conversation in which the AI uses MCP tools to inspect cluster state and produce a bug diagnosis.
- **Bug Evidence**: Observable cluster artifacts — pod label state, controller log lines, event records, Lease list — that prove each bug exists.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer with no prior kmcp experience can have the MCP server connected to a live cluster within 5 minutes of following the setup documentation.
- **SC-002**: Claude Code identifies all three bug symptoms from cluster evidence without the developer issuing any manual cluster commands during the debugging session.
- **SC-003**: The required kubeconfig permissions are scoped to the minimum necessary — no cluster-admin or wildcard rules — and are verifiable by inspection.
- **SC-004**: A complete debugging session (start to final diagnosis) completes within 20 minutes end-to-end.
- **SC-005**: The debugging environment is fully reproducible by any developer on a fresh machine by following the documented steps — zero manual configuration steps beyond the documented procedure.

## Assumptions

- The local cluster is provisioned with kind (consistent with `specs/001-kind-istio-setup`); minikube or other local runtimes are out of scope for v1.
- Istio is already installed in the cluster per the setup from `specs/001-kind-istio-setup`; this feature does not re-install Istio.
- The Act I buggy controller source is available in `act1/` and its manifests are in `act1/manifests/`; no modifications to controller code are made in this feature.
- Claude Code is running locally with the kmcp MCP server configured as an MCP provider; cloud-hosted Claude instances are out of scope.
- The debugging goal is diagnosis only — identifying and explaining each bug — not fixing the controller code (that is Act II scope).
- Read-only cluster access is sufficient for all three bug diagnoses; no write operations through the MCP are required.
- The developer has Docker and Go available for cluster provisioning and controller image building.
