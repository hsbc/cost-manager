package cloudprovider

import (
	"context"
	"fmt"

	"github.com/hsbc/cost-manager/pkg/cloudprovider/fake"
	"github.com/hsbc/cost-manager/pkg/cloudprovider/gcp"
	corev1 "k8s.io/api/core/v1"
)

const (
	FakeCloudProviderName = "fake"
	GCPCloudProviderName  = "gcp"
)

// CloudProvider contains the functions for interacting with a cloud provider
type CloudProvider interface {
	// IsSpotInstance determines whether the underlying instance of the Node is a spot instance
	IsSpotInstance(ctx context.Context, node *corev1.Node) (bool, error)
	// DeleteInstance should drain connections from external load balancers to the Node and then
	// delete the underlying instance. Implementations can assume that before this function is
	// called Pods have already been drained from the Node and it has been tainted with
	// ToBeDeletedByClusterAutoscaler to fail kube-proxy health checks as described in KEP-3836:
	// https://github.com/kubernetes/enhancements/tree/27ef0d9a740ae5058472aac4763483f0e7218c0e/keps/sig-network/3836-kube-proxy-improved-ingress-connectivity-reliability
	DeleteInstance(ctx context.Context, node *corev1.Node) error
}

// NewCloudProvider returns a new CloudProvider instance
func NewCloudProvider(ctx context.Context, cloudProviderName string) (CloudProvider, error) {
	switch cloudProviderName {
	case FakeCloudProviderName:
		return &fake.CloudProvider{}, nil
	case GCPCloudProviderName:
		return gcp.NewCloudProvider(ctx)
	default:
		return nil, fmt.Errorf("unknown cloud provider: %s", cloudProviderName)
	}
}
