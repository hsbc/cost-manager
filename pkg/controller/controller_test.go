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

func TestSetupAllControllersWithManager(t *testing.T) {
	// Create manager
	scheme, err := kubernetes.NewScheme()
	require.Nil(t, err)
	mgr, err := ctrl.NewManager(&rest.Config{}, manager.Options{Scheme: scheme})
	require.Nil(t, err)

	// Setup manager with all controllers...
	config := &v1alpha1.CostManagerConfiguration{
		CloudProvider: v1alpha1.CloudProvider{
			Name: cloudprovider.FakeCloudProviderName,
		},
		Controllers: AllControllerNames,
	}
	err = SetupWithManager(context.Background(), mgr, config)
	// ...and make sure there are no errors
	require.Nil(t, err)
}
