package kubernetes

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewWatcher(ctx context.Context, kubeClient client.WithWatch, objectList client.ObjectList, opts ...client.ListOption) cache.Watcher {
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		listOptions := &client.ListOptions{Raw: &options}
		// Apply default options to caller options
		listOptions.ApplyOptions(opts)
		return kubeClient.Watch(ctx, objectList, listOptions)
	}
	return &cache.ListWatch{
		WatchFunc: watchFunc,
	}
}

func NewListerWatcher(ctx context.Context, kubeClient client.WithWatch, objectList client.ObjectList, opts ...client.ListOption) cache.ListerWatcher {
	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		listOptions := &client.ListOptions{Raw: &options}
		// Apply default options to caller options
		listOptions.ApplyOptions(opts)
		err := kubeClient.List(ctx, objectList, listOptions)
		if err != nil {
			return nil, err
		}
		return objectList, nil
	}
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		listOptions := &client.ListOptions{Raw: &options}
		// Apply default options to caller options
		listOptions.ApplyOptions(opts)
		return kubeClient.Watch(ctx, objectList, listOptions)
	}
	return &cache.ListWatch{
		ListFunc:  listFunc,
		WatchFunc: watchFunc,
	}
}

// ParseWatchEventObject determines if the specified watch event is an error and if so returns an
// error and otherwise asserts and returns an object of the expected type
func ParseWatchEventObject[T runtime.Object](event watch.Event) (T, error) {
	var runtimeObject T
	if event.Type == watch.Error {
		if status, ok := event.Object.(*metav1.Status); ok {
			return runtimeObject, fmt.Errorf("watch failed with error: %s", status.Message)
		}
		return runtimeObject, fmt.Errorf("watch failed with error: %+v", event.Object)
	}
	var ok bool
	runtimeObject, ok = event.Object.(T)
	if !ok {
		return runtimeObject, errors.New("failed to type assert runtime object")
	}
	return runtimeObject, nil
}
