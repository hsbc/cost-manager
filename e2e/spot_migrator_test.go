package e2e

import (
	"context"
	"fmt"
	"testing"

	cloudproviderfake "github.com/hsbc/cost-manager/pkg/cloudprovider/fake"
	"github.com/hsbc/cost-manager/pkg/kubernetes"
	"github.com/hsbc/cost-manager/pkg/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apiwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func TestSpotMigrator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	kubeClient, err := client.NewWithWatch(config.GetConfigOrDie(), client.Options{})
	require.Nil(t, err)

	// Find a worker Node
	workerNodeSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "node-role.kubernetes.io/control-plane",
				Operator: "DoesNotExist",
			},
		},
	})
	require.Nil(t, err)
	nodeList := &corev1.NodeList{}
	err = kubeClient.List(ctx, nodeList, client.MatchingLabelsSelector{Selector: workerNodeSelector})
	require.Nil(t, err)
	var nodeName string
	for _, node := range nodeList.Items {
		nodeName = node.Name
		break
	}
	require.Greater(t, len(nodeName), 0)

	// Deploy a workload to the worker Node
	namespaceName := test.GenerateResourceName(t)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
	err = kubeClient.Create(ctx, namespace)
	require.Nil(t, err)
	deploymentName := namespaceName
	deployment, err := test.GenerateDeployment(namespaceName, deploymentName)
	require.Nil(t, err)
	t.Logf("Waiting for Deployment %s/%s to become available...", deployment.Namespace, deployment.Name)
	deployment.Spec.Template.Spec.NodeSelector = map[string]string{"kubernetes.io/hostname": nodeName}
	err = kubeClient.Create(ctx, deployment)
	require.Nil(t, err)
	err = kubernetes.WaitUntilDeploymentAvailable(ctx, kubeClient, namespaceName, deploymentName)
	require.Nil(t, err)
	t.Logf("Deployment %s/%s is available!", deployment.Namespace, deployment.Name)

	// Label worker Node as an on-demand Node to give spot-migrator something to drain
	node := &corev1.Node{}
	err = kubeClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	require.Nil(t, err)
	patch := []byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"false"}}}`, cloudproviderfake.SpotInstanceLabelKey))
	err = kubeClient.Patch(ctx, node, client.RawPatch(types.StrategicMergePatchType, patch))
	require.Nil(t, err)

	// Wait for the Node to be marked as unschedulable
	t.Logf("Waiting for Node %s to be marked as unschedulable...", nodeName)
	listerWatcher := kubernetes.NewListerWatcher(ctx, kubeClient, &corev1.NodeList{})
	condition := func(event apiwatch.Event) (bool, error) {
		node, err := kubernetes.ParseWatchEventObject[*corev1.Node](event)
		if err != nil {
			return false, err
		}
		return node.Name == nodeName && node.Spec.Unschedulable, nil
	}
	_, err = watch.UntilWithSync(ctx, listerWatcher, &corev1.Node{}, nil, condition)
	require.Nil(t, err)
	t.Logf("Node %s marked as unschedulable!", nodeName)

	// Wait for the Deployment to become unavailable
	t.Logf("Waiting for Deployment %s/%s to become unavailable...", deployment.Namespace, deployment.Name)
	err = kubernetes.WaitUntilDeploymentUnavailable(ctx, kubeClient, namespaceName, deploymentName)
	require.Nil(t, err)
	t.Logf("Deployment %s/%s is unavailable!", deployment.Namespace, deployment.Name)

	// Verify that all control plane Nodes are schedulable
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

	// Delete Namespace
	err = kubeClient.Delete(ctx, namespace)
	require.Nil(t, err)
}
