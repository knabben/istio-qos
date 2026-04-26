# Quickstart: Live Kubernetes Debugging with kmcp MCP

**Feature**: `004-kmcp-k8s-debug`
**Time to complete**: ~20 minutes end-to-end

---

## Prerequisites

- kind cluster running with Istio (see `specs/001-kind-istio-setup`)
- Docker available
- Python 3.11+ with pip
- Claude Code CLI installed and authenticated

---

## Step 1 — Set Up the MCP Server (5 min)

```bash
cd kmcp-server
pip install -r requirements.txt

# Apply read-only RBAC for the MCP server
kubectl apply -f rbac/role.yaml
kubectl apply -f rbac/rolebinding.yaml

# Register the MCP server in Claude Code
claude mcp add --transport stdio podlabeler-debug -- \
  python $(pwd)/server.py
```

Verify:
```bash
# In Claude Code, ask: "list all pods in the default namespace"
# Claude should call list_pods() and return cluster state.
```

---

## Step 2 — Deploy the Broken Controller (3 min)

```bash
cd act1
make docker-build kind-load   # builds podlabeler:act1, loads into kind
kubectl apply -f manifests/crd.yaml
kubectl apply -f manifests/rbac.yaml
kubectl apply -f manifests/controller.yaml   # replicas:2, LeaderElection:false
kubectl apply -f manifests/sample-policies.yaml
kubectl apply -f manifests/sample-workload.yaml
```

Verify:
```bash
kubectl get pods -n default      # controller pods Running
kubectl get podlabelerpolicies   # policies listed
```

---

## Step 3 — Confirm Tests Fail (1 min)

```bash
cd act1
KUBEBUILDER_ASSETS=/home/amimk/.local/share/kubebuilder-envtest/k8s/1.35.0-linux-amd64 \
  go test ./controller/... -v -timeout 60s || true
# Expected: 1 PASS (TestCoreLabeling), 3 FAIL (TestBug1_LostUpdate, TestBug2_StaleCache, TestBug3_NoLease)
```

---

## Step 4 — Debug Bug 1: Lost Update (3 min)

Run the skill in Claude Code:
```
/debug-bug1
```

The skill will:
1. Show the failing test output (`TestBug1_LostUpdate` in act1)
2. Call `list_pods` → pod has only one of two expected labels
3. Call `list_events` → 409 Conflict events from the controller
4. Point to `act1/controller/reconciler.go` ~line 83 — `r.Update(ctx, pod)`
5. Apply the SSA patch (already in `act2/controller/reconciler.go`)
6. Run `make docker-build kind-load` in `act2/`, then rollout restart
7. Run `go test ./controller/... -run TestBug1_Fixed` in `act2/` → PASS

---

## Step 5 — Debug Bug 2: Stale Cache (3 min)

```
/debug-bug2
```

The skill will:
1. Show the failing test output (`TestBug2_StaleCache` in act1)
2. Call `list_pod_logs` → `pods "X" not found` error lines in controller log
3. Point to `act1/controller/reconciler.go` ~lines 38-41 — bare `return ctrl.Result{}, err`
4. The `IsNotFound` guard is already in `act2/controller/reconciler.go` lines 44-46
5. Rebuild and redeploy act2
6. Run `go test ./controller/... -run TestBug2_Fixed` in `act2/` → PASS

---

## Step 6 — Debug Bug 3: Missing Lease (3 min)

```
/debug-bug3
```

The skill will:
1. Show the failing test output (`TestBug3_NoLease` in act1)
2. Call `list_leases` → empty list in `kube-system` (no podlabeler Lease)
3. Point to `act1/main.go` ~line 51 — `ctrl.NewManager()` with no LeaderElection field
4. `LeaderElection: true` is already set in `act2/main.go` lines 61-64
5. Rebuild and redeploy act2; verify `list_leases namespace=kube-system` returns the Lease
6. Run `go test ./controller/... -run TestBug3_Fixed` in `act2/` → PASS

---

## Step 7 — Verify All Tests Pass (1 min)

```bash
cd act2
KUBEBUILDER_ASSETS=/home/amimk/.local/share/kubebuilder-envtest/k8s/1.35.0-linux-amd64 \
  go test ./controller/... -v -timeout 60s
# Expected: 4 PASS, 0 FAIL
# --- PASS: TestCoreLabeling
# --- PASS: TestBug1_Fixed
# --- PASS: TestBug2_Fixed
# --- PASS: TestBug3_Fixed
```

---

## Full Session (all 3 bugs at once)

```
/debug-podlabeler-all
```

Runs Step 4–6 in sequence and ends with the full test suite PASS confirmation.

---

## MCP Tool Reference

| Tool | Purpose | Example prompt |
|---|---|---|
| `list_pods` | See all pod labels | "Show me all pods and their labels" |
| `get_pod` | Full pod detail | "Describe the bug1-pod" |
| `list_pod_logs` | Controller errors | "Show controller logs for the last 50 lines" |
| `list_events` | 409 Conflicts | "Show warning events in default namespace" |
| `list_leases` | Leader election | "Are there any Lease objects in the cluster?" |
| `list_podlabelerpolicies` | Policy state | "What policies are active?" |
