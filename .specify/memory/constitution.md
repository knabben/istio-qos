<!--
SYNC IMPACT REPORT
Version change: [template placeholders] → 1.0.0 (initial ratification)
Modified principles: All — first concrete population of the template
Added sections:
  - Core Principles (I. Label Correctness, II. Label Stability,
    III. Policy-Driven Classification, IV. Fleet-Safe Engineering,
    V. Observability)
  - Operational Constraints
  - Quality Gates
  - Governance
Removed sections: None (template placeholders replaced)
Templates requiring updates:
  - .specify/templates/plan-template.md ✅ no structural change required;
    Constitution Check section already defers to this file
  - .specify/templates/spec-template.md ✅ no change required
  - .specify/templates/tasks-template.md ✅ no change required
Follow-up TODOs: None
-->

# mesh-priority-controller Constitution

## Core Principles

### I. Label Correctness

The controller is the authoritative source of truth for traffic-tier classification in the
mesh. Every managed pod MUST carry exactly one tier label with a value of `high` or
`standard`. A missing or incorrect label is a traffic-routing failure: premium customer
traffic will land on standard-tier pods. Classification MUST be deterministic given the same
`PodLabelerPolicy` set; any non-determinism is a defect, not a race condition to retry.

### II. Label Stability

The controller MUST NOT write a label whose value is identical to the one already present on
the pod. Every redundant label write triggers an Istio xDS config push; at fleet scale this
produces a push storm that degrades the entire mesh control plane. The reconciler MUST gate
every write on a diff check (current label value vs. computed value) and skip no-op updates
silently. Label flicker — toggling between values across reconcile cycles — is treated as a
P0 defect equivalent to an outage.

### III. Policy-Driven Classification

All tier assignments MUST be derived solely from `PodLabelerPolicy` custom resources whose
`imagePattern` matchers are evaluated against the pod's container images at reconcile time.
Hardcoded tier logic anywhere in controller source code is prohibited. The `PodLabelerPolicy`
CRD is the sole authoritative rulebook; operators change classification by editing policies,
not code. Policy ordering MUST be deterministic (e.g., sorted by name) when multiple policies
match a pod, to prevent label value variance between controller restarts.

### IV. Fleet-Safe Engineering

A defect in the reconciliation loop is paid by the entire fleet. Therefore:

- Every reconcile-path change MUST be covered by unit tests that exercise the full reconcile
  loop with fakes (no live API server required for unit tests).
- Changes to label-assignment logic MUST additionally be covered by integration tests running
  against an `envtest` API server or a `kind` cluster.
- Breaking changes to label behavior (e.g., adding a new tier value or changing match
  semantics) require a documented migration plan and staged rollout before merging to main.
- The controller MUST fail closed: if a `PodLabelerPolicy` cannot be evaluated (e.g., parse
  error, missing CRD, API server timeout), the pod's existing label MUST remain unchanged
  rather than being removed or overwritten with a default. Uncertainty never justifies a write.

### V. Observability

The controller MUST expose Prometheus metrics for every observable event:

- `mesh_priority_labels_applied_total` — label writes, labeled by `tier` and `namespace`
- `mesh_priority_labels_skipped_total` — no-op skips (label value already correct)
- `mesh_priority_reconcile_errors_total` — reconcile failures, labeled by `error_category`
- `mesh_priority_policy_evaluations_total` — policy match evaluations, labeled by `result`

Every label mutation MUST be recorded in structured JSON logs including pod name, namespace,
old label value (or `<none>` for first application), and new label value. Log entries MUST
be emitted at `Info` level for normal operations and `Error` level for failures. Audit trail
completeness is non-negotiable; silent mutations are a debugging and compliance failure.

## Operational Constraints

- The controller MUST be deployable as a single Deployment with leader election enabled so
  only one replica acts as the active reconciler at a time, preventing concurrent label writes
  to the same pod.
- The controller MUST respect Kubernetes API rate limits via client-go's RateLimiter and MUST
  NOT issue unbounded LIST calls during reconcile bursts. Exponential backoff with jitter is
  required on retry.
- Pod churn (rapid creation/deletion) MUST NOT cause more than one label write per pod per
  reconcile event. Duplicate events MUST be deduplicated via the controller-runtime queue.
- The `PodLabelerPolicy` CRD MUST be versioned (`v1alpha1` → `v1beta1` → `v1`). Promoting a
  version MUST include either a conversion webhook or a documented manual migration path.
  Version promotion without a migration path is a breaking change requiring a MAJOR bump.
- Secrets and credentials (if any) MUST be mounted as Kubernetes Secrets, never baked into
  container images or supplied as plaintext environment variables in Deployment manifests.

## Quality Gates

All pull requests modifying reconcile logic, CRD schema, label-assignment rules, or metric
definitions MUST pass:

1. **Unit tests** — `go test ./...` with no real cluster dependency; target runtime under 30 s.
2. **Integration tests** — `envtest`-based tests covering at minimum:
   - Policy match → label applied to pod.
   - Policy match change → label updated (not flickered — single write per reconcile).
   - No matching policy → existing label removed (or left if fail-closed condition applies).
   - Policy parse error → label unchanged (fail-closed verified).
3. **Lint** — `golangci-lint run` passes with the project's `.golangci.yml` configuration.
4. **Diff-gate test** — At least one test MUST confirm that a reconcile cycle with an
   already-correct label produces zero API server label write calls.

Pull requests that cannot pass a gate MUST document the exception in the PR description and
receive approval from at least one additional maintainer beyond the author.

## Governance

This constitution supersedes all other documented practices for the `mesh-priority-controller`
project. Amendments follow this procedure:

1. Open a pull request with the proposed change and a written rationale explaining why the
   current text is insufficient.
2. The PR description MUST identify which principle(s) change, the semantic version bump type,
   and any migration impact on open specs or in-flight features.
3. Amendments require at least one approval from a project maintainer who did not author
   the change.
4. On merge, the version line MUST be updated per semantic versioning:
   - **MAJOR**: removing or redefining an existing principle in a backward-incompatible way.
   - **MINOR**: adding a new principle or substantially expanding guidance in an existing one.
   - **PATCH**: clarifications, wording fixes, or non-semantic refinements.
5. After any MAJOR version bump, all open feature specs and plans MUST be reviewed for
   compliance and updated within one sprint.

**Version**: 1.0.0 | **Ratified**: 2026-04-26 | **Last Amended**: 2026-04-26
