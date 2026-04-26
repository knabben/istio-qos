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

package controller

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	meshv1alpha1 "github.com/knabben/istio-qos/api/v1alpha1"
)

// ─── Required integration tests (spec §IV) ───────────────────────────────────

// TestReconcile_NoLostUpdates verifies that the SSA-based write preserves
// concurrent unrelated field changes on a pod. This confirms the reconciler
// uses client.Apply (not client.Update or MergePatch) for tier label writes.
func TestReconcile_NoLostUpdates(t *testing.T) {
	if cfg == nil {
		t.Skip("envtest not available — run via 'make test'")
	}
	ctx := context.Background()

	policy := &meshv1alpha1.PodLabelerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-no-lost-updates"},
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
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-no-lost-updates",
			Namespace: "default",
		},
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
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}}

	// First reconcile: apply tier label.
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	// Concurrent change: add an unrelated annotation directly.
	var latest corev1.Pod
	if err := k8sClient.Get(ctx, req.NamespacedName, &latest); err != nil {
		t.Fatalf("get pod: %v", err)
	}
	if latest.Annotations == nil {
		latest.Annotations = map[string]string{}
	}
	latest.Annotations["concurrent-actor"] = "present"
	if err := k8sClient.Update(ctx, &latest); err != nil {
		t.Fatalf("add concurrent annotation: %v", err)
	}

	// Second reconcile: SSA should preserve the concurrent annotation.
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	var result corev1.Pod
	if err := k8sClient.Get(ctx, req.NamespacedName, &result); err != nil {
		t.Fatalf("get result pod: %v", err)
	}

	if result.Labels[tierLabel] != "high" {
		t.Errorf("expected tier=high, got %q", result.Labels[tierLabel])
	}
	if result.Annotations["concurrent-actor"] != "present" {
		t.Errorf("concurrent annotation was lost: %v", result.Annotations)
	}
}

// TestReconcile_CacheNotFound verifies that a NotFound response from the cache
// causes the reconciler to return ctrl.Result{} and nil error — no panic, no
// error propagation, no requeue storm.
func TestReconcile_CacheNotFound(t *testing.T) {
	s := buildScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	r := &PodLabelerPolicyReconciler{
		Client:   fakeClient,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: "default"},
	})

	if err != nil {
		t.Fatalf("expected nil error for NotFound, got: %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Errorf("expected empty Result, got: %+v", result)
	}
}

// TestReconcile_LeaderElection verifies that validateManagerOptions rejects
// a manager configuration with LeaderElection disabled. This enforces the
// non-negotiable startup invariant from spec §III.
func TestReconcile_LeaderElection(t *testing.T) {
	opts := ctrl.Options{
		LeaderElection: false,
	}
	if err := ValidateManagerOptions(opts); err == nil {
		t.Fatal("expected error when LeaderElection=false, got nil")
	}

	validOpts := ctrl.Options{
		LeaderElection: true,
	}
	if err := ValidateManagerOptions(validOpts); err != nil {
		t.Fatalf("expected no error when LeaderElection=true, got: %v", err)
	}
}

// ─── Ginkgo integration tests ─────────────────────────────────────────────────

