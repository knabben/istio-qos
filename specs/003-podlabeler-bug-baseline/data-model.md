# Data Model: PodLabeler Bug Baseline (Act I)

**Feature**: 003-podlabeler-bug-baseline
**Date**: 2026-04-26

---

## Entities

### PodLabelerPolicy (cluster-scoped CRD)

The sole custom resource. Defines a matching rule and the labels to apply.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `metadata.name` | string | yes | Unique policy identifier, cluster-scoped |
| `spec.imagePattern` | string | yes | Pattern matched against pod's primary container image |
| `spec.labels` | map[string]string | yes | Labels applied to all matching pods |

**Pattern matching rules** (imageMatches function in reconciler.go):
- Registry prefix stripped: `registry.io/team/app1:v1` → `app1:v1`
- Wildcard tag: `app1:*` matches any `app1:<tag>`
- Exact match: `app1:latest` matches only `app1:latest`
- Prefix match: `app1` matches `app1`, `app1:v1`, `app1-extra:v1`

**Go type**:
```go
type PodLabelerPolicy struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              PodLabelerPolicySpec `json:"spec,omitempty"`
}

type PodLabelerPolicySpec struct {
    ImagePattern string            `json:"imagePattern"`
    Labels       map[string]string `json:"labels"`
}
```

**DeepCopy**: Written by hand in `types.go` — no controller-gen.

---

### Pod (Kubernetes core/v1)

The target resource. The reconciler reads pods from the informer cache and writes
labels back via the API server. Only the pod's primary container (index 0) image is
used for policy matching.

**Relevant fields for the reconciler**:
| Field | Used for |
|-------|----------|
| `metadata.name` + `metadata.namespace` | Reconcile request key |
| `metadata.deletionTimestamp` | Skip pods being deleted |
| `metadata.labels` | Read current labels; write desired labels |
| `metadata.resourceVersion` | **Bug 1**: embedded in Update() PUT — causes lost update on concurrency |
| `spec.containers[0].image` | Primary container image for policy matching |

---

### Lease (coordination.k8s.io/v1)

Created by the controller-runtime manager when `LeaderElection: true`. **Absent** in
the Act I baseline because `LeaderElection: false`. The envtest Bug 3 test asserts this
object's presence.

| Field | Value in correct code | Value in Act I |
|-------|----------------------|----------------|
| `metadata.name` | `podlabeler.knabben.dev` | (object does not exist) |
| `metadata.namespace` | `podlabeler-system` | (object does not exist) |
| `spec.holderIdentity` | pod name of active replica | N/A |

---

### PodReconciler (controller struct)

| Field | Type | Purpose |
|-------|------|---------|
| `Client` | `client.Client` | API server reads (cache) and writes |
| `Scheme` | `*runtime.Scheme` | Type registration for GVK resolution |

**Reconcile contract** (broken version):

```
Input:  ctrl.Request{NamespacedName: {Namespace, Name}}
Output: (ctrl.Result{}, error)

Steps:
  1. r.Get(pod)      → BUG 2: NotFound returned as terminal error
  2. Skip if DeletionTimestamp set
  3. Skip if no containers
  4. r.List(policies)
  5. Compute desired labels by walking policies
  6. Mutate pod.Labels in memory
  7. r.Update(pod)   → BUG 1: full PUT without conflict detection
```

---

## State Transitions

### Pod Label State (under Act I bugs)

```
[No label]
    │  reconcile fires (single replica, no concurrent writers)
    ▼
[tier=high]          ← CORRECT under ideal conditions
    │
    │  concurrent reconcile OR kubelet status update
    ▼
[tier=?? / conflict]  ← Bug 1: 409 Conflict or clobber; label may be lost
    │
    │  next reconcile (if not dropped)
    ▼
[tier=high]          ← may recover, or oscillate indefinitely (Bug 3)
```

### Manager Startup State

```
[Starting]
    │  LeaderElection: false
    ▼
[Active — all replicas]   ← Bug 3: every replica reconciles independently
                             No Lease object created
```

---

## Relationships

```
PodLabelerPolicy  ──(imagePattern matches)──▶  Pod.spec.containers[0].image
                                               │
                                               ▼
                                          PodReconciler.Reconcile()
                                               │
                                               ├── r.Get(pod)     [Bug 2]
                                               ├── r.List(policies)
                                               └── r.Update(pod)  [Bug 1]

Manager
  └── LeaderElection: false  [Bug 3]
        └── No Lease created in podlabeler-system namespace
```

---

## CRD Schema Summary

Group: `labeling.knabben.dev`
Version: `v1alpha1`
Kind: `PodLabelerPolicy`
Scope: Cluster

Validation (from `manifests/crd.yaml`):
- `spec.imagePattern`: string, minLength: 1, required
- `spec.labels`: object, additionalProperties: string, minProperties: 1, required
