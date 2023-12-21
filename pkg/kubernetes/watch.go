package kubernetes

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

// parseWatchEventObject determines if the specified watch event is an error and if so returns an
// error and otherwise asserts and returns an object of the expected type
func parseWatchEventObject[T runtime.Object](event watch.Event) (T, error) {
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
