package kubernetes

import (
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func NewClient() (client.WithWatch, *rest.Config, error) {
	scheme, err := NewScheme()
	if err != nil {
		return nil, nil, err
	}

	restConfig := config.GetConfigOrDie()
	kubeClient, err := client.NewWithWatch(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, err
	}

	return kubeClient, restConfig, nil
}
