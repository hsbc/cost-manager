package kubernetes

import (
	"github.com/pkg/errors"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

// NewScheme creates a new scheme:
// https://book.kubebuilder.io/cronjob-tutorial/gvks.html#err-but-whats-that-scheme-thing
func NewScheme() (*runtime.Scheme, error) {
	newScheme := runtime.NewScheme()

	err := scheme.AddToScheme(newScheme)
	if err != nil {
		return newScheme, errors.Wrap(err, "failed to add core kinds to scheme")
	}

	err = monitoringv1.AddToScheme(newScheme)
	if err != nil {
		return newScheme, errors.Wrap(err, "failed to add monitoring kinds to scheme")
	}

	return newScheme, nil
}
