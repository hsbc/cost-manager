package cloudprovider

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

// CloudProvider contains the functions for interacting with a cloud provider
type CloudProvider interface {
	// DrainExternalLoadBalancerConnections drains connections from external load balancers to a
	// Kubernetes Node to allow it to be safely deleted
	DrainExternalLoadBalancerConnections(ctx context.Context, node *corev1.Node) error
	// DeleteMachine deletes the underlying machine of a Node
	DeleteMachine(ctx context.Context, node *corev1.Node) error
}
