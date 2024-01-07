package e2e

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/hsbc/cost-manager/pkg/cloudprovider"
	cloudproviderfake "github.com/hsbc/cost-manager/pkg/cloudprovider/fake"
	"github.com/hsbc/cost-manager/pkg/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	kindClusterName = "cost-manager"
)

func init() {
	log.SetLogger(zap.New())
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	logger := log.FromContext(ctx).WithName("e2e")

	// Parse flags
	image := flag.String("test.image", "cost-manager", "Local Docker image to test")
	helmChartPath := flag.String("test.helm-chart-path", "../charts/cost-manager", "Path to Helm chart")
	flag.Parse()

	// Setup test suite
	err := setup(ctx, *image, *helmChartPath)
	if err != nil {
		logger.Error(err, "failed to setup E2E test suite")
		os.Exit(1)
	}

	code := m.Run()

	// If the E2E tests failed then we print some debug information
	if code > 0 {
		err := printDebugInformation()
		if err != nil {
			logger.Error(err, "failed to print debug information")
			os.Exit(1)
		}
	}

	// Teardown test suite
	err = teardown()
	if err != nil {
		logger.Error(err, "failed to teardown E2E test suite")
		os.Exit(1)
	}

	os.Exit(code)
}

func setup(ctx context.Context, image, helmChartPath string) error {
	// Cleanup from any previous failed runs
	err := runCommand("kind", "delete", "cluster", "--name", kindClusterName)
	if err != nil {
		return err
	}

	// Create kind cluster
	err = createKindCluster()
	if err != nil {
		return err
	}

	// Load image into kind cluster
	err = runCommand("kind", "load", "docker-image", image, "--name", kindClusterName)
	if err != nil {
		return err
	}

	// Install CRDs
	err = runCommand("kubectl", "apply",
		"-f", "https://raw.githubusercontent.com/kubernetes/autoscaler/5469d7912072c1070eedc680c89e27d46b8f4f82/vertical-pod-autoscaler/deploy/vpa-v1-crd-gen.yaml",
		"-f", "https://raw.githubusercontent.com/prometheus-community/helm-charts/d616961860a0248f77a2783923511550fad23569/charts/kube-prometheus-stack/charts/crds/crds/crd-prometheusrules.yaml",
		"-f", "https://raw.githubusercontent.com/prometheus-community/helm-charts/d616961860a0248f77a2783923511550fad23569/charts/kube-prometheus-stack/charts/crds/crds/crd-podmonitors.yaml")
	if err != nil {
		return err
	}

	// Install cost-manager
	err = installCostManager(ctx, image, helmChartPath)
	if err != nil {
		return err
	}

	return nil
}

func createKindCluster() (rerr error) {
	// Create temporary file to store kind configuration
	kindConfigurationFile, err := os.CreateTemp("", "kind-*.yaml")
	if err != nil {
		return err
	}
	defer func() {
		err := os.Remove(kindConfigurationFile.Name())
		if rerr == nil {
			rerr = err
		}
	}()

	// Write kind configuration. We create one worker Node for spot-migrator to drain an another
	// worker Node to make sure there is a Node for cost-manager to be scheduled to
	_, err = kindConfigurationFile.WriteString(fmt.Sprintf(`
apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
name: %s
nodes:
- role: control-plane
- role: worker
- role: worker
`, kindClusterName))
	if err != nil {
		return err
	}

	err = runCommand("kind", "create", "cluster", "--config", kindConfigurationFile.Name())
	if err != nil {
		return err
	}

	// Label all Nodes as spot Nodes until we are ready to test spot-migrator
	err = runCommand("kubectl", "label", "node", "--all", fmt.Sprintf("%s=%s", cloudproviderfake.SpotInstanceLabelKey, cloudproviderfake.SpotInstanceLabelValue))

	return nil
}

func installCostManager(ctx context.Context, image, helmChartPath string) (rerr error) {
	// Create temporary file to store Helm values
	valuesFile, err := os.CreateTemp("", "cost-manager-values-*.yaml")
	if err != nil {
		return err
	}
	defer func() {
		err := os.Remove(valuesFile.Name())
		if rerr == nil {
			rerr = err
		}
	}()

	// Write Helm values
	_, err = valuesFile.WriteString(fmt.Sprintf(`
image:
  repository: %s
  tag: ""
  pullPolicy: Never

config:
  apiVersion: cost-manager.io/v1alpha1
  kind: CostManagerConfiguration
  controllers:
  - spot-migrator
  - pod-safe-to-evict-annotator
  cloudProvider:
    name: %s
  spotMigrator:
    migrationSchedule: "* * * * *"
  podSafeToEvictAnnotator:
    namespaceSelector:
      matchExpressions:
      - key: kubernetes.io/metadata.name
        operator: In
        values:
        - kube-system

vpa:
  enabled: true

prometheusRule:
  enabled: true

podMonitor:
  enabled: true
`, image, cloudprovider.FakeCloudProviderName))
	if err != nil {
		return err
	}

	// Install cost-manager
	err = runCommand("helm", "upgrade", "--install",
		"cost-manager", helmChartPath,
		"--namespace", "cost-manager", "--create-namespace",
		"--values", valuesFile.Name(),
		"--wait", "--timeout", "2m")
	if err != nil {
		return err
	}

	// Wait for the cost-manager Deployment to become available
	kubeClient, err := client.NewWithWatch(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		return err
	}
	return kubernetes.WaitUntilDeploymentAvailable(ctx, kubeClient, "cost-manager", "cost-manager")
}

func teardown() error {
	// err := runCommand("kind", "delete", "cluster", "--name", kindClusterName)
	// if err != nil {
	// 	return err
	// }

	return nil
}

func printDebugInformation() error {
	err := runCommand("kubectl", "get", "nodes")
	if err != nil {
		return err
	}
	err = runCommand("kubectl", "describe", "deployment/cost-manager", "-n", "cost-manager")
	if err != nil {
		return err
	}
	err = runCommand("kubectl", "describe", "pod", "-n", "cost-manager", "-l", "app.kubernetes.io/name=cost-manager")
	if err != nil {
		return err
	}
	err = runCommand("kubectl", "logs", "-n", "cost-manager", "-l", "app.kubernetes.io/name=cost-manager")
	if err != nil {
		return err
	}
	return nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
