// Package controller contains the PodLabeler reconciler.
//
// THIS IS THE ACT I BASELINE — the broken version of the controller.
//
// Three distributed-systems bugs are deliberately present in this code.
// Each bug is marked with a // BUG <N>: comment so they can be located
// quickly during the talk.
//
// Act II of the talk fixes each bug, with a constitutional rule and an
// envtest assertion. This file is the failure baseline — what an AI
// assistant generates when asked for "a controller that labels pods,
// production ready" without any supervision architecture.
package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	labelingv1alpha1 "github.com/knabben/istio-poc/api/v1alpha1"
)

// PodReconciler reconciles a Pod object: it reads all PodLabelerPolicy
// resources, finds those whose imagePattern matches the pod's primary
// container image, and applies the policy's labels to the pod.
type PodReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is the heart of the controller.
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("pod", req.NamespacedName)

	// Fetch the Pod from the cache.
	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		// BUG 2: stale cache read.
		// The informer cache is eventually consistent. When a Pod Create
		// event fires, the cache may not yet reflect the write that
		// triggered it. r.Get() will return a NotFound error.
		//
		// This code returns the error as-if it were terminal, which:
		//   - Causes the workqueue to apply error backoff
		//   - Logs a misleading "pod not found" message
		//   - Eventually drops the request after maxretries
		//
		// The pod is never labeled. It silently joins the default
		// endpoint pool instead of the high-tier subset.
		//
		// FIX (Act II): check apierrors.IsNotFound(err) and return
		// ctrl.Result{}, nil — this is a benign condition (the pod
		// was deleted, or the cache hasn't caught up yet).
		return ctrl.Result{}, err
	}

	// Skip pods that are being deleted.
	if pod.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// Pod must have at least one container — otherwise nothing to match.
	if len(pod.Spec.Containers) == 0 {
		return ctrl.Result{}, nil
	}
	primaryImage := pod.Spec.Containers[0].Image

	// List all PodLabelerPolicy resources in the cluster.
	policies := &labelingv1alpha1.PodLabelerPolicyList{}
	if err := r.List(ctx, policies); err != nil {
		logger.Error(err, "failed to list PodLabelerPolicy resources")
		return ctrl.Result{}, err
	}

	// Compute the labels to apply by walking every policy.
	desired := map[string]string{}
	for _, policy := range policies.Items {
		if !imageMatches(primaryImage, policy.Spec.ImagePattern) {
			continue
		}
		for k, v := range policy.Spec.Labels {
			desired[k] = v
		}
	}

	// Nothing to do if no policy matched.
	if len(desired) == 0 {
		return ctrl.Result{}, nil
	}

	// Mutate the pod's labels in memory.
	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	for k, v := range desired {
		pod.Labels[k] = v
	}

	// BUG 1: lost update.
	// r.Update() reads the pod's current resourceVersion from the in-memory
	// object and sends a full PUT with that version. If two controller
	// replicas (BUG 3) reconcile the same pod concurrently, both read the
	// same resourceVersion, both Update(). One write wins. The other write
	// silently clobbers it.
	//
	// Worse: even with a single replica, a concurrent change from any
	// other actor (kubelet status update, another controller, kubectl edit)
	// can cause our write to clobber theirs — or fail with a 409 Conflict
	// that this code does not handle.
	//
	// FIX (Act II): use server-side apply with FieldOwner("podlabeler"),
	// sending only the labels we own. The API server resolves conflicts
	// at the field level instead of the object level.
	if err := r.Update(ctx, pod); err != nil {
		logger.Error(err, "failed to update pod")
		return ctrl.Result{}, err
	}

	logger.Info("applied labels", "labels", desired, "image", primaryImage)
	return ctrl.Result{}, nil
}

// SetupWithManager wires the controller into the manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Watches(
			&labelingv1alpha1.PodLabelerPolicy{},
			handler.EnqueueRequestsFromMapFunc(r.policyToPodRequests),
		).
		Complete(r)
}

// policyToPodRequests returns reconcile requests for every pod when a
// PodLabelerPolicy changes. This is intentionally simple — production
// code would index pods by image and only enqueue affected pods.
func (r *PodReconciler) policyToPodRequests(ctx context.Context, _ client.Object) []reconcile.Request {
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(pods.Items))
	for _, pod := range pods.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKey{Namespace: pod.Namespace, Name: pod.Name},
		})
	}
	return requests
}

// imageMatches checks whether a container image matches a policy pattern.
//
// Patterns supported:
//   - "app1:latest"   exact match
//   - "app1:*"        wildcard tag (any tag of app1)
//   - "app1"          prefix match (anything starting with app1)
//
// The registry prefix (e.g. "registry.io/team/") is stripped from the
// image before matching.
func imageMatches(image, pattern string) bool {
	// Strip registry prefix: "registry.io/team/app1:v1" -> "app1:v1".
	if idx := strings.LastIndex(image, "/"); idx >= 0 {
		image = image[idx+1:]
	}

	// Wildcard tag: "app1:*" matches "app1:anything".
	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, ":*") + ":"
		return strings.HasPrefix(image, prefix)
	}

	// Exact match: "app1:latest".
	if strings.Contains(pattern, ":") {
		return image == pattern
	}

	// Prefix match: "app1" matches "app1", "app1:v1", "app1-extra:v1", etc.
	return strings.HasPrefix(image, pattern)
}
