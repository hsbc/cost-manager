package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	cloudproviderfake "github.com/hsbc/cost-manager/pkg/cloudprovider/fake"
	"github.com/hsbc/cost-manager/pkg/kubernetes"
	"github.com/hsbc/cost-manager/pkg/test"
	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	apiwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestSpotMigrator tests that spot-migrator successfully drains a worker Node while respecting
// PodDisruptionBudgets and excludes control plane Nodes
func TestSpotMigrator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	kubeClient, restConfig, err := kubernetes.NewClient()
	require.Nil(t, err)

	// Find the worker Node to be drained by spot-migrator
	workerNodeSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "node-role.kubernetes.io/control-plane",
				Operator: "DoesNotExist",
			},
			{
				Key:      "spot-migrator",
				Operator: "In",
				Values:   []string{"true"},
			},
		},
	})
	require.Nil(t, err)
	nodeList := &corev1.NodeList{}
	err = kubeClient.List(ctx, nodeList, client.MatchingLabelsSelector{Selector: workerNodeSelector})
	require.Nil(t, err)
	require.Greater(t, len(nodeList.Items), 0)
	nodeName := nodeList.Items[0].Name

	// Deploy a workload to the worker Node
	namespaceName := test.GenerateResourceName(t)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
	err = kubeClient.Create(ctx, namespace)
	require.Nil(t, err)
	deploymentName := namespaceName
	deployment, err := test.GenerateDeployment(namespaceName, deploymentName)
	require.Nil(t, err)
	deployment.Spec.Template.Spec.NodeSelector = map[string]string{"kubernetes.io/hostname": nodeName}
	deployment.Spec.Template.Spec.Tolerations = []corev1.Toleration{
		{
			Key:      "spot-migrator",
			Operator: corev1.TolerationOpEqual,
			Value:    "true",
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}
	err = kubeClient.Create(ctx, deployment)
	require.Nil(t, err)
	t.Logf("Waiting for Deployment %s/%s to become available...", deployment.Namespace, deployment.Name)
	err = kubernetes.WaitUntilDeploymentAvailable(ctx, kubeClient, namespaceName, deploymentName)
	require.Nil(t, err)
	t.Logf("Deployment %s/%s is available!", deployment.Namespace, deployment.Name)

	// Create PodDisruptionBudget...
	zero := intstr.FromInt(0)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespaceName,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &zero,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": deploymentName},
			},
		},
	}
	err = kubeClient.Create(ctx, pdb)
	require.Nil(t, err)
	// ...and wait until it's blocking eviction
	pdbName := pdb.Name
	listerWatcher := kubernetes.NewListerWatcher(ctx, kubeClient, &policyv1.PodDisruptionBudgetList{}, &client.ListOptions{Namespace: pdb.Namespace})
	condition := func(event apiwatch.Event) (bool, error) {
		pdb, err := kubernetes.ParseWatchEventObject[*policyv1.PodDisruptionBudget](event)
		if err != nil {
			return false, err
		}
		return pdb.Name == pdbName && pdb.Status.DisruptionsAllowed == 0 && pdb.Generation == pdb.Status.ObservedGeneration, nil
	}
	_, err = watch.UntilWithSync(ctx, listerWatcher, &policyv1.PodDisruptionBudget{}, nil, condition)
	require.Nil(t, err)

	// Label worker Node as an on-demand Node to give spot-migrator something to drain
	node := &corev1.Node{}
	err = kubeClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	require.Nil(t, err)
	patch := []byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"false"}}}`, cloudproviderfake.SpotInstanceLabelKey))
	err = kubeClient.Patch(ctx, node, client.RawPatch(types.StrategicMergePatchType, patch))
	require.Nil(t, err)

	// Wait for the Node to be marked as unschedulable. This should not take any longer than 2
	// minutes since spot-migrator is configured with a 1 minute migration interval
	t.Logf("Waiting for Node %s to be marked as unschedulable...", nodeName)
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	listerWatcher = kubernetes.NewListerWatcher(ctx, kubeClient, &corev1.NodeList{})
	condition = func(event apiwatch.Event) (bool, error) {
		node, err := kubernetes.ParseWatchEventObject[*corev1.Node](event)
		if err != nil {
			return false, err
		}
		return node.Name == nodeName && node.Spec.Unschedulable, nil
	}
	_, err = watch.UntilWithSync(ctxWithTimeout, listerWatcher, &corev1.Node{}, nil, condition)
	require.Nil(t, err)
	t.Logf("Node %s marked as unschedulable!", nodeName)

	// Make sure that the PodDisruptionBudget blocks eviction
	t.Logf("Ensuring that Deployment %s/%s is not evicted...", deployment.Namespace, deployment.Name)
	ctxWithTimeout, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = kubernetes.WaitUntilDeploymentUnavailable(ctxWithTimeout, kubeClient, namespaceName, deploymentName)
	require.True(t, wait.Interrupted(err))
	t.Logf("Deployment %s/%s has not been evicted!", deployment.Namespace, deployment.Name)

	// Delete PodDisruptionBudget...
	err = kubeClient.Delete(ctx, pdb)
	require.Nil(t, err)
	// ...and wait for the Deployment to become unavailable
	t.Logf("Waiting for Deployment %s/%s to become unavailable...", deployment.Namespace, deployment.Name)
	err = kubernetes.WaitUntilDeploymentUnavailable(ctx, kubeClient, namespaceName, deploymentName)
	require.Nil(t, err)
	t.Logf("Deployment %s/%s is unavailable!", deployment.Namespace, deployment.Name)

	// Delete Node; typically this would be done by the node controller but we simulate it here:
	// https://github.com/hsbc/cost-manager/blob/bf176ada100e19a765d276aee1a0a2d6038275e0/pkg/controller/spot_migrator.go#L242-L250
	err = kubeClient.Delete(ctx, node)
	require.Nil(t, err)

	// Wait for Prometheus metric to indicate successful migration
	t.Log("Waiting for Prometheus metric to indicate successful migration...")
	pod, err := kubernetes.WaitForAnyReadyPod(ctx, kubeClient, client.InNamespace("monitoring"), client.MatchingLabels{"app.kubernetes.io/name": "prometheus"})
	require.Nil(t, err)
	// Port forward to Prometheus in the background
	forwardedPort, close, err := kubernetes.PortForward(ctx, restConfig, pod.Namespace, pod.Name, 9090)
	require.Nil(t, err)
	defer func() {
		err := close()
		require.Nil(t, err)
	}()
	// Setup Prometheus client using local forwarded port
	prometheusAddress := fmt.Sprintf("http://127.0.0.1:%d", forwardedPort)
	prometheusClient, err := api.NewClient(api.Config{
		Address: prometheusAddress,
	})
	require.Nil(t, err)
	prometheusAPI := prometheusv1.NewAPI(prometheusClient)
	// Wait for the spot-migrator metric to be scraped by Prometheus...
	var currentMetricValue model.SampleValue
	for {
		results, _, err := prometheusAPI.Query(ctx, `sum(cost_manager_spot_migrator_operation_success_total{job="cost-manager",namespace="cost-manager"})`, time.Now())
		require.Nil(t, err)
		if len(results.(model.Vector)) == 1 {
			currentMetricValue = results.(model.Vector)[0].Value
			break
		}
		time.Sleep(time.Second)
	}
	// ...and then wait for it to increase
	for {
		results, _, err := prometheusAPI.Query(ctx, `sum(cost_manager_spot_migrator_operation_success_total{job="cost-manager",namespace="cost-manager"})`, time.Now())
		require.Nil(t, err)
		require.Equal(t, 1, len(results.(model.Vector)))
		if results.(model.Vector)[0].Value > currentMetricValue {
			break
		}
		time.Sleep(time.Second)
	}
	t.Log("Migration successful!")

	// Delete Namespace
	err = kubeClient.Delete(ctx, namespace)
	require.Nil(t, err)

	// Finally, we verify that all control plane Nodes are schedulable
	controlPlaneNodeSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "node-role.kubernetes.io/control-plane",
				Operator: "Exists",
			},
		},
	})
	require.Nil(t, err)
	nodeList = &corev1.NodeList{}
	err = kubeClient.List(ctx, nodeList, client.MatchingLabelsSelector{Selector: controlPlaneNodeSelector})
	require.Nil(t, err)
	require.Greater(t, len(nodeList.Items), 0)
	for _, node := range nodeList.Items {
		require.False(t, node.Spec.Unschedulable)
	}
}
