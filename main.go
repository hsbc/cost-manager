package main

import (
	"flag"
	"os"

	"github.com/hsbc/cost-manager/pkg/cloudprovider"
	"github.com/hsbc/cost-manager/pkg/cloudprovider/fake"
	"github.com/hsbc/cost-manager/pkg/cloudprovider/gcp"
	"github.com/hsbc/cost-manager/pkg/controller"
	"github.com/hsbc/cost-manager/pkg/kubernetes"
	"github.com/hsbc/cost-manager/pkg/logging"
	clientgo "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

func init() {
	log.SetLogger(zap.New())
}

var (
	cloudProviderName string
)

func main() {
	// Parse flags
	flag.StringVar(&cloudProviderName, "cloud-provider", "", "Cloud provider")
	flag.Parse()
	if cloudProviderName == "" {
		logging.Logger.Error(nil, "cloud provider is required")
		os.Exit(1)
	}

	// Create new scheme
	scheme, err := kubernetes.NewScheme()
	if err != nil {
		logging.Logger.Error(err, "failed to create new scheme")
		os.Exit(1)
	}

	// Setup controller manager
	logging.Logger.Info("Setting up controller manager")
	mgr, err := ctrl.NewManager(config.GetConfigOrDie(), manager.Options{Scheme: scheme})
	if err != nil {
		logging.Logger.Error(err, "failed to setup controller manager")
		os.Exit(1)
	}

	// Create clientset
	restConfig, err := config.GetConfig()
	if err != nil {
		logging.Logger.Error(err, "failed to get REST config")
		os.Exit(1)
	}
	// Disable client-side rate-limiting: https://github.com/kubernetes/kubernetes/issues/111880
	restConfig.QPS = -1
	clientset, err := clientgo.NewForConfig(restConfig)
	if err != nil {
		logging.Logger.Error(err, "failed to create clientset")
		os.Exit(1)
	}

	// Create context
	ctx := signals.SetupSignalHandler()

	// Instantiate cloud provider
	var cloudProvider cloudprovider.CloudProvider
	if cloudProviderName == "gcp" {
		cloudProvider, err = gcp.NewCloudProvider(ctx)
		if err != nil {
			logging.Logger.Error(err, "failed to create cloud provider")
			os.Exit(1)
		}
	} else if cloudProviderName == "fake" {
		cloudProvider = &fake.CloudProvider{}
	} else {
		logging.Logger.Error(err, "unrecognised cloud provider: "+cloudProviderName)
		os.Exit(1)
	}

	// Setup spot-migrator
	if err := mgr.Add(&controller.SpotMigrator{
		Clientset:     clientset,
		CloudProvider: cloudProvider,
	}); err != nil {
		logging.Logger.Error(err, "failed to setup spot-migrator")
		os.Exit(1)
	}

	// Start controller manager
	logging.Logger.Info("Starting controller manager")
	if err := mgr.Start(ctx); err != nil {
		logging.Logger.Error(err, "failed to start controller manager")
		os.Exit(1)
	}
}
