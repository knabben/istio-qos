# Research: Pod Tier Label Controller

**Feature**: 002-pod-label-controller
**Date**: 2026-04-26

## Decision 1: Kubebuilder Version & Scaffolding

**Decision**: Kubebuilder v4 (current stable: v4.x, Go 1.22+). Scaffold a cluster-scoped
`PodLabelerPolicy` CRD in group `mesh`, version `v1alpha1`:

```bash
kubebuilder init --domain knabben.github.com --repo github.com/knabben/istio-qos
kubebuilder create api --group mesh --version v1alpha1 --kind PodLabelerPolicy \
  --namespaced=false --resource --controller
```

The `--namespaced=false` flag generates the `//+kubebuilder:resource:scope=Cluster` marker
and omits namespace from the CRD scope.

**Rationale**: Kubebuilder v4 is the active release line. Cluster-scoped CRD matches FR-001
(controller watches pods in all namespaces).

**Alternatives considered**: Manual marker injection post-scaffold — works but the CLI flag
is canonical and reduces drift.

---

## Decision 2: Server-Side Apply for Label Writes

**Decision**: Use `client.Patch` with `client.Apply` and a fixed field manager name
`"mesh-priority-controller"`. The apply patch is constructed as a minimal `corev1.Pod`
object carrying only the `tier` label (no other fields), so unrelated fields are never
touched.

```go
apply := &corev1.Pod{
    TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
    ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace,
                    Labels: map[string]string{"tier": tier}},
}
err := r.Patch(ctx, apply, client.Apply,
    client.FieldOwner("mesh-priority-controller"),
    client.ForceOwnership)
```

For label removal, the apply patch omits the `tier` key entirely; Kubernetes removes fields
not present in an apply payload that were previously owned by this manager.

**Rationale**: Server-side apply with field ownership satisfies FR-002 (no lost updates).
The API server detects conflicts rather than the client, eliminating TOCTOU races.

**Alternatives considered**:
- Strategic merge patch: no field ownership, silently overwrites concurrent changes.
- Three-way merge client-side: requires storing last-applied annotation, fragile.

---

## Decision 3: Glob Pattern Matching

**Decision**: Use `github.com/gobwas/glob` for image pattern matching. Patterns are compiled
once at policy reconcile time and cached; matching is performed per container image during
pod reconcile.

```go
g, err := glob.Compile(policy.Spec.ImagePattern)
if g.Match(containerImage) { ... }
```

**Rationale**: `filepath.Match` / `path.Match` don't handle image patterns correctly
(e.g., `*/myapp:v1.*` — the `/` separator semantics differ). `gobwas/glob` compiles
patterns to a DFA and is well-maintained.

**Alternatives considered**:
- `filepath.Match`: mishandles registry prefixes with `/`.
- `doublestar`: supports `**` which is unnecessary for image names; heavier dependency.
- Regex: more powerful but increases operator error surface; glob is sufficient for v1.

---

## Decision 4: Cross-Type Watch (Policy → Pods)

**Decision**: Register two watches in the controller builder:
1. Primary watch on `Pod` (enqueues the pod's namespaced name on any pod event).
2. Secondary watch on `PodLabelerPolicy` using `handler.EnqueueRequestsFromMapFunc` that
   lists all pods cluster-wide and enqueues a reconcile request for each.

This means every `PodLabelerPolicy` create/update/delete triggers a reconcile for every
pod — acceptable for v1 given the small scale of the kind dev environment. A field index on
pod container images can be added in v2 to scope the fan-out.

**Rationale**: Simple, correct, and avoids premature optimization. The pod-level reconciler
is idempotent, so redundant enqueues are harmless.

**Alternatives considered**:
- Field index on pod images: more efficient but adds complexity; deferred to v2.
- Reconciling the policy (not pods): requires iterating pods inside the policy reconciler,
  duplicating logic and making the reconciler harder to test.

---

## Decision 5: Integration Tests with envtest

**Decision**: Use `sigs.k8s.io/controller-runtime/pkg/envtest` with binaries managed by
`setup-envtest` (`make envtest`). Tests live in `internal/controller/suite_test.go` (Ginkgo
suite) and `internal/controller/*_test.go`.

```go
testEnv = &envtest.Environment{
    CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
    BinaryAssetsDirectory: os.Getenv("KUBEBUILDER_ASSETS"),
}
```

**Rationale**: kubebuilder scaffolds this pattern by default; it runs without a live cluster
and satisfies SC-006.

**Alternatives considered**:
- Kind cluster for integration tests: slower, requires Docker; used for e2e only.
- `USE_EXISTING_CLUSTER=true`: non-reproducible across CI environments.

---

## Decision 6: Leader Election

**Decision**: Set `LeaderElection: true` and `LeaderElectionID: "mesh-priority-controller.knabben.github.com"`
in `ctrl.Options`. The controller validates this at startup and refuses to start if the
value is overridden to `false` (FR-006). No CLI flag or env var exposes this setting.

**Rationale**: Hardcoding leader election as non-configurable prevents accidental disabling
in dev/test configs, which is a common source of concurrent write bugs.

**Alternatives considered**:
- Flag-driven: exposes `--leader-elect=false` — forbidden by FR-006.
- Operator-configured via ConfigMap: unnecessarily complex for a boolean invariant.

---

## Decision 7: Project Structure

**Decision**: Standard kubebuilder v4 layout with additions:

```
.
├── api/v1alpha1/             # PodLabelerPolicy types + deepcopy
├── cmd/main.go               # Entry point, manager setup
├── config/
│   ├── crd/                  # CRD manifests (generated)
│   ├── rbac/                 # ClusterRole, ClusterRoleBinding
│   ├── manager/              # Deployment, leader election lease
│   └── samples/              # Reference example (FR-008)
├── hack/
│   ├── bootstrap.sh          # Kind cluster setup (feature 001)
│   ├── install-istio.sh      # Istio install (feature 001)
│   ├── teardown.sh           # Teardown (feature 001)
│   └── test-policy.sh        # NEW: apply PodLabelerPolicy and verify labeling
├── internal/controller/      # Reconciler implementation
└── internal/matcher/         # Glob pattern matching (pure, easily testable)
```

The `internal/matcher` package isolates glob matching so it can be unit-tested without any
Kubernetes machinery.
