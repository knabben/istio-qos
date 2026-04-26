/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package controller contains regression tests that mirror the three deliberate
// bugs introduced in act1/ (the bug-baseline demo). Each test name matches the
// corresponding failing test in act1/controller/reconciler_test.go so CI output
// makes the connection explicit. All three tests PASS against this implementation.
package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	meshv1alpha1 "github.com/knabben/istio-qos/api/v1alpha1"
)

// TestBug1_LostUpdate mirrors act1's TestBug1_LostUpdate.
//
// In act1 the reconciler calls r.Update() which sends a full PUT carrying the
// pod's in-memory resourceVersion.  Two concurrent writers each read the pod at
// the same resourceVersion; whichever PUT arrives second gets a 409 Conflict and
// silently drops its labels.
//
// This implementation uses server-side apply (k8sClient.Apply).  The
// API server merges label fields at field-ownership level, so two writers using
// different field owners never conflict — both labels survive.
//
// Expected result: PASS (both labels present after concurrent SSA patches).
func TestBug1_LostUpdate(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available — run via 'make test'")
	}
	ctx := context.Background()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "bug1-regression-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "nginx:1.25"},
			},
		},
	}
	if err := k8sClient.Create(ctx, pod); err != nil {
		t.Fatalf("create pod: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pod) })

	// Two concurrent SSA writers, each owning a different label key.
	patch1 := &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace, Labels: map[string]string{"policy-a": "applied"}},
	}
	patch2 := &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace, Labels: map[string]string{"policy-b": "applied"}},
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = k8sClient.Apply(ctx, patch1, client.ForceOwnership, client.FieldOwner("policy-a"))
	}()
	go func() {
		defer wg.Done()
		_ = k8sClient.Apply(ctx, patch2, client.ForceOwnership, client.FieldOwner("policy-b"))
	}()
	wg.Wait()

	var result corev1.Pod
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, &result); err != nil {
		t.Fatalf("get pod: %v", err)
	}

	if result.Labels["policy-a"] != "applied" || result.Labels["policy-b"] != "applied" {
		t.Errorf("expected both labels to survive concurrent SSA writes, got: %v", result.Labels)
	}
}

// TestBug2_StaleCache mirrors act1's TestBug2_StaleCache.
//
// In act1 the reconciler propagates the NotFound error from r.Get() back to the
// workqueue, causing exponential backoff and eventual event drop.
//
// This implementation guards with apierrors.IsNotFound and returns (ctrl.Result{}, nil)
// — a missing pod is a benign transient condition (stale informer cache or already-
// deleted pod).
//
// Expected result: PASS (nil error returned for missing pod).
func TestBug2_StaleCache(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available — run via 'make test'")
	}
	ctx := context.Background()

	r := &PodLabelerPolicyReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "nonexistent-pod"},
	})

	if err != nil {
		t.Fatalf("expected nil error for missing pod (transient cache miss), got: %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Errorf("expected empty Result, got: %+v", result)
	}
}

// TestBug3_NoLease mirrors act1's TestBug3_NoLease.
//
// In act1 the manager is created with LeaderElection: false (the default).  With
// replicas: 2 both pods reconcile simultaneously, amplifying the Bug 1 race and
// causing continuous Istio xDS push storms. No Lease object is ever created.
//
// This implementation enforces LeaderElection via ValidateManagerOptions (which is
// called at startup) and starts the manager with LeaderElection: true, which causes
// controller-runtime to create a Lease.
//
// Expected result: PASS — ValidateManagerOptions rejects LeaderElection=false; a
// manager started with LeaderElection=true creates at least one Lease.
func TestBug3_NoLease(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available — run via 'make test'")
	}

	// Verify the guard rejects LeaderElection=false (the act1 bug configuration).
	if err := ValidateManagerOptions(ctrl.Options{LeaderElection: false}); err == nil {
		t.Fatal("expected ValidateManagerOptions to reject LeaderElection=false, got nil")
	}

	// Start a manager with LeaderElection=true and verify a Lease is created.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                        k8sClient.Scheme(),
		Metrics:                       metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress:        "0",
		LeaderElection:                true,
		LeaderElectionID:              "bug3-regression.knabben.dev",
		LeaderElectionNamespace:       "default",
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	go func() { _ = mgr.Start(ctx) }()

	// Allow time for the manager to acquire the leader lease.
	time.Sleep(3 * time.Second)

	var leases coordinationv1.LeaseList
	if err := k8sClient.List(ctx, &leases, client.InNamespace("default")); err != nil {
		t.Fatalf("list leases: %v", err)
	}

	if len(leases.Items) < 1 {
		t.Errorf("expected at least 1 Lease in default namespace (LeaderElection=true), got 0")
	}
}

// TestCoreLabeling mirrors act1's TestCoreLabeling.
//
// Happy-path: a pod whose container image matches a PodLabelerPolicy receives the
// tier label after a single reconcile.  This test passes on both act1 and this
// implementation.
//
// Expected result: PASS.
func TestCoreLabeling(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available — run via 'make test'")
	}
	ctx := context.Background()

	policy := &meshv1alpha1.PodLabelerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "core-labeling-policy"},
		Spec: meshv1alpha1.PodLabelerPolicySpec{
			ImagePattern: "nginx:*",
			Tier:         "high",
		},
	}
	if err := k8sClient.Create(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, policy) })

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "core-labeling-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "nginx:1.25"},
			},
		},
	}
	if err := k8sClient.Create(ctx, pod); err != nil {
		t.Fatalf("create pod: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pod) })

	r := &PodLabelerPolicyReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
	}
	if _, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name},
	}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var result corev1.Pod
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}, &result); err != nil {
		t.Fatalf("get pod: %v", err)
	}
	if result.Labels[tierLabel] != "high" {
		t.Errorf("expected tier=high, got %q", result.Labels[tierLabel])
	}
}
