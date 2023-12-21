package kubernetes

import (
	"context"
	"io"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiwatch "k8s.io/apimachinery/pkg/watch"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/watch"
	"k8s.io/kubectl/pkg/drain"
)

const (
	// We match our Node drain timeout with GKE:
	// https://cloud.google.com/kubernetes-engine/docs/concepts/node-pools#drain
	nodeDrainTimeout = time.Hour
)

// We use the default drain implementation:
// https://github.com/kubernetes/kubectl/blob/3ec401449e5821ad954942c7ecec9d2c90ecaaa1/pkg/drain/default.go
func DrainNode(ctx context.Context, clientset kubernetes.Interface, node *corev1.Node) error {
	// https://github.com/kubernetes/kubectl/blob/3ec401449e5821ad954942c7ecec9d2c90ecaaa1/pkg/cmd/drain/drain.go#L147-L160
	drainer := &drain.Helper{
		Ctx:                 ctx,
		Client:              clientset,
		Force:               true,
		GracePeriodSeconds:  -1,
		IgnoreAllDaemonSets: true,
		Timeout:             nodeDrainTimeout,
		DeleteEmptyDirData:  true,
		Out:                 io.Discard,
		ErrOut:              io.Discard,
	}

	err := drain.RunCordonOrUncordon(drainer, node, true)
	if err != nil {
		return errors.Wrapf(err, "failed to cordon Node %s", node.Name)
	}

	err = drain.RunNodeDrain(drainer, node.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to drain Node %s", node.Name)
	}

	return nil
}

func WaitForNodeToBeDeleted(ctx context.Context, clientset kubernetes.Interface, nodeName string) error {
	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Determine whether Node has already been deleted
	nodeIsNotFound := true
	for _, node := range nodeList.Items {
		if node.Name == nodeName {
			nodeIsNotFound = false
			break
		}
	}
	if nodeIsNotFound {
		return nil
	}

	// Wait for Node to be deleted
	watcher := &cache.ListWatch{
		WatchFunc: func(options metav1.ListOptions) (apiwatch.Interface, error) {
			return clientset.CoreV1().Nodes().Watch(ctx, options)
		},
	}
	condition := func(event apiwatch.Event) (bool, error) {
		node, err := parseWatchEventObject[*corev1.Node](event)
		if err != nil {
			return false, err
		}
		if event.Type == apiwatch.Deleted {
			if node.Name == nodeName {
				return true, nil
			}
		}
		return false, nil
	}

	_, err = watch.Until(ctx, nodeList.ResourceVersion, watcher, condition)
	return err
}
