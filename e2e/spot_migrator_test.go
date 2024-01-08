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

	// Port forward to Prometheus and create client using local forwarded port
	pod, err := kubernetes.WaitForAnyReadyPod(ctx, kubeClient, client.InNamespace("monitoring"), client.MatchingLabels{"app.kubernetes.io/name": "prometheus"})
	require.Nil(t, err)
	forwardedPort, stop, err := kubernetes.PortForward(ctx, restConfig, pod.Namespace, pod.Name, 9090)
	require.Nil(t, err)
	defer func() {
		err := stop()
		require.Nil(t, err)
	}()
	prometheusAddress := fmt.Sprintf("http://127.0.0.1:%d", forwardedPort)
	prometheusClient, err := api.NewClient(api.Config{
		Address: prometheusAddress,
	})
	require.Nil(t, err)
	prometheusAPI := prometheusv1.NewAPI(prometheusClient)

	t.Log("Waiting for the failure metric to be scraped by Prometheus...")
	for {
		results, _, err := prometheusAPI.Query(ctx, `sum(cost_manager_spot_migrator_operation_failure_total{job="cost-manager",namespace="cost-manager"})`, time.Now())
		require.Nil(t, err)
		if len(results.(model.Vector)) == 1 {
			break
		}
		time.Sleep(time.Second)
	}
	t.Log("Failure metric has been scraped by Prometheus!")

	// Find the cost-manager Pod...
	podList := &corev1.PodList{}
	err = kubeClient.List(ctx, podList,
		client.InNamespace("cost-manager"),
		client.MatchingLabels{"app.kubernetes.io/name": "cost-manager"})
	require.Nil(t, err)
	require.Equal(t, len(podList.Items), 1)
	// ...and make sure it is never deleted to ensure that the failure metric is not reset
	ctxWithCancel, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		watcher := kubernetes.NewWatcher(ctx, kubeClient, &corev1.PodList{},
			client.InNamespace("cost-manager"),
			client.MatchingLabels{"app.kubernetes.io/name": "cost-manager"})
		condition := func(event apiwatch.Event) (bool, error) {
			pod, err := kubernetes.ParseWatchEventObject[*corev1.Pod](event)
			if err != nil {
				return false, err
			}
			if event.Type == apiwatch.Deleted {
				return false, fmt.Errorf("cost-manager Pod %s/%s was deleted!", pod.Namespace, pod.Name)
			}
			return false, nil
		}
		_, err := watch.Until(ctxWithCancel, podList.ResourceVersion, watcher, condition)
		require.True(t, wait.Interrupted(err), extractErrorMessage(err))
	}()

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
	time.Sleep(10 * time.Second)
	err = kubeClient.Delete(ctx, node)
	require.Nil(t, err)

	// Delete Namespace
	err = kubeClient.Delete(ctx, namespace)
	require.Nil(t, err)

	// Make sure that the failure metric was never incremented
	results, _, err := prometheusAPI.Query(ctx, `sum(sum_over_time(cost_manager_spot_migrator_operation_failure_total{job="cost-manager",namespace="cost-manager"}[1h]))`, time.Now())
	require.Nil(t, err)
	require.Equal(t, 1, len(results.(model.Vector)))
	require.True(t, results.(model.Vector)[0].Value == 0, "spot-migrator failure metric was incremented!")

	// Finally, we verify that all control plane Nodes are still schedulable
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
