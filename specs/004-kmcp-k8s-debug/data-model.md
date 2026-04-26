# Data Model: Live Kubernetes Cluster Debugging via kmcp MCP

**Feature**: `004-kmcp-k8s-debug`
**Date**: 2026-04-26

---

## Entities

### MCP Server

The local Python process that bridges Claude Code to the Kubernetes cluster.

| Field | Type | Description |
|---|---|---|
| `transport` | `stdio` | Communication mode with Claude Code |
| `tools` | `list[Tool]` | The six Kubernetes inspection functions exposed |
| `kubeconfig_path` | `string` | Path to the scoped kubeconfig file |
| `namespace` | `string` | Default namespace for queries (`default`) |

**Relationships**: exposes → Tool; uses → KubeconfigRole; connects to → KindCluster

---

### Tool

One callable function exposed by the MCP server, invokable by Claude Code.

| Field | Type | Description |
|---|---|---|
| `name` | `string` | Tool identifier (`list_pods`, `get_pod`, etc.) |
| `description` | `string` | Natural-language description used by Claude to decide when to call the tool |
| `parameters` | `dict` | Input schema (e.g., `namespace`, `pod_name`, `container`) |
| `returns` | `string` | JSON or plain-text response |

**Tools defined**: `list_pods`, `get_pod`, `list_pod_logs`, `list_events`, `list_leases`, `list_podlabelerpolicies`

---

### KubeconfigRole

The RBAC configuration granting the MCP server read-only access to cluster resources.

| Field | Type | Description |
|---|---|---|
| `kind` | `ClusterRole` | Kubernetes RBAC resource type |
| `name` | `string` | `podlabeler-debug-reader` |
| `rules` | `list[PolicyRule]` | API groups, resources, verbs (see research.md Decision 4) |
| `binding` | `ClusterRoleBinding` | Binds the role to the service account running the MCP server |

---

### BugEvidence

The observable cluster artifacts that prove each bug exists. Collected by the MCP tools.

| Field | Type | Description |
|---|---|---|
| `bug_id` | `int` | 1, 2, or 3 |
| `symptom` | `string` | Human-readable description of what Claude observes |
| `tool_calls` | `list[string]` | MCP tools invoked to surface the evidence |
| `evidence_type` | `enum` | `pod_labels`, `controller_logs`, `events`, `lease_list` |
| `expected_value` | `string` | What healthy behavior looks like |
| `actual_value` | `string` | What the buggy behavior produces |

**Bug Evidence Map**:

| Bug | Evidence Type | MCP Tool | What Claude Sees |
|---|---|---|---|
| Bug 1 — Lost update | `pod_labels` | `get_pod` | Pod has only one of two expected policy labels |
| Bug 2 — Stale cache | `controller_logs` | `list_pod_logs` | `pods "X" not found` errors in controller log |
| Bug 3 — No Lease | `lease_list` | `list_leases` | Empty lease list in `default` namespace |

---

### FixRecipe

The documented instructions for fixing each bug — the content of each Claude Code skill.

| Field | Type | Description |
|---|---|---|
| `bug_id` | `int` | 1, 2, or 3 |
| `bug_name` | `string` | Human-readable bug label |
| `test_name` | `string` | `TestBug1_LostUpdate`, `TestBug2_StaleCache`, `TestBug3_NoLease` |
| `file` | `string` | File containing the buggy code (relative path in act1/) |
| `line_range` | `string` | Approximate line range of the bug |
| `buggy_pattern` | `string` | The specific code construct that is wrong |
| `fixed_pattern` | `string` | The replacement code |
| `act2_file` | `string` | Corresponding file in act2/ where the fix is applied |
| `verify_command` | `string` | `go test` invocation that should PASS after the fix |

**Fix Recipe Table**:

| Bug | Buggy File:Line | Buggy Pattern | Fixed Pattern | Test |
|---|---|---|---|---|
| Bug 1 | `act1/controller/reconciler.go` ~L60 | `r.Update(ctx, pod)` | `r.Patch(ctx, pod, client.Apply, ...)` | `TestBug1_LostUpdate` |
| Bug 2 | `act1/controller/reconciler.go` ~L40 | `return ctrl.Result{}, err` | `if errors.IsNotFound(err) { return nil }` | `TestBug2_StaleCache` |
| Bug 3 | `act1/main.go` ~L30 | `LeaderElection: false` | `LeaderElection: true` | `TestBug3_NoLease` |

---

### KindCluster

The local Kubernetes cluster used for live debugging.

| Field | Type | Description |
|---|---|---|
| `name` | `string` | `istio-qos` (from specs/001 setup) |
| `istio_installed` | `bool` | `true` (pre-condition per specs/001) |
| `controller_image` | `string` | `podlabeler:act1` (buggy) or `podlabeler:act2` (fixed) |
| `namespaces` | `list[string]` | `default`, `istio-system` |

---

### DebugSession

One end-to-end run of the debug-fix-verify workflow (executed by a skill).

| Field | Type | Description |
|---|---|---|
| `bug_id` | `int` | Which bug is being debugged (1, 2, or 3) |
| `steps` | `list[string]` | Ordered: run test → inspect cluster → apply fix → rebuild → redeploy → rerun test |
| `initial_test_result` | `FAIL` | Always FAIL at start (act1 has the bugs) |
| `final_test_result` | `PASS` | Expected after skill applies the fix |
| `mcp_tools_used` | `list[string]` | Tools invoked during the MCP diagnosis step |
