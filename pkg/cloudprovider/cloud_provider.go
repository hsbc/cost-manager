package cloudprovider

import (
	"context"
	"fmt"

	"github.com/hsbc/cost-manager/pkg/cloudprovider/fake"
	"github.com/hsbc/cost-manager/pkg/cloudprovider/gcp"
	corev1 "k8s.io/api/core/v1"
)

const (
	fakeCloudProviderName = "fake"
	gcpCloudProviderName  = "gcp"
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

// NewCloudProvider returns a new CloudProvider instance
func NewCloudProvider(ctx context.Context, cloudProviderName string) (CloudProvider, error) {
	switch cloudProviderName {
	case fakeCloudProviderName:
		return &fake.CloudProvider{}, nil
	case gcpCloudProviderName:
		return gcp.NewCloudProvider(ctx)
	default:
		return nil, fmt.Errorf("unknown cloud provider: %s", cloudProviderName)
	}
}
