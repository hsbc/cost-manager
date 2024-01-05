package controller

import (
	"context"
	"testing"

	"github.com/hsbc/cost-manager/pkg/api/v1alpha1"
	"github.com/hsbc/cost-manager/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestPodSafeToEvictAnnotatorReconcile(t *testing.T) {
	name := "foo"
	namespace := "bar"
	tests := map[string]struct {
		pod            *corev1.Pod
		namespace      *corev1.Namespace
		config         *v1alpha1.PodSafeToEvictAnnotator
		shouldAnnotate bool
	}{
		"annotationMissing": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			},
			shouldAnnotate: true,
		},
		"annotationFalse": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"cluster-autoscaler.kubernetes.io/safe-to-evict": "false",
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			},
			shouldAnnotate: false,
		},
		"annotationMissingWithNilNamespaceSelector": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			},
			config:         &v1alpha1.PodSafeToEvictAnnotator{},
			shouldAnnotate: true,
		},
		"annotationMissingWithMatchingNamespaceSelector": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "kube-system",
					},
				},
			},
			config: &v1alpha1.PodSafeToEvictAnnotator{
				NamespaceSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "kubernetes.io/metadata.name",
							Operator: "In",
							Values: []string{
								"kube-system",
							},
						},
					},
				},
			},
			shouldAnnotate: true,
		},
		"annotationMissingWithNonMatchingNamespaceSelector": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "kube-system",
					},
				},
			},
			config: &v1alpha1.PodSafeToEvictAnnotator{
				NamespaceSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "kubernetes.io/metadata.name",
							Operator: "In",
							Values: []string{
								"kube-public",
							},
						},
					},
				},
			},
			shouldAnnotate: false,
		},
		"annotationMissingWithMissingNamespace": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
			namespace:      nil,
			shouldAnnotate: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Create fake client
			scheme, err := kubernetes.NewScheme()
			require.Nil(t, err)
			objects := []client.Object{test.pod}
			if test.namespace != nil {
				objects = append(objects, test.namespace)
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

			// Setup controller
			podSafeToEvictAnnotator := &podSafeToEvictAnnotator{
				Client: client,
				Config: test.config,
			}

			// Run reconciliation
			ctx := context.Background()
			_, err = podSafeToEvictAnnotator.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: test.pod.Name, Namespace: test.pod.Namespace}})
			require.Nil(t, err)

			// Determine whether the Pod has been annotated
			pod := &corev1.Pod{}
			err = client.Get(ctx, types.NamespacedName{Name: test.pod.Name, Namespace: test.pod.Namespace}, pod)
			require.Nil(t, err)
			annotated := true
			if pod.Annotations == nil {
				annotated = false
			} else {
				value, ok := pod.Annotations["cluster-autoscaler.kubernetes.io/safe-to-evict"]
				if !ok || value != "true" {
					annotated = false
				}
			}
			require.Equal(t, test.shouldAnnotate, annotated)
		})
	}
}
