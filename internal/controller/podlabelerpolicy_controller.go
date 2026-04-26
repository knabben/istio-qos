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
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	meshv1alpha1 "github.com/knabben/istio-qos/api/v1alpha1"
	"github.com/knabben/istio-qos/internal/matcher"
)

const (
	fieldManager = "mesh-priority-controller"
	tierLabel    = "tier"
)

// PodLabelerPolicyReconciler reconciles Pods using PodLabelerPolicy rules.
type PodLabelerPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=mesh.knabben.github.com,resources=podlabelerpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=mesh.knabben.github.com,resources=podlabelerpolicies/status,verbs=update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

func (r *PodLabelerPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("pod", req.Name, "namespace", req.Namespace)

	// Step 1: Fetch pod from cache. NotFound is transient — skip silently.
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("pod not found in cache, skipping")
			return ctrl.Result{}, nil
		}
		reconcileErrorsTotal.WithLabelValues("get_pod").Inc()
		return ctrl.Result{}, fmt.Errorf("get pod: %w", err)
	}

	// Step 2: List all PodLabelerPolicies from cache.
	var policyList meshv1alpha1.PodLabelerPolicyList
	if err := r.List(ctx, &policyList); err != nil {
		reconcileErrorsTotal.WithLabelValues("list_policies").Inc()
		return ctrl.Result{}, fmt.Errorf("list PodLabelerPolicies: %w", err)
	}

	// Step 3: Collect container images from the pod.
	images := podImages(&pod)

	// Step 4: Evaluate each policy against pod images.
	type match struct {
		name string
		tier string
	}
	var matches []match

	for _, policy := range policyList.Items {
		m, err := matcher.Compile(policy.Spec.ImagePattern)
		if err != nil {
			// Invalid pattern: fail-closed — leave pod label unchanged.
			log.Error(err, "invalid imagePattern, skipping policy", "policy", policy.Name)
			policyEvaluationsTotal.WithLabelValues("invalid_pattern").Inc()
			continue
		}
		if m.MatchesAnyImage(images) {
			policyEvaluationsTotal.WithLabelValues("match").Inc()
			matches = append(matches, match{name: policy.Name, tier: policy.Spec.Tier})
		} else {
			policyEvaluationsTotal.WithLabelValues("no_match").Inc()
		}
	}

	// Step 5: Resolve target tier (alphabetical tie-break).
	computedTier := ""
	if len(matches) > 0 {
		sort.Slice(matches, func(i, j int) bool { return matches[i].name < matches[j].name })
		computedTier = matches[0].tier

		if len(matches) > 1 {
			firstTier := matches[0].tier
			for _, m := range matches[1:] {
				if m.tier != firstTier {
					msg := fmt.Sprintf("conflicting policies: %s=%s wins over %s=%s",
						matches[0].name, firstTier, m.name, m.tier)
					r.Recorder.Event(&pod, corev1.EventTypeWarning, "TierConflict", msg)
					log.Info("tier conflict detected", "winner", matches[0].name, "loser", m.name)
					break
				}
			}
		}
	}

	// Step 6: Diff-gate — skip write if label already correct.
	currentTier := pod.Labels[tierLabel]
	if currentTier == computedTier {
		labelsSkippedTotal.WithLabelValues(pod.Namespace).Inc()
		return ctrl.Result{}, nil
	}

	// Step 7: Build SSA patch and apply.
	oldTier := currentTier
	if oldTier == "" {
		oldTier = "<none>"
	}

	if err := r.applyTierLabel(ctx, &pod, computedTier); err != nil {
		reconcileErrorsTotal.WithLabelValues("patch_pod").Inc()
		return ctrl.Result{}, fmt.Errorf("apply tier label: %w", err)
	}

	// Step 8: Emit event and metrics.
	if computedTier == "" {
		msg := fmt.Sprintf("removed tier label (was %s)", oldTier)
		r.Recorder.Event(&pod, corev1.EventTypeNormal, "TierLabelRemoved", msg)
		log.Info("tier label removed", "old_tier", oldTier, "namespace", pod.Namespace)
	} else {
		policyName := ""
		if len(matches) > 0 {
			policyName = matches[0].name
		}
		msg := fmt.Sprintf("applied tier=%s (policy: %s, was: %s)", computedTier, policyName, oldTier)
		r.Recorder.Event(&pod, corev1.EventTypeNormal, "TierLabelApplied", msg)
		log.Info("tier label applied",
			"old_tier", oldTier,
			"new_tier", computedTier,
			"policy", policyName,
			"namespace", pod.Namespace,
		)
		labelsAppliedTotal.WithLabelValues(computedTier, pod.Namespace).Inc()
	}

	return ctrl.Result{}, nil
}

// applyTierLabel writes the tier label to the pod via server-side apply.
// If computedTier is empty, the tier key is omitted so it is removed.
func (r *PodLabelerPolicyReconciler) applyTierLabel(ctx context.Context, pod *corev1.Pod, computedTier string) error {
	labels := map[string]string{}
	if computedTier != "" {
		labels[tierLabel] = computedTier
	}

	patch := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Labels:    labels,
		},
	}
	return r.Patch(ctx, patch, client.Apply, //nolint:staticcheck
		client.FieldOwner(fieldManager),
		client.ForceOwnership,
	)
}

// SetupWithManager wires the reconciler with a primary Pod watch and a
// secondary PodLabelerPolicy watch that fans out to all pods.
func (r *PodLabelerPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Watches(
			&meshv1alpha1.PodLabelerPolicy{},
			handler.EnqueueRequestsFromMapFunc(r.mapPolicyToPods),
		).
		Named("pod-tier-labeler").
		Complete(r)
}

// mapPolicyToPods lists all pods and enqueues a reconcile request for each.
func (r *PodLabelerPolicyReconciler) mapPolicyToPods(ctx context.Context, _ client.Object) []reconcile.Request {
	var podList corev1.PodList
	if err := r.List(ctx, &podList); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(podList.Items))
	for _, pod := range podList.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		})
	}
	return reqs
}

// podImages returns all unique container image references from the pod spec.
func podImages(pod *corev1.Pod) []string {
	seen := make(map[string]struct{})
	var images []string
	for _, c := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
		if _, ok := seen[c.Image]; !ok {
			seen[c.Image] = struct{}{}
			images = append(images, c.Image)
		}
	}
	return images
}
