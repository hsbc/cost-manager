package controller

import (
	"context"
	"testing"

	"github.com/hsbc/cost-manager/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestPodSafeToEvictAnnotatorReconcileAddsAnnotation(t *testing.T) {
	name := "foo"
	namespace := "bar"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	// Create fake client
	scheme, err := kubernetes.NewScheme()
	require.Nil(t, err)
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	// Setup controller
	podSafeToEvictAnnotator := &podSafeToEvictAnnotator{
		Client: client,
	}
	ctx := context.Background()
	_, err = podSafeToEvictAnnotator.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}})
	require.Nil(t, err)

	// Verify that the Pod has been annotated
	err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, pod)
	require.Nil(t, err)
	value, ok := pod.Annotations["cluster-autoscaler.kubernetes.io/safe-to-evict"]
	require.True(t, ok)
	require.Equal(t, "true", value)
}

func TestPodSafeToEvictAnnotatorReconcileDoesNotUpdateAnnotation(t *testing.T) {
	name := "foo"
	namespace := "bar"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"cluster-autoscaler.kubernetes.io/safe-to-evict": "false",
			},
		},
	}

	// Create fake client
	scheme, err := kubernetes.NewScheme()
	require.Nil(t, err)
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	// Setup controller
	podSafeToEvictAnnotator := &podSafeToEvictAnnotator{
		Client: client,
	}
	ctx := context.Background()
	_, err = podSafeToEvictAnnotator.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}})
	require.Nil(t, err)

	// Verify that the Pod has been annotated
	err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, pod)
	require.Nil(t, err)
	value, ok := pod.Annotations["cluster-autoscaler.kubernetes.io/safe-to-evict"]
	require.True(t, ok)
	require.Equal(t, "false", value)
}
