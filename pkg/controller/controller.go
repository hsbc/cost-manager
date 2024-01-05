package controller

import (
	"context"

	"github.com/hsbc/cost-manager/pkg/api/v1alpha1"
	"github.com/hsbc/cost-manager/pkg/cloudprovider"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/controller-manager/app"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	// The following link describes how controller names should be treated:
	// https://github.com/kubernetes/cloud-provider/blob/30270693811ff7d3c4646509eed7efd659332e72/names/controller_names.go
	AllControllerNames = []string{
		spotMigratorControllerName,
		podSafeToEvictAnnotatorControllerName,
	}
	// All controllers are disabled by default
	disabledByDefaultControllerNames = sets.NewString(AllControllerNames...)
)

// SetupWithManager sets up the controllers with the manager
func SetupWithManager(ctx context.Context, mgr ctrl.Manager, config *v1alpha1.CostManagerConfiguration) error {
	// Create clientset
	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return errors.Wrapf(err, "failed to create clientset")
	}

	// Setup controllers
	for _, controllerName := range AllControllerNames {
		if app.IsControllerEnabled(controllerName, disabledByDefaultControllerNames, config.Controllers) {
			switch controllerName {
			case spotMigratorControllerName:
				// Instantiate cloud provider
				cloudProvider, err := cloudprovider.NewCloudProvider(ctx, config.CloudProvider.Name)
				if err != nil {
					return errors.Wrapf(err, "failed to instantiate cloud provider")
				}
				err = mgr.Add(&spotMigrator{
					Clientset:     clientset,
					CloudProvider: cloudProvider,
				})
				if err != nil {
					return errors.Wrapf(err, "failed to setup %s", spotMigratorControllerName)
				}
			case podSafeToEvictAnnotatorControllerName:
				err := (&podSafeToEvictAnnotator{
					Client: mgr.GetClient(),
					Config: config.PodSafeToEvictAnnotator,
				}).SetupWithManager(mgr)
				if err != nil {
					return errors.Wrapf(err, "failed to setup %s", podSafeToEvictAnnotatorControllerName)
				}
			default:
				return errors.Errorf("unknown controller: %s", controllerName)
			}
		}
	}

	return nil
}
