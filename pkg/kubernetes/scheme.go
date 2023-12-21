package kubernetes

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/scale/scheme"
)

// NewScheme creates a new scheme:
// https://book.kubebuilder.io/cronjob-tutorial/gvks.html#err-but-whats-that-scheme-thing
func NewScheme() (*runtime.Scheme, error) {
	newScheme := runtime.NewScheme()

	err := scheme.AddToScheme(newScheme)
	if err != nil {
		return newScheme, errors.Wrap(err, "failed to add core kinds to scheme")
	}

	return newScheme, nil
}
