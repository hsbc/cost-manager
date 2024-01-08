package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/hsbc/cost-manager/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestPodSafeToEvictAnnotator tests that pod-safe-to-evict-annotator correctly annotates all Pods
func TestPodSafeToEvictAnnotator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	kubeClient, _, err := kubernetes.NewClient()
	require.Nil(t, err)

	// Wait until all Pods have expected safe-to-evict annotation
	for {
		success, err := allPodsHaveExpectedSafeToEvictAnnotation(ctx, kubeClient)
		require.Nil(t, err)
		if success {
			// Make sure condition still holds after 2 seconds
			time.Sleep(2 * time.Second)
			stillSuccess, err := allPodsHaveExpectedSafeToEvictAnnotation(ctx, kubeClient)
			require.Nil(t, err)
			require.True(t, stillSuccess)
			break
		}
		time.Sleep(time.Second)
	}
}

func allPodsHaveExpectedSafeToEvictAnnotation(ctx context.Context, kubeClient client.WithWatch) (bool, error) {
	podList := &corev1.PodList{}
	err := kubeClient.List(ctx, podList)
	if err != nil {
		return false, err
	}
	for _, pod := range podList.Items {
		// kube-system Pods should have the annotation...
		if pod.Namespace == "kube-system" && !hasSafeToEvictAnnotation(&pod) {
			return false, nil
		}
		// ...all other Pods should not have the annotation
		if pod.Namespace != "kube-system" && hasSafeToEvictAnnotation(&pod) {
			return false, nil
		}
	}
	return true, nil
}

func hasSafeToEvictAnnotation(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	value, ok := pod.Annotations["cluster-autoscaler.kubernetes.io/safe-to-evict"]
	if ok && value == "true" {
		return true
	}
	return false
}
