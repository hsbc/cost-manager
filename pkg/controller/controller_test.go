package controller

import (
	"context"
	"testing"

	"github.com/hsbc/cost-manager/pkg/api/v1alpha1"
	"github.com/hsbc/cost-manager/pkg/cloudprovider"
	"github.com/hsbc/cost-manager/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestSetupWithManager(t *testing.T) {
	tests := map[string]struct {
		config        *v1alpha1.CostManagerConfiguration
		shouldSucceed bool
	}{
		"allControllers": {
			config: &v1alpha1.CostManagerConfiguration{
				CloudProvider: v1alpha1.CloudProvider{
					Name: cloudprovider.FakeCloudProviderName,
				},
				Controllers: AllControllerNames,
			},
			shouldSucceed: true,
		},
		"allControllersWithoutCloudProvider": {
			config: &v1alpha1.CostManagerConfiguration{
				Controllers: AllControllerNames,
			},
			shouldSucceed: false,
		},
		"withoutCloudProvider": {
			// Configure a controller that does not interact with the cloud provider
			config: &v1alpha1.CostManagerConfiguration{
				Controllers: []string{podSafeToEvictAnnotatorControllerName},
			},
			shouldSucceed: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Create manager
			scheme, err := kubernetes.NewScheme()
			require.Nil(t, err)
			mgr, err := ctrl.NewManager(&rest.Config{}, manager.Options{Scheme: scheme})
			require.Nil(t, err)

			// Setup manager...
			err = SetupWithManager(context.Background(), mgr, test.config)
			// ...and verify success
			if test.shouldSucceed {
				require.Nil(t, err)
			} else {
				require.NotNil(t, err)
			}
		})
	}
}
