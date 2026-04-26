# Research: Live Kubernetes Cluster Debugging via kmcp MCP

**Feature**: `004-kmcp-k8s-debug`
**Date**: 2026-04-26

---

## Decision 1 — MCP Server Technology

**Decision**: Python 3.11 with FastMCP as the MCP server framework.

**Rationale**: kmcp is a deployment platform (CRD + controller) for MCP servers in Kubernetes, not a pre-built tool library. The MCP server itself is user-authored code deployed into the cluster (or run locally) that exposes custom tools. FastMCP is the fastest path to a working server: it uses Python decorators (`@mcp.tool()`) to expose any function as an MCP tool, has strong community adoption, and the Kubernetes Python client (`kubernetes`) is a first-class, well-documented way to read cluster state. Go is also viable but yields slower iteration for a tooling layer.

**Alternatives considered**:
- TypeScript + `@modelcontextprotocol/sdk`: more boilerplate, stronger typing but slower to iterate for a tooling layer.
- Go + `mcp-go`: natural fit for a Go-centric repo but no significant advantage for a thin cluster-query layer.
- Using a pre-built Kubernetes MCP server (e.g., `mcp-server-kubernetes`): evaluated but requires an external dependency with less control over the exact tools exposed for this bug-diagnosis workflow.

---

## Decision 2 — MCP Server Deployment Mode

**Decision**: Run the MCP server as a local process (stdio transport), not as a Kubernetes Deployment. Register it in Claude Code's MCP config via `claude mcp add`.

**Rationale**: The debugging workflow is developer-local. Running the server as a local Python process using stdio transport avoids the overhead of building a container image and deploying it through kmcp's controller — that lifecycle is useful for production MCP servers, not for a dev-debugging tool. The local process connects to the kind cluster via the developer's kubeconfig.

**Alternatives considered**:
- Full kmcp Deployment (MCPServer CRD): correct approach for production, unnecessary overhead for local debugging.
- HTTP transport: adds complexity without benefit for a single-developer workflow.

---

## Decision 3 — Kubernetes Tools Exposed by the MCP Server

**Decision**: The MCP server exposes exactly the tools needed to diagnose the three bugs and no more:

| Tool name | API call | Purpose |
|---|---|---|
| `list_pods` | `core/v1 pods list` | Observe pod labels (Bug 1 evidence) |
| `get_pod` | `core/v1 pods get` | Read full pod state including label map |
| `list_pod_logs` | `core/v1 pods/log` | Read controller logs (Bug 2 error messages) |
| `list_events` | `core/v1 events list` | Observe warning events from failed reconciles |
| `list_leases` | `coordination.k8s.io/v1 leases list` | Check for Lease objects (Bug 3 evidence) |
| `list_podlabelerpolicies` | `labeling.knabben.dev/v1alpha1 podlabelerpolicies` | Inspect policies matched against pods |

**Rationale**: Minimal surface — only read operations, only the resource types relevant to the three bugs. Follows the spec's requirement for least-privilege access.

---

## Decision 4 — Required kubeconfig RBAC Permissions

**Decision**: A dedicated `ClusterRole` scoped to the minimum verbs needed for diagnosis:

```yaml
rules:
- apiGroups: [""]
  resources: ["pods", "events"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["labeling.knabben.dev"]
  resources: ["podlabelerpolicies"]
  verbs: ["get", "list", "watch"]
```

No `create`, `update`, `patch`, or `delete` verbs are granted. Cluster-admin is never used. This satisfies SC-003 (minimum permissions, verifiable by inspection).

**Alternatives considered**:
- Using existing cluster-admin kubeconfig: faster setup but violates the spec's least-privilege requirement and the constitution's security stance.
- Namespace-scoped Role (not ClusterRole): insufficient since controller logs and policies may span namespaces.

---

## Decision 5 — Fixed Controller Location (Act II)

**Decision**: The fixed controller lives in a new `act2/` directory, parallel to `act1/`. The Act I baseline is never modified — it is the reference buggy implementation.

**Rationale**: The debugging workflow starts with act1 tests FAILING. The skill applies fixes to produce act2. Keeping act1 pristine preserves the bug baseline for the envtest suite and for the presentation demo ("before and after").

**Act II fixes (one per bug)**:

| Bug | File | Buggy code | Fixed code |
|---|---|---|---|
| Bug 1 — Lost update | `controller/reconciler.go` | `r.Update(ctx, pod)` | `r.Patch(ctx, pod, client.Apply, client.ForceOwnership, client.FieldOwner("podlabeler"))` with SSA object |
| Bug 2 — Stale cache | `controller/reconciler.go` | `return ctrl.Result{}, err` (after Get NotFound) | `if errors.IsNotFound(err) { return ctrl.Result{}, nil }` |
| Bug 3 — No Lease | `main.go` | `LeaderElection: false` | `LeaderElection: true` + `LeaderElectionNamespace: "kube-system"` |

**Alternatives considered**:
- Patching act1 in place: destroys the bug baseline.
- A single `act2/` that applies all 3 fixes at once: loses the ability to show each fix independently.

---

## Decision 6 — Skill Structure

**Decision**: Three Claude Code skills — one per bug — each following the same template:
1. Run the failing test to confirm the bug.
2. Use MCP tools to inspect live cluster evidence.
3. Apply the specific code fix (points to exact file:line).
4. Rebuild the controller image and load it into kind.
5. Redeploy the controller.
6. Rerun the test to confirm it now PASSES.

A fourth combined skill (`/debug-podlabeler-all`) runs all three in sequence.

**Rationale**: Per-bug skills let the user demonstrate each fix independently (useful for teaching). The combined skill gives the full end-to-end demo path.

---

## Decision 7 — Image Build and Deployment

**Decision**: Build a Docker image for the controller, load it into the kind cluster with `kind load docker-image`, and apply the manifests with `kubectl apply`. No external registry is required.

**Rationale**: kind's built-in image loading (`kind load docker-image`) is the standard local development pattern. It avoids registry credential management and keeps the setup self-contained.

**Alternatives considered**:
- `ko` (Go image builder): elegant for Go but adds a dependency and requires registry or ko-kind integration.
- `skaffold`: adds abstraction overhead for a one-time demo workflow.
