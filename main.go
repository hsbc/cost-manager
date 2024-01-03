package main

import (
	"flag"
	"os"

	costmanagerconfig "github.com/hsbc/cost-manager/pkg/config"
	"github.com/hsbc/cost-manager/pkg/controller"
	"github.com/hsbc/cost-manager/pkg/kubernetes"
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

const (
	costManagerName = "cost-manager"
)

func main() {
	// Parse flags
	costManagerConfigFilePath := flag.String("config", "", "Configuration file path")
	flag.Parse()

	// Create signal handling context and logger
	ctx := signals.SetupSignalHandler()
	logger := log.FromContext(ctx).WithName(costManagerName)
	ctx = log.IntoContext(ctx, logger)

	// Load configuration
	logger.Info("Loading configuration")
	costManagerConfig, err := costmanagerconfig.Load(*costManagerConfigFilePath)
	if err != nil {
		logger.Error(err, "failed to load configuration")
		os.Exit(1)
	}

	// Create new scheme
	scheme, err := kubernetes.NewScheme()
	if err != nil {
		logger.Error(err, "failed to create new scheme")
		os.Exit(1)
	}

	// Setup controller manager
	logger.Info("Setting up controller manager")
	restConfig := config.GetConfigOrDie()
	// Disable client-side rate-limiting: https://github.com/kubernetes/kubernetes/issues/111880
	restConfig.QPS = -1
	mgr, err := ctrl.NewManager(restConfig, manager.Options{Scheme: scheme})
	if err != nil {
		logger.Error(err, "failed to setup controller manager")
		os.Exit(1)
	}

	// Setup controllers
	err = controller.SetupWithManager(ctx, mgr, costManagerConfig)
	if err != nil {
		logger.Error(err, "failed to setup controllers with manager")
		os.Exit(1)
	}

	// Start controller manager
	logger.Info("Starting controller manager")
	err = mgr.Start(ctx)
	if err != nil {
		logger.Error(err, "failed to start controller manager")
		os.Exit(1)
	}
}
