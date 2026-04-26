# Implementation Plan: Live Kubernetes Cluster Debugging via kmcp MCP

**Branch**: `004-kmcp-k8s-debug` | **Date**: 2026-04-26 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specs/004-kmcp-k8s-debug/spec.md`

## Summary

Build a live debugging environment for the three Act I podlabeler bugs. The workflow starts
with failing envtest tests (act1 baseline), uses a kmcp-backed MCP server to give Claude Code
live visibility into a running kind cluster, and provides per-bug Claude Code skills that
identify the observable symptom, locate the exact code defect, apply the fix in an `act2/`
directory, rebuild and redeploy the controller image, and confirm the previously-failing test
now passes.

**Bug→Fix→Test cycle (the core of this feature)**:

```
go test act1/ → FAIL (3 bugs)
         ↓
MCP tools → Claude observes live cluster evidence
         ↓
/debug-bugN skill → applies fix to act2/, rebuilds, redeploys
         ↓
go test act2/ → PASS (same test, fixed code)
```

## Technical Context

**Language/Version**: Go 1.22 (controller), Python 3.11 (MCP server)
**Primary Dependencies**:
- MCP server: `fastmcp`, `kubernetes` Python client
- Controller (act2): `controller-runtime` v0.19, `k8s.io/api` v0.32
- Cluster: `kind` v0.27+, `kubectl`, `docker`
**Storage**: N/A (no persistent storage outside the Kubernetes API)
**Testing**: `envtest` (Go) for controller tests; manual MCP tool call verification for server
**Target Platform**: Ubuntu/WSL + kind cluster (local development machine)
**Project Type**: CLI tooling + local MCP server + Kubernetes controller
**Performance Goals**: MCP tool response < 5 s; full debug session (all 3 bugs) < 20 min
**Constraints**: Read-only kubeconfig access for the MCP server; no cluster-admin; kind only
**Scale/Scope**: Single-developer demo environment; kind cluster with 1-3 nodes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

This feature produces two categories of output: (1) the MCP server + skills (tooling), and
(2) the `act2/` fixed controller. Constitution gates apply primarily to act2.

| Principle | Applies | Status |
|---|---|---|
| I. Label Correctness | act2 controller | ✓ SSA fix ensures both labels always persist |
| II. Label Stability | act2 controller | ✓ diff-gate added to act2 reconciler (skip no-op writes) |
| III. Policy-Driven Classification | act2 controller | ✓ no logic change, same policy evaluation |
| IV. Fleet-Safe Engineering | act2 controller | ✓ all 4 envtest tests PASS; lint required |
| V. Observability | act2 controller | ✓ structured JSON logs retained; metrics out of scope for act2 demo |

**Quality Gates for act2** (constitution §Quality Gates):
- `go test ./controller/... -v` — all 4 tests PASS
- `go vet ./...` — 0 issues
- `golangci-lint run` — 0 issues (if golangci-lint available; else `go vet` only)
- Diff-gate test: one test confirms label already present → no API write (in TestCoreLabeling extension)

**MCP server + skills** (tooling layer, not controller code): constitution does not impose quality
gates on Python tooling. The skills are markdown instructions, not compiled code.

## Project Structure

### Documentation (this feature)

```text
specs/004-kmcp-k8s-debug/
├── plan.md              ← this file
├── research.md          ← technology decisions (Phase 0)
├── data-model.md        ← entities and fix recipes (Phase 1)
├── quickstart.md        ← end-to-end walkthrough (Phase 1)
└── tasks.md             ← task breakdown (/speckit-tasks output)
```

### Source Code (repository root)

```text
kmcp-server/                    ← NEW: Python MCP server
├── server.py                   ← FastMCP server with 6 k8s tools
├── requirements.txt            ← fastmcp, kubernetes
├── Makefile                    ← start, stop, setup targets
└── rbac/
    ├── role.yaml               ← ClusterRole: podlabeler-debug-reader
    └── rolebinding.yaml        ← ClusterRoleBinding for local user

act2/                           ← NEW: Fixed controller (Act II)
├── main.go                     ← Bug 3 fix: LeaderElection: true
├── go.mod                      ← module github.com/knabben/istio-poc, go 1.22
├── go.sum
├── Dockerfile                  ← multi-stage Go image build
├── Makefile                    ← build, docker-build, kind-load, deploy, test targets
├── api/
│   └── v1alpha1/
│       ├── types.go            ← copied verbatim from act1 (no change)
│       └── register.go         ← copied verbatim from act1 (no change)
├── controller/
│   ├── reconciler.go           ← Bug 1+2 fixes applied
│   └── reconciler_test.go      ← same 4 tests, all expected to PASS
└── manifests/
    ├── crd.yaml                ← copied verbatim from act1
    ├── rbac.yaml               ← copied verbatim from act1
    ├── controller.yaml         ← replicas:1, LeaderElection enabled
    ├── sample-policies.yaml    ← copied verbatim from act1
    ├── sample-workload.yaml    ← copied verbatim from act1
    └── istio/
        ├── destinationrule.yaml
        └── virtualservice.yaml

