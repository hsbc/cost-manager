package e2e

import (
	"context"
	"fmt"
	"testing"

	cloudproviderfake "github.com/hsbc/cost-manager/pkg/cloudprovider/fake"
	"github.com/hsbc/cost-manager/pkg/kubernetes"
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

	// Label worker Node as an on-demand Node to give spot-migrator something to drain
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "node-role.kubernetes.io/control-plane",
				Operator: "DoesNotExist",
			},
		},
	})
	require.Nil(t, err)
	nodeList := corev1.NodeList{}
	err = kubeClient.List(ctx, &nodeList, client.MatchingLabelsSelector{Selector: selector})
	require.Nil(t, err)
	var nodeName string
	for _, node := range nodeList.Items {
		patch := []byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"false"}}}`, cloudproviderfake.SpotInstanceLabelKey))
		err = kubeClient.Patch(ctx, &node, client.RawPatch(types.StrategicMergePatchType, patch))
		require.Nil(t, err)
		nodeName = node.Name
		break
	}
	require.Greater(t, len(nodeName), 0)

	// Wait for the Node to be marked as unschedulable
	t.Logf("Waiting for Node %s to be marked as unschedulable", nodeName)
	watcher := kubernetes.NewWatcher(ctx, kubeClient, &corev1.NodeList{})
	condition := func(event apiwatch.Event) (bool, error) {
		node, err := kubernetes.ParseWatchEventObject[*corev1.Node](event)
		if err != nil {
			return false, err
		}
		return node.Name == nodeName && node.Spec.Unschedulable, nil
	}
	_, err = watch.Until(ctx, nodeList.ResourceVersion, watcher, condition)
	require.Nil(t, err)
	t.Logf("Node %s marked as unschedulable!", nodeName)
}