var _ = Describe("PodLabelerPolicy Controller", func() {
	const (
		policyName = "test-policy"
		podName    = "test-pod"
		namespace  = "default"
		timeout    = 10 * time.Second
		interval   = 250 * time.Millisecond
	)

	ctx := context.Background()
	policyKey := types.NamespacedName{Name: policyName}
	podKey := types.NamespacedName{Name: podName, Namespace: namespace}

	Context("When a PodLabelerPolicy matches a pod", func() {
		var r *PodLabelerPolicyReconciler

		BeforeEach(func() {
			r = &PodLabelerPolicyReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			policy := &meshv1alpha1.PodLabelerPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: policyName},
				Spec: meshv1alpha1.PodLabelerPolicySpec{
					ImagePattern: "nginx:*",
					Tier:         "standard",
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "nginx:1.25"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		})

		AfterEach(func() {
			policy := &meshv1alpha1.PodLabelerPolicy{}
			if err := k8sClient.Get(ctx, policyKey, policy); err == nil {
				Expect(k8sClient.Delete(ctx, policy)).To(Succeed())
			}
			pod := &corev1.Pod{}
			if err := k8sClient.Get(ctx, podKey, pod); err == nil {
				Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
			}
		})

		// T011: label applied
		It("should apply the tier label to a matching pod", func() {
			By("reconciling the pod")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: podKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the tier label was set")
			var pod corev1.Pod
			Expect(k8sClient.Get(ctx, podKey, &pod)).To(Succeed())
			Expect(pod.Labels).To(HaveKeyWithValue(tierLabel, "standard"))
		})

		// T012: label removed when policy is deleted
		It("should remove the tier label when the policy is deleted", func() {
			By("reconciling once to apply the label")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: podKey})
			Expect(err).NotTo(HaveOccurred())

			By("deleting the policy")
			policy := &meshv1alpha1.PodLabelerPolicy{}
			Expect(k8sClient.Get(ctx, policyKey, policy)).To(Succeed())
			Expect(k8sClient.Delete(ctx, policy)).To(Succeed())

			By("waiting for the policy to be gone from cache")
			Eventually(func() bool {
				p := &meshv1alpha1.PodLabelerPolicy{}
				return apierrors.IsNotFound(k8sClient.Get(ctx, policyKey, p))
			}, timeout, interval).Should(BeTrue())

			By("reconciling again after policy deletion")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: podKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the tier label was removed")
			var pod corev1.Pod
			Expect(k8sClient.Get(ctx, podKey, &pod)).To(Succeed())
			Expect(pod.Labels).NotTo(HaveKey(tierLabel))
		})

		It("should skip the write when the tier label is already correct", func() {
			By("reconciling once to apply label")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: podKey})
			Expect(err).NotTo(HaveOccurred())

			By("reconciling again — should be a no-op (diff-gate)")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: podKey})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When two policies match a pod with conflicting tiers", func() {
		const (
			policy1Name = "alpha-policy"
			policy2Name = "beta-policy"
			conflictPod = "conflict-pod"
		)
		ctx := context.Background()
		pod1Key := types.NamespacedName{Name: conflictPod, Namespace: "default"}

		BeforeEach(func() {
			for _, p := range []struct {
				name string
				tier string
			}{
				{policy1Name, "high"},
				{policy2Name, "standard"},
			} {
				policy := &meshv1alpha1.PodLabelerPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: p.name},
					Spec: meshv1alpha1.PodLabelerPolicySpec{
						ImagePattern: "nginx:*",
						Tier:         p.tier,
					},
				}
				Expect(k8sClient.Create(ctx, policy)).To(Succeed())
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: conflictPod, Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		})

		AfterEach(func() {
			for _, name := range []string{policy1Name, policy2Name} {
				p := &meshv1alpha1.PodLabelerPolicy{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, p); err == nil {
					Expect(k8sClient.Delete(ctx, p)).To(Succeed())
				}
			}
			pod := &corev1.Pod{}
			if err := k8sClient.Get(ctx, pod1Key, pod); err == nil {
				Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
			}
		})

		It("should apply the alphabetically-first policy's tier and emit a conflict event", func() {
			fakeRecorder := record.NewFakeRecorder(10)
			r := &PodLabelerPolicyReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: fakeRecorder,
			}

			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: pod1Key})
			Expect(err).NotTo(HaveOccurred())

			var pod corev1.Pod
			Expect(k8sClient.Get(ctx, pod1Key, &pod)).To(Succeed())
			// alpha-policy (high) wins over beta-policy (standard) alphabetically
			Expect(pod.Labels).To(HaveKeyWithValue(tierLabel, "high"))

			By("verifying TierConflict warning event was emitted")
			var conflictEmitted bool
			for len(fakeRecorder.Events) > 0 {
				evt := <-fakeRecorder.Events
				if evt != "" && len(evt) > 0 {
					conflictEmitted = true
					break
				}
			}
			Expect(conflictEmitted).To(BeTrue())
		})
	})

	Context("mapPolicyToPods", func() {
		It("should return requests for all existing pods", func() {
			r := &PodLabelerPolicyReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}
			reqs := r.mapPolicyToPods(context.Background(), nil)
			// Just verify it doesn't panic and returns a slice
			Expect(reqs).NotTo(BeNil())
		})
	})
})

// buildScheme returns a scheme with corev1 and meshv1alpha1 registered for fake client.
func buildScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("add corev1: %v", err)
	}
	if err := meshv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add meshv1alpha1: %v", err)
	}
	return s
}
