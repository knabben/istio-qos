"""podlabeler-debug MCP server.

Provides six read-only Kubernetes tools that give Claude Code live visibility
into a running cluster for debugging the three Act I podlabeler bugs.

Tools:
  list_pods               — list pods with labels and phase
  get_pod                 — full pod detail (labels, status, containers)
  list_pod_logs           — tail recent log lines from a pod container
  list_events             — warning / normal events in a namespace
  list_leases             — coordination.k8s.io Lease objects
  list_podlabelerpolicies — PodLabelerPolicy custom resources
"""

import json
from typing import Any

from fastmcp import FastMCP
from kubernetes import client, config

mcp = FastMCP("podlabeler-debug")


def _load_kube_config() -> None:
    """Load kubeconfig from the default location (~/.kube/config)."""
    try:
        config.load_kube_config()
    except config.ConfigException:
        # Fall back to in-cluster config when running inside a pod.
        config.load_incluster_config()


def _pod_summary(pod: Any) -> dict:
    """Extract a compact dict from a V1Pod object."""
    return {
        "name": pod.metadata.name,
        "namespace": pod.metadata.namespace,
        "phase": pod.status.phase if pod.status else None,
        "labels": pod.metadata.labels or {},
        "node": pod.spec.node_name,
        "containers": [c.image for c in pod.spec.containers],
    }


@mcp.tool()
def list_pods(namespace: str = "default") -> str:
    """List all pods in *namespace* with their labels, phase, and container images.

    Use this tool to observe which labels the controller applied to each pod.
    An absent `tier` label or a missing `policy-*` label is evidence of Bug 1
    (lost update) or Bug 2 (stale cache / pod never labeled).
    """
    _load_kube_config()
    v1 = client.CoreV1Api()
    pods = v1.list_namespaced_pod(namespace=namespace)
    return json.dumps([_pod_summary(p) for p in pods.items], indent=2)


@mcp.tool()
def get_pod(name: str, namespace: str = "default") -> str:
    """Return the full detail of a single pod including all labels and conditions.

    Use this when `list_pods` shows a pod whose labels look wrong — this tool
    gives the complete label map so you can confirm which labels are present
    and which are missing.
    """
    _load_kube_config()
    v1 = client.CoreV1Api()
    pod = v1.read_namespaced_pod(name=name, namespace=namespace)
    return json.dumps(
        {
            "name": pod.metadata.name,
            "namespace": pod.metadata.namespace,
            "uid": pod.metadata.uid,
            "resource_version": pod.metadata.resource_version,
            "labels": pod.metadata.labels or {},
            "phase": pod.status.phase if pod.status else None,
            "conditions": [
                {"type": c.type, "status": c.status}
                for c in (pod.status.conditions or [])
            ],
            "containers": [
                {"name": c.name, "image": c.image}
                for c in pod.spec.containers
            ],
        },
        indent=2,
    )


@mcp.tool()
def list_pod_logs(
    pod_name: str,
    namespace: str = "default",
    container: str = "",
    tail_lines: int = 100,
) -> str:
    """Fetch the most recent *tail_lines* log lines from a pod container.

    Use this to read controller logs. Look for:
    - `pods "X" not found` → Bug 2 (stale cache read treated as terminal error)
    - `failed to update pod` with 409 Conflict → Bug 1 (lost update)
    - No leader-election messages → Bug 3 (LeaderElection: false)

    Pass `container=""` to use the first (and only) container.
    """
    _load_kube_config()
    v1 = client.CoreV1Api()
    kwargs: dict[str, Any] = {"tail_lines": tail_lines}
    if container:
        kwargs["container"] = container
    logs = v1.read_namespaced_pod_log(
        name=pod_name, namespace=namespace, **kwargs
    )
    return logs


@mcp.tool()
def list_events(namespace: str = "default", field_selector: str = "") -> str:
    """List Kubernetes events in *namespace*, optionally filtered by *field_selector*.

    Useful field selectors:
      `reason=FailedReconcile`
      `involvedObject.name=<pod-name>`

    Warning events with reason `FailedUpdate` or messages containing `Conflict`
    are direct evidence of Bug 1 racing writes.
    """
    _load_kube_config()
    v1 = client.CoreV1Api()
    kwargs: dict[str, Any] = {}
    if field_selector:
        kwargs["field_selector"] = field_selector
    events = v1.list_namespaced_event(namespace=namespace, **kwargs)
    return json.dumps(
        [
            {
                "name": e.metadata.name,
                "reason": e.reason,
                "message": e.message,
                "type": e.type,
                "count": e.count,
                "first_time": str(e.first_timestamp),
                "last_time": str(e.last_timestamp),
                "involved_object": e.involved_object.name
                if e.involved_object
                else None,
            }
            for e in events.items
        ],
        indent=2,
    )


@mcp.tool()
def list_leases(namespace: str = "default") -> str:
    """List coordination.k8s.io/v1 Lease objects in *namespace*.

    When the podlabeler manager is configured with `LeaderElection: true`, it
    creates exactly one Lease in the configured namespace. An **empty list is
    direct evidence of Bug 3** (LeaderElection: false — no Lease is ever created).

    Check `kube-system` for the act2 fixed controller's lease, or `default`
    for the act1 buggy controller (which creates no lease at all).
    """
    _load_kube_config()
    coord = client.CoordinationV1Api()
    leases = coord.list_namespaced_lease(namespace=namespace)
    return json.dumps(
        [
            {
                "name": l.metadata.name,
                "namespace": l.metadata.namespace,
                "holder": l.spec.holder_identity if l.spec else None,
                "acquire_time": str(l.spec.acquire_time) if l.spec else None,
                "renew_time": str(l.spec.renew_time) if l.spec else None,
            }
            for l in leases.items
        ],
        indent=2,
    )


@mcp.tool()
def list_podlabelerpolicies(namespace: str = "") -> str:
    """List PodLabelerPolicy custom resources (cluster-scoped).

    Returns each policy's name, imagePattern, and labels map. Use this to
    verify that the expected policies are installed and to cross-reference
    which labels should have been applied to a given pod image.
    """
    _load_kube_config()
    custom = client.CustomObjectsApi()
    result = custom.list_cluster_custom_object(
        group="labeling.knabben.dev",
        version="v1alpha1",
        plural="podlabelerpolicies",
    )
    items = result.get("items", [])
    return json.dumps(
        [
            {
                "name": p.get("metadata", {}).get("name"),
                "image_pattern": p.get("spec", {}).get("imagePattern"),
                "labels": p.get("spec", {}).get("labels", {}),
            }
            for p in items
        ],
        indent=2,
    )


if __name__ == "__main__":
    mcp.run()
