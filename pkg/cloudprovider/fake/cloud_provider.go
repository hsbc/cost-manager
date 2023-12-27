package fake

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

const (
	SpotInstanceLabelKey   = "is-spot-instance"
	SpotInstanceLabelValue = "true"
)

// CloudProvider is a fake implementation of the cloudprovider.CloudProvider interface for testing
type CloudProvider struct{}

func (fake *CloudProvider) DeleteInstance(ctx context.Context, node *corev1.Node) error {
	return nil
}

func (fake *CloudProvider) IsSpotInstance(ctx context.Context, node *corev1.Node) (bool, error) {
	if node.Labels == nil {
		return false, nil
	}
	value, ok := node.Labels[SpotInstanceLabelKey]
	if !ok {
		return false, nil
	}
	return value == SpotInstanceLabelValue, nil
}
