package controller_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	labelingv1alpha1 "github.com/knabben/istio-poc/api/v1alpha1"
	"github.com/knabben/istio-poc/controller"
	"github.com/stretchr/testify/require"
)

var (
	k8sClient client.Client
	scheme    *runtime.Scheme
	cfg       *rest.Config
)

func TestMain(m *testing.M) {
	scheme = runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := labelingv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	// Register the meta types (CreateOptions, ListOptions, …) for our custom
	// group so that controller-runtime's paramCodec can encode request options.
	// Without this, client.Create() fails with "v1.CreateOptions is not suitable
	// for converting to 'labeling.knabben.dev/v1alpha1'".
	metav1.AddToGroupVersion(scheme, labelingv1alpha1.GroupVersion)

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{"../manifests"},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = env.Start()
	if err != nil {
		panic(err)
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	code := m.Run()
	_ = env.Stop()
	os.Exit(code)
}

// TestCoreLabeling verifies the happy-path labeling loop: a pod whose primary
// container image matches a PodLabelerPolicy receives the policy's labels after
// a single reconcile cycle.
//
// Expected result on Act I: PASS — the sequential, single-replica path has no
// race condition.
func TestCoreLabeling(t *testing.T) {
	ctx := context.Background()

	policy := &labelingv1alpha1.PodLabelerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "core-policy"},
		Spec: labelingv1alpha1.PodLabelerPolicySpec{
			ImagePattern: "app1:*",
			Labels:       map[string]string{"tier": "high"},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, policy))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, policy) })

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "core-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "localhost/team/app1:v2"},
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, pod))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pod) })

	r := &controller.PodReconciler{Client: k8sClient, Scheme: scheme}
	_, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "core-pod"},
	})
	require.NoError(t, err)

	result := &corev1.Pod{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "core-pod"}, result))
	require.Equal(t, "high", result.Labels["tier"])
}

// TestBug1_LostUpdate exposes Bug 1: the reconciler uses r.Update() which
// sends a full PUT carrying the pod's in-memory resourceVersion. Two concurrent
// writers both read the pod at the same resourceVersion; whichever write lands
// second gets a 409 Conflict that is returned as a terminal error — its labels
// are silently dropped.
//
// The test replicates this exactly by giving both goroutines the same stale
// pod snapshot (identical resourceVersion) and writing different labels.
// This is identical to what happens inside PodReconciler.Reconcile when two
// replicas (Bug 3) call it concurrently.
//
// Expected result on Act I: FAIL — only one label survives because one Update
// gets a 409 Conflict and the error is propagated without retry.
func TestBug1_LostUpdate(t *testing.T) {
	ctx := context.Background()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "bug1-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "localhost/team/app1:v1"},
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, pod))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pod) })

	// Fetch the pod to obtain its current resourceVersion.
	current := &corev1.Pod{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "bug1-pod"}, current))

	// Each copy carries the same resourceVersion as the one the reconciler
	// reads from the informer cache — the root cause of the lost-update bug.
	copy1 := current.DeepCopy()
	if copy1.Labels == nil {
		copy1.Labels = map[string]string{}
	}
	copy1.Labels["policy-a"] = "applied"

	copy2 := current.DeepCopy()
	if copy2.Labels == nil {
		copy2.Labels = map[string]string{}
	}
	copy2.Labels["policy-b"] = "applied"

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_ = k8sClient.Update(ctx, copy1)
	}()
	go func() {
		defer wg.Done()
		_ = k8sClient.Update(ctx, copy2)
	}()

	wg.Wait()

	result := &corev1.Pod{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "bug1-pod"}, result))

	// Both labels should survive if the controller used server-side apply.
	// With r.Update() and concurrent writes, only one label survives.
	require.Equal(t,
		map[string]string{"policy-a": "applied", "policy-b": "applied"},
		result.Labels,
		"expected both labels to be applied, but one was lost due to concurrent Update (Bug 1)")
}

// TestBug2_StaleCache exposes Bug 2: the reconciler returns the NotFound error
// from r.Get() as a terminal error. A pod that has been deleted or whose cache
// entry has not yet propagated causes the request to be dropped after exponential
// backoff — the pod is never labeled.
//
// Expected result on Act I: FAIL — the reconciler returns the NotFound error
// instead of nil.
func TestBug2_StaleCache(t *testing.T) {
	ctx := context.Background()

	r := &controller.PodReconciler{Client: k8sClient, Scheme: scheme}
	_, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "nonexistent-pod"},
	})

	// Correct behavior: a missing pod is a benign transient condition (stale
	// informer cache or already-deleted pod). The reconciler should return nil.
	// Bug 2 returns the NotFound error instead, causing workqueue backoff and
	// eventual request drop.
	require.NoError(t, err,
		"expected nil error for a missing pod (transient cache miss), but reconciler returned: %v", err)
}

// TestBug3_NoLease exposes Bug 3: the manager is configured with
// LeaderElection: false, so no Lease object is ever created. When deployed
// with replicas: 2, both replicas reconcile independently, amplifying Bug 1
// and causing a continuous Istio config-push storm.
//
// The observable symptom is the absence of the coordination.k8s.io/v1 Lease
// that a leader-election-enabled manager would create.
//
// Expected result on Act I: FAIL — no Lease is found in the namespace.
func TestBug3_NoLease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                  scheme,
		Metrics:                 metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress:  "0",
		LeaderElection:          false, // Bug 3: intentionally disabled
		LeaderElectionID:        "podlabeler-bug3.knabben.dev",
		LeaderElectionNamespace: "default",
	})
	require.NoError(t, err)

	go func() {
		_ = mgr.Start(ctx)
	}()

	// Give the manager time to start and (if leader election were enabled)
	// create the Lease object.
	time.Sleep(2 * time.Second)

	leases := &coordinationv1.LeaseList{}
	require.NoError(t, k8sClient.List(ctx, leases, client.InNamespace("default")))

	// With LeaderElection: true the manager creates exactly one Lease.
	// With Bug 3 (LeaderElection: false) no Lease is created — len == 0.
	require.Len(t, leases.Items, 1,
		"expected 1 leader-election Lease in default namespace (got %d) — Bug 3: LeaderElection is false", len(leases.Items))
}
