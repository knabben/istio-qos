# Research: PodLabeler Bug Baseline (Act I)

**Feature**: 003-podlabeler-bug-baseline
**Date**: 2026-04-26

---

## Decision 1: Directory Location

**Decision**: Extract `bugs/files.zip` into `bugs/podlabeler-act1/` as a standalone Go module.

**Rationale**: Keeping the Act I code under `bugs/` makes its role explicit — it is a
teaching artifact, not a production component. A separate directory from the main
`istio-qos` module avoids `go.mod` conflicts and allows the module to pin its own
older dependency versions (controller-runtime v0.17.3) without affecting the main project.

**Alternatives considered**:
- Sub-package inside main module: rejected — dependency version conflicts; `go.mod` would
  need to satisfy both sets of constraints.
- Separate repository: rejected — adds friction for presenters who clone one repo and
  need both Act I and Act II side by side.

---

## Decision 2: Go Module Path

**Decision**: Preserve `github.com/knabben/istio-poc` from `bugs/files.zip`.

**Rationale**: The module path is referenced in import paths inside the source files
(`reconciler.go`, `main.go`). Changing it requires editing every import — which changes
the bug baseline. Preserving it keeps the code identical to what was shown as Act I.

**Alternatives considered**:
- Rename to `github.com/knabben/istio-qos/bugs/act1`: rejected — requires touching every
  import statement, violating the "faithful copy" constraint.

---

## Decision 3: Dependency Versions

**Decision**: Use the exact versions from `bugs/files.zip` go.mod:
- `k8s.io/api v0.29.3`
- `k8s.io/apimachinery v0.29.3`
- `k8s.io/client-go v0.29.3`
- `sigs.k8s.io/controller-runtime v0.17.3`

**Rationale**: The bugs are logic errors, not version-specific issues. Pinning original
versions ensures the code compiles and behaves identically to the talk baseline. Adding
`sigs.k8s.io/controller-runtime/pkg/envtest` as a test dependency uses the same module
(v0.17.3), so no new major dependency is introduced.

**Alternatives considered**:
- Upgrade to current controller-runtime: rejected — might mask or change bug behavior;
  also contradicts the "faithful copy" constraint.

---

## Decision 4: envtest Setup Without kubebuilder

**Decision**: Use `controller-runtime/pkg/envtest` directly with `CRDDirectoryPaths`
pointing to `manifests/crd.yaml`. No `setup-envtest` binary or kubebuilder tooling.

**Rationale**: The hand-written `crd.yaml` already exists in the zip. envtest can load
CRD YAML files directly via `envtest.Environment{CRDDirectoryPaths: []string{"../manifests"}}`.
This avoids any kubebuilder dependency and keeps the setup consistent with the "raw
controller-runtime only" constraint.

**Test binary location**: envtest requires Kubernetes API server and etcd binaries. The
test adds a `TestMain` that calls `envtest.Environment.Start()`. Binaries are fetched
via `go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest use -p path` in
a `Makefile` target — this is a test tooling concern, not a kubebuilder dependency.

**Alternatives considered**:
- Mock client only (no envtest): rejected — Bug 1 (concurrent write conflict) and Bug 3
  (Lease creation) require a real API server to reproduce.
- kind cluster: rejected — too heavy for CI; envtest is lighter and deterministic.

---

## Decision 5: "Expected to Fail" Test Pattern

**Decision**: Each test asserts the **correct** behavior. Against the buggy code, the
assertion fails — proving the bug. No special `t.ExpectFailure` or build-tag isolation
is used for the test file itself.

**Rationale**: Standard Go test assertion failures (`t.Fatal`, `require.Equal`, etc.) are
the clearest way to show a bug: the test output reads "FAIL: expected both labels, got
only one" which is self-documenting. CI is configured to expect these three tests to fail
on the Act I branch and pass on the Act II branch.

**How CI handles it**: The Makefile `test-bugs` target runs with `|| true` so the
pipeline records the failure without blocking. A companion `test-bugs-expected-failures`
target counts expected failures and asserts exactly 3 fail (not 0, not 4).

**Alternatives considered**:
- `//go:build bugs` tag: considered but rejected — hides the tests in normal `go test ./...`
  runs, making it less visible to presenters.
- `t.Skip("Known bug")`: rejected — skip ≠ fail; the point is to SHOW the failure, not hide it.
- Separate package with `_test` suffix only: compatible with the chosen approach; test file
  lives in `controller/reconciler_test.go` using `package controller_test`.

---

## Decision 6: Bug 1 Test Design (Concurrent Write)

**Decision**: Inject two reconcile calls on the same pod simultaneously using goroutines
inside envtest. Both calls write a different label key (`policy-a: applied` and
`policy-b: applied`). After both complete, assert the pod carries both labels.

**Rationale**: envtest runs a real API server, so concurrent writes genuinely race. The
buggy `r.Update()` uses the pod's `resourceVersion` snapshot from the cache — whichever
goroutine writes second wins; the first writer's label is silently lost.

**How it fails on Act I**: Only one of the two labels survives. `require.Equal` fails with
a clear message: "expected {policy-a: applied, policy-b: applied}, got {policy-b: applied}".

---

## Decision 7: Bug 2 Test Design (Stale Cache NotFound)

**Decision**: Call `reconciler.Reconcile(ctx, req)` directly with a `NamespacedName` for
a pod that was never created in envtest. The cache returns `NotFound`. Assert `err == nil`.

**Rationale**: Direct reconciler invocation bypasses the workqueue and manager, isolating
the `r.Get()` error path precisely. No pod creation is needed — the test is purely about
the error return.

**How it fails on Act I**: The reconciler returns the `NotFound` error. `require.NoError`
fails with: "expected no error, got: pods 'nonexistent-pod' not found".

---

## Decision 8: Bug 3 Test Design (No Lease)

**Decision**: Start the manager in a goroutine, wait 2 seconds for startup, then
`kubectl get lease -n podlabeler-system`. Assert a `Lease` object exists.

**Rationale**: Lease creation is the observable side-effect of `LeaderElection: true`.
Its absence is the exact symptom of Bug 3. A 2-second wait is sufficient for the manager
to create the Lease if election is enabled.

**How it fails on Act I**: No Lease object exists in the namespace. The `List` call returns
an empty list. `require.Len(t, leases.Items, 1)` fails: "expected 1, got 0".

---

## Decision 9: Manifests Preservation

**Decision**: Copy all manifests from `bugs/files.zip` verbatim, including `controller.yaml`
with `replicas: 2` and no `LeaderElection` flag (because `main.go` hardcodes `false`).

**Rationale**: The manifests are part of the teaching demo — the presenter applies them to
a kind cluster to show the live symptom of Bug 3 (Lease oscillation visible via `kubectl
get lease --watch`). Changing the manifests would change the demo.

**Alternatives considered**:
- Fix the manifests but keep the code broken: rejected — the spec requires faithful
  reproduction of the entire Act I baseline.
