# Debug All Podlabeler Bugs — Full Detect → Fix → Verify Workflow

This skill walks through detecting, fixing, redeploying, and verifying all three
podlabeler bugs in sequence using the MCP server tools and the per-bug skills.

## Prerequisites

1. Kind cluster `istio-qos` is running with the act1 (buggy) controller deployed.
2. MCP server is registered and reachable: `.claude/settings.json` lists `podlabeler-debug`.
3. `act1/` and `act2/` directories are present.
4. `KUBEBUILDER_ASSETS` is set for envtest runs.

Quick sanity check:
```
list_pods namespace=podlabeler-system
list_podlabelerpolicies
```

Expected: controller pod in Running state, at least one PodLabelerPolicy present.

---

## Bug 1 — Lost Update (SSA fix)

Follow: `.claude/skills/debug-bug1.md`

Quick summary:
1. **Detect**: `list_pod_logs` — look for `"conflict"` / `"failed to update pod"`
2. **Fix**: replace `r.Update(ctx, pod)` with `r.Patch(ctx, patch, client.Apply, ...)`
   - File: `act1/controller/reconciler.go` ~line 83
3. **Deploy**: `make docker-build kind-load` in `act1/`, then `kubectl rollout restart`
4. **Verify**: `cd act2 && make test` — `TestBug1_Fixed PASS`

---

## Bug 2 — Stale Cache (NotFound guard)

Follow: `.claude/skills/debug-bug2.md`

Quick summary:
1. **Detect**: `list_pod_logs` — look for `"not found"` errors for deleted pods
2. **Fix**: wrap `r.Get()` error with `apierrors.IsNotFound` guard returning nil
   - File: `act1/controller/reconciler.go` ~lines 38-41
3. **Deploy**: `make docker-build kind-load` in `act1/`, then `kubectl rollout restart`
4. **Verify**: `cd act2 && make test` — `TestBug2_Fixed PASS`

---

## Bug 3 — No Leader Election (Lease check)

Follow: `.claude/skills/debug-bug3.md`

Quick summary:
1. **Detect**: `list_leases namespace=kube-system` — no `podlabeler.*` Lease found
2. **Fix**: add `LeaderElection: true` block to `ctrl.NewManager()` in `act1/main.go`
   - File: `act1/main.go` ~lines 51-65
3. **Deploy**: `make docker-build kind-load`, then `kubectl apply -f manifests/controller.yaml`
4. **Verify**: `cd act2 && make test` — `TestBug3_Fixed PASS`

---

## Final Verification

After all three bugs are fixed:

```bash
cd act2
make test
```

Expected output:
```
--- PASS: TestCoreLabeling
--- PASS: TestBug1_Fixed
--- PASS: TestBug2_Fixed
--- PASS: TestBug3_Fixed
PASS
```

Also verify the live cluster:
```
list_pods namespace=default
```

All pods matching a PodLabelerPolicy should show the correct `tier` label without
any log errors in the controller.