.claude/skills/                 ← NEW: per-bug debug+fix skills
├── debug-bug1.md               ← detect lost update, fix, redeploy, verify
├── debug-bug2.md               ← detect stale cache, fix, redeploy, verify
├── debug-bug3.md               ← detect missing Lease, fix, redeploy, verify
└── debug-podlabeler-all.md     ← run all three in sequence
```

## Bug Fix Specifications

### Bug 1 — Lost Update (act1/controller/reconciler.go:121)

**Buggy code**:
```go
if err := r.Update(ctx, pod); err != nil {
    logger.Error(err, "failed to update pod")
    return ctrl.Result{}, err
}
```

**Fixed code** (act2/controller/reconciler.go):
```go
// Build a minimal SSA patch object carrying only the labels we own.
patch := &corev1.Pod{
    TypeMeta: metav1.TypeMeta{
        APIVersion: "v1",
        Kind:       "Pod",
    },
    ObjectMeta: metav1.ObjectMeta{
        Name:      pod.Name,
        Namespace: pod.Namespace,
        Labels:    desired,
    },
}
if err := r.Patch(ctx, patch, client.Apply,
    client.ForceOwnership, client.FieldOwner("podlabeler")); err != nil {
    logger.Error(err, "failed to patch pod labels")
    return ctrl.Result{}, err
}
```

**Why it fixes it**: Server-side apply merges label fields at the API server level using field
ownership. Two concurrent writers with different label keys no longer conflict — each owns its
own fields. No `resourceVersion` conflict occurs; both label writes survive.

---

### Bug 2 — Stale Cache / Terminal NotFound (act1/controller/reconciler.go:44–62)

**Buggy code**:
```go
if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
    return ctrl.Result{}, err          // BUG: propagates NotFound as terminal
}
```

**Fixed code** (act2/controller/reconciler.go):
```go
if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
    if apierrors.IsNotFound(err) {
        return ctrl.Result{}, nil      // benign — cache miss or pod deleted
    }
    return ctrl.Result{}, err
}
```

**Why it fixes it**: A `NotFound` from the informer cache means the pod was deleted or the
cache hasn't propagated the create yet. Returning `nil` lets the workqueue discard the stale
request cleanly. Non-NotFound errors (API server down, etc.) still propagate for retry.

---

### Bug 3 — No Leader Election (act1/main.go:69)

**Buggy code**:
```go
LeaderElection: false,
```

**Fixed code** (act2/main.go):
```go
LeaderElection:                true,
LeaderElectionID:              "podlabeler.knabben.dev",
LeaderElectionNamespace:       "kube-system",
LeaderElectionReleaseOnCancel: true,
```

**Why it fixes it**: With leader election enabled, only one controller replica holds the
Lease and reconciles at a time. The second replica stands by. Bug 1's concurrent-write
race is no longer triggered by multi-replica deployment.

---

## Skill Specification: per-bug debug+fix+verify

Each skill in `.claude/skills/debug-bugN.md` follows this template:

```
1. Run the failing test (act1) to confirm the bug is present.
2. Deploy the act1 buggy controller to the local kind cluster.
3. Trigger the bug condition (create pods, wait for reconciliation).
4. Use MCP tools to observe the live cluster evidence.
5. Locate the exact defect in act1/controller/reconciler.go (or main.go).
6. Apply the fix to the corresponding act2/ file.
7. Build the act2 Docker image and load it into kind.
8. Redeploy the controller using act2/manifests/.
9. Rerun the test (act2) and confirm it PASSES.
10. Summarize: symptom observed, defect location, fix applied, test result.
```

Each skill file includes:
- The exact MCP tool calls to run (copy-pasteable)
- The exact `git diff`-style patch to apply to act2/
- The exact `make` commands to build and deploy
- The exact `go test` command and expected PASS output

---

## MCP Server Specification

### Tool Definitions (server.py)

```python
@mcp.tool()
def list_pods(namespace: str = "default") -> str:
    """List all pods in namespace with their labels and phase."""

@mcp.tool()
def get_pod(name: str, namespace: str = "default") -> str:
    """Get full pod spec including labels, status, and events."""

@mcp.tool()
def list_pod_logs(pod_name: str, namespace: str = "default",
                  container: str = "", tail_lines: int = 100) -> str:
    """Fetch recent log lines from a pod container."""

@mcp.tool()
def list_events(namespace: str = "default", field_selector: str = "") -> str:
    """List events in namespace, optionally filtered by field selector."""

@mcp.tool()
def list_leases(namespace: str = "default") -> str:
    """List coordination.k8s.io Lease objects."""

@mcp.tool()
def list_podlabelerpolicies(namespace: str = "") -> str:
    """List PodLabelerPolicy resources (cluster-scoped)."""
```

### Claude Code Registration

```bash
claude mcp add --transport stdio podlabeler-debug -- \
  python /path/to/kmcp-server/server.py
```

Or via `.claude/settings.json`:
```json
{
  "mcpServers": {
    "podlabeler-debug": {
      "command": "python",
      "args": ["/path/to/kmcp-server/server.py"],
      "type": "stdio"
    }
  }
}
```

---

## Complexity Tracking

No constitution violations. The only non-standard element is the Python MCP server alongside
a Go-only repository. Justified: the MCP server is tooling (not controller code); FastMCP
in Python is the fastest path to the 6 required read-only tools and does not compete with
the Go controller module.
