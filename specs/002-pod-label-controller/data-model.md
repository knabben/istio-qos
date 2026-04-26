# Data Model: Pod Tier Label Controller

**Feature**: 002-pod-label-controller
**Date**: 2026-04-26

## CRD: PodLabelerPolicy

Cluster-scoped custom resource. Group: `mesh.knabben.github.com`. Version: `v1alpha1`.

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `imagePattern` | `string` | Yes | Glob pattern matched against each container image in a pod (e.g. `nginx:*`, `*/myapp:v1.*`) |
| `tier` | `string` enum | Yes | Target tier label value: `high` or `standard` |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `matchedPods` | `int32` | Number of pods currently labeled by this policy |
| `conditions` | `[]metav1.Condition` | Standard condition set; `Ready` condition reflects policy validity |

### Validation Rules

- `tier` MUST be one of `["high", "standard"]` — enforced by CRD CEL validation.
- `imagePattern` MUST be a non-empty string — enforced by CRD validation.
- `imagePattern` MUST compile as a valid glob — validated at admission time via webhook (v2);
  in v1 an invalid pattern is recorded as a `Ready=False` condition on the policy.

### Example Manifest

```yaml
apiVersion: mesh.knabben.github.com/v1alpha1
kind: PodLabelerPolicy
metadata:
  name: premium-app-policy
spec:
  imagePattern: "*/premium-app:*"
  tier: high
```

---

## Managed Resource: Pod

The controller reads pods from the informer cache and writes the `tier` label via
server-side apply. It does not own any other pod field.

### Owned Fields

| Field | Value | Owner |
|-------|-------|-------|
| `metadata.labels["tier"]` | `high` \| `standard` | `mesh-priority-controller` |

### Label Lifecycle

| Event | Action |
|-------|--------|
| Pod created, matches policy | Apply `tier=<value>` |
| Policy created/updated, pod now matches | Apply `tier=<value>` (or update if changed) |
| Policy deleted, pod no longer matches any policy | Remove `tier` label (apply patch without `tier`) |
| Pod already has correct label | Skip write (diff-gate, Principle II) |
| Conflicting policies match the pod | Apply tier from alphabetically-first policy name; emit Warning event |

---

## Supporting Entities

### Tier Label

| Attribute | Value |
|-----------|-------|
| Key | `tier` |
| Allowed values | `high`, `standard` |
| Owner field manager | `mesh-priority-controller` |

### Kubernetes Event (audit)

Emitted on each managed pod when a tier label is applied or removed.

| Field | Value |
|-------|-------|
| `reason` | `TierLabelApplied` \| `TierLabelRemoved` \| `TierConflict` |
| `message` | Includes policy name, old value, new value |
| `type` | `Normal` (apply/remove) \| `Warning` (conflict) |

---

## Reference Example Entities (config/samples/)

### Deployment (4 pods, 2 high / 2 standard)

Two `PodLabelerPolicy` resources target different image patterns:
- `policy-high`: pattern `*/tier-app-high:*` → `tier: high`
- `policy-standard`: pattern `*/tier-app-standard:*` → `tier: standard`

A Deployment runs 4 replicas split across two container image variants.

### DestinationRule

```yaml
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: tier-routing
spec:
  host: tier-app-svc
  subsets:
    - name: high-priority-pods
      labels:
        tier: high
    - name: standard-pods
      labels:
        tier: standard
```

### VirtualService

```yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: tier-routing
spec:
  hosts: [tier-app-svc]
  http:
    - match:
        - headers:
            user-type:
              exact: premium
      route:
        - destination:
            host: tier-app-svc
            subset: high-priority-pods
    - route:
        - destination:
            host: tier-app-svc
            subset: standard-pods
```

---

## State Transitions

```
[Pod created]
      │
      ▼ Reconcile triggered
[Evaluate all PodLabelerPolicies against pod images]
      │
      ├── Match found (single or tie-broken)
      │         │
      │         ▼
      │   [Diff: current label == computed?]
      │         ├── Yes → skip write (Principle II)
      │         └── No  → server-side apply tier label → emit Event
      │
      └── No match found
                │
                ▼
          [tier label present?]
                ├── Yes → server-side apply (remove tier) → emit Event
                └── No  → no-op
```
