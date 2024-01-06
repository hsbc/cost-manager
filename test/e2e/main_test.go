package e2e

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	kindClusterName = "cost-manager"
)

var ()

func TestMain(m *testing.M) {
	ctx := context.Background()
	logger := log.FromContext(ctx).WithName("e2e")

	// Parse flags
	// image := flag.String("test.image", "docker.io/dippynark/cost-manager:latest", "Image to test")
	// helmChartPath := flag.String("test.helm-chart-path", "./charts/cost-manager", "Path to Helm chart")
	// flag.Parse()

	// Setup test suite
	err := setup()
	if err != nil {
		logger.Error(err, "failed to setup E2E test suite")
		os.Exit(1)
	}

	code := m.Run()

	// Teardown test suite
	// err = teardown()
	// if err != nil {
	// 	logger.Error(err, "failed to teardown E2E test suite")
	// 	os.Exit(1)
	// }

	os.Exit(code)
}

func setup() error {
	// Cleanup from any previous failed runs
	err := deleteKindCluster()
	if err != nil {
		return err
	}

	// Create kind cluster
	err = createKindCluster()
	if err != nil {
		return err
	}

	// Install cost-manager
	err = installCostManager()
	if err != nil {
		return err
	}

	return nil
}

func teardown() error {
	// Cleanup from any previous failed runs
	err := deleteKindCluster()
	if err != nil {
		return err
	}

	return nil
}

func createKindCluster() error {
	return runCommand("kind", "create", "cluster", "--name", kindClusterName)
}

func deleteKindCluster() error {
	return runCommand("kind", "delete", "cluster", "--name", kindClusterName)
}

func installCostManager() error {
	return runCommand("helm", "upgrade", "--install",
		"cost-manager", "../../charts/cost-manager",
		"--namespace", "cost-manager", "--create-namespace",
		"--wait", "--timeout", "2m")
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}