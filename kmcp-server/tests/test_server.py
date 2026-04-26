"""Unit tests for the podlabeler-debug MCP server tools.

Each test mocks the kubernetes Python client so no live cluster is required.
These tests run in CI on a plain ubuntu runner (see .github/workflows/kmcp-server-tests.yml).
"""

import json
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest


# ---------------------------------------------------------------------------
# Helpers — build mock Kubernetes API objects
# ---------------------------------------------------------------------------

def _make_container(cname="app", image="localhost/team/app1:v1"):
    # MagicMock(name=...) sets the mock's internal name, not a .name attribute.
    # Use SimpleNamespace so that c.name and c.image are plain strings.
    return SimpleNamespace(name=cname, image=image)


def _make_pod(name="test-pod", namespace="default", labels=None, phase="Running"):
    pod = MagicMock()
    pod.metadata.name = name
    pod.metadata.namespace = namespace
    pod.metadata.uid = "uid-1234"
    pod.metadata.resource_version = "42"
    pod.metadata.labels = labels or {"tier": "high"}
    pod.status.phase = phase
    pod.status.conditions = []
    pod.spec.node_name = "kind-worker"
    pod.spec.containers = [_make_container()]
    return pod


def _make_event(name="ev-1", reason="FailedUpdate", message="conflict", event_type="Warning"):
    ev = MagicMock()
    ev.metadata.name = name
    ev.reason = reason
    ev.message = message
    ev.type = event_type
    ev.count = 3
    ev.first_timestamp = None
    ev.last_timestamp = None
    ev.involved_object.name = "test-pod"
    return ev


def _make_lease(name="podlabeler.knabben.dev", namespace="kube-system", holder="controller-0"):
    lease = MagicMock()
    lease.metadata.name = name
    lease.metadata.namespace = namespace
    lease.spec.holder_identity = holder
    lease.spec.acquire_time = None
    lease.spec.renew_time = None
    return lease


# ---------------------------------------------------------------------------
# test_list_pods
# ---------------------------------------------------------------------------

@patch("server.config.load_kube_config")
@patch("server.client.CoreV1Api")
def test_list_pods(mock_core_api_cls, mock_load_config):
    pod1 = _make_pod("pod-a", labels={"tier": "high"})
    pod2 = _make_pod("pod-b", labels={"tier": "standard"})
    mock_core_api_cls.return_value.list_namespaced_pod.return_value.items = [pod1, pod2]

    from server import list_pods
    result = json.loads(list_pods(namespace="default"))

    assert len(result) == 2
    assert result[0]["name"] == "pod-a"
    assert result[0]["labels"]["tier"] == "high"
    assert result[1]["name"] == "pod-b"
    mock_core_api_cls.return_value.list_namespaced_pod.assert_called_once_with(namespace="default")


# ---------------------------------------------------------------------------
# test_get_pod
# ---------------------------------------------------------------------------

@patch("server.config.load_kube_config")
@patch("server.client.CoreV1Api")
def test_get_pod(mock_core_api_cls, mock_load_config):
    pod = _make_pod("bug1-pod", labels={"policy-b": "applied"})
    mock_core_api_cls.return_value.read_namespaced_pod.return_value = pod

    from server import get_pod
    result = json.loads(get_pod(name="bug1-pod", namespace="default"))

    assert result["name"] == "bug1-pod"
    assert "policy-b" in result["labels"]
    # policy-a is missing — this is Bug 1 evidence
    assert "policy-a" not in result["labels"]
    assert result["resource_version"] == "42"


# ---------------------------------------------------------------------------
# test_list_pod_logs
# ---------------------------------------------------------------------------

@patch("server.config.load_kube_config")
@patch("server.client.CoreV1Api")
def test_list_pod_logs(mock_core_api_cls, mock_load_config):
    log_output = 'pods "nonexistent-pod" not found\nreconcile error\n'
    mock_core_api_cls.return_value.read_namespaced_pod_log.return_value = log_output

    from server import list_pod_logs
    result = list_pod_logs(pod_name="controller-abc", namespace="default", tail_lines=50)

    assert "not found" in result
    mock_core_api_cls.return_value.read_namespaced_pod_log.assert_called_once_with(
        name="controller-abc", namespace="default", tail_lines=50
    )


# ---------------------------------------------------------------------------
# test_list_events
# ---------------------------------------------------------------------------

@patch("server.config.load_kube_config")
@patch("server.client.CoreV1Api")
def test_list_events(mock_core_api_cls, mock_load_config):
    ev = _make_event(reason="FailedUpdate", message="Operation cannot be fulfilled: conflict")
    mock_core_api_cls.return_value.list_namespaced_event.return_value.items = [ev]

    from server import list_events
    result = json.loads(list_events(namespace="default"))

    assert len(result) == 1
    assert result[0]["reason"] == "FailedUpdate"
    assert "conflict" in result[0]["message"]
    assert result[0]["type"] == "Warning"
    assert result[0]["count"] == 3


# ---------------------------------------------------------------------------
# test_list_leases — empty list is Bug 3 evidence
# ---------------------------------------------------------------------------

@patch("server.config.load_kube_config")
@patch("server.client.CoordinationV1Api")
def test_list_leases_empty(mock_coord_api_cls, mock_load_config):
    mock_coord_api_cls.return_value.list_namespaced_lease.return_value.items = []

    from server import list_leases
    result = json.loads(list_leases(namespace="default"))

    # Empty list = Bug 3: no Lease created because LeaderElection is false
    assert result == []
    mock_coord_api_cls.return_value.list_namespaced_lease.assert_called_once_with(namespace="default")


@patch("server.config.load_kube_config")
@patch("server.client.CoordinationV1Api")
def test_list_leases_with_lease(mock_coord_api_cls, mock_load_config):
    lease = _make_lease(name="podlabeler.knabben.dev", holder="controller-pod-0")
    mock_coord_api_cls.return_value.list_namespaced_lease.return_value.items = [lease]

    from server import list_leases
    result = json.loads(list_leases(namespace="kube-system"))

    assert len(result) == 1
    assert result[0]["name"] == "podlabeler.knabben.dev"
    assert result[0]["holder"] == "controller-pod-0"


# ---------------------------------------------------------------------------
# test_list_podlabelerpolicies
# ---------------------------------------------------------------------------

@patch("server.config.load_kube_config")
@patch("server.client.CustomObjectsApi")
def test_list_podlabelerpolicies(mock_custom_api_cls, mock_load_config):
    mock_custom_api_cls.return_value.list_cluster_custom_object.return_value = {
        "items": [
            {
                "metadata": {"name": "high-tier-policy"},
                "spec": {
                    "imagePattern": "app1:*",
                    "labels": {"tier": "high"},
                },
            }
        ]
    }

    from server import list_podlabelerpolicies
    result = json.loads(list_podlabelerpolicies())

    assert len(result) == 1
    assert result[0]["name"] == "high-tier-policy"
    assert result[0]["image_pattern"] == "app1:*"
    assert result[0]["labels"]["tier"] == "high"
    mock_custom_api_cls.return_value.list_cluster_custom_object.assert_called_once_with(
        group="labeling.knabben.dev",
        version="v1alpha1",
        plural="podlabelerpolicies",
    )
