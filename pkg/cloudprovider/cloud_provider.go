package cloudprovider

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

// CloudProvider contains the functions for interacting with a cloud provider
type CloudProvider interface {
	// IsSpotInstance determines whether the underlying instance of the Node is a spot instance
	IsSpotInstance(ctx context.Context, node *corev1.Node) (bool, error)
	// DeleteInstance should drain connections from external load balancers to the Node and then
	// delete the underlying instance. Implementations can assume that before this function is
	// called the Node has already been modified to ensure that the KCCM service controller will
	// eventually remove the Node from load balancing although this process may still be in progress
	// when this function is called:
	// https://kubernetes.io/docs/concepts/architecture/cloud-controller/#service-controller
	DeleteInstance(ctx context.Context, node *corev1.Node) error
}
