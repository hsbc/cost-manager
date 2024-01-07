package kubernetes

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apiwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/watch"
	"k8s.io/kubectl/pkg/util/podutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func WaitForAnyReadyPod(ctx context.Context, kubeClient client.WithWatch, opts ...client.ListOption) (*corev1.Pod, error) {
	listerWatcher := NewListerWatcher(ctx, kubeClient, &corev1.PodList{}, opts...)
	condition := func(event apiwatch.Event) (bool, error) {
		pod, err := ParseWatchEventObject[*corev1.Pod](event)
		if err != nil {
			return false, err
		}
		return podutils.IsPodReady(pod), nil
	}
	event, err := watch.UntilWithSync(ctx, listerWatcher, &corev1.Pod{}, nil, condition)
	if err != nil {
		return nil, err
	}
	return event.Object.(*corev1.Pod), nil
}
