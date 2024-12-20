package controller

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/hsbc/cost-manager/pkg/api/v1alpha1"
	"github.com/hsbc/cost-manager/pkg/cloudprovider"
	"github.com/hsbc/cost-manager/pkg/kubernetes"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgo "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	spotMigratorControllerName = "spot-migrator"

	// Running spot migration hourly seems like a good tradeoff between cluster stability and
	// reactivity to spot availability. Note that this will schedule on the hour rather than
	// relative to the current time; this ensures that spot migration still has a good chance of
	// running even if cost-manager is being restarted regularly:
	// https://pkg.go.dev/github.com/robfig/cron#hdr-Predefined_schedules
	defaultMigrationSchedule = "@hourly"

	// https://kubernetes.io/docs/reference/labels-annotations-taints/#node-role-kubernetes-io-control-plane
	controlPlaneNodeRoleLabelKey = "node-role.kubernetes.io/control-plane"
)

var (
	spotMigratorOperationSuccessTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cost_manager_spot_migrator_operation_success_total",
		Help: "The total number of successful spot-migrator operations",
	})
	spotMigratorOperationFailureTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cost_manager_spot_migrator_operation_failure_total",
		Help: "The total number of failed spot-migrator operations",
	})

	// Label to add to Nodes before draining to allow them to be identified if we are restarted
	nodeSelectedForDeletionLabelKey = fmt.Sprintf("%s/%s", v1alpha1.GroupName, "selected-for-deletion")
)

// spotMigrator periodically drains on-demand Nodes in an attempt to migrate workloads to spot
// Nodes; this works because draining Nodes will eventually trigger cluster scale up and the cluster
// autoscaler attempts to scale up the least expensive node pool, taking into account the reduced
// cost of spot Nodes:
// https://github.com/kubernetes/autoscaler/blob/600cda52cf764a1f08b06fc8cc29b1ef95f13c76/cluster-autoscaler/proposals/pricing.md
type spotMigrator struct {
	Config        *v1alpha1.SpotMigrator
	Clientset     clientgo.Interface
	CloudProvider cloudprovider.CloudProvider
}

var _ manager.Runnable = &spotMigrator{}

// Start starts spot-migrator and blocks until the context is cancelled
func (sm *spotMigrator) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName(spotMigratorControllerName)
	ctx = log.IntoContext(ctx, logger)

	// Register Prometheus metrics
	metrics.Registry.MustRegister(spotMigratorOperationSuccessTotal)
	metrics.Registry.MustRegister(spotMigratorOperationFailureTotal)

	// Parse migration schedule
	migrationSchedule := defaultMigrationSchedule
	if sm.Config != nil && sm.Config.MigrationSchedule != nil {
		migrationSchedule = *sm.Config.MigrationSchedule
	}
	parsedMigrationSchedule, err := parseMigrationSchedule(migrationSchedule)
	if err != nil {
		return fmt.Errorf("failed to parse migration schedule: %s", err)
	}

	// If spot-migrator drains itself then any ongoing migration operations will be cancelled. To
	// mitigate this we first drain and delete any Nodes that have previously been selected for
	// deletion. Note that we do not run a full migration in this case because otherwise we could
	// get stuck in a continuous loop of draining and deleting the Node that spot-migrator is
	// running on; we will need to wait for the next schedule time for the migration to continue
	onDemandNodes, err := sm.listOnDemandNodes(ctx)
	if err != nil {
		return err
	}
	for _, onDemandNode := range onDemandNodes {
		if isSelectedForDeletion(onDemandNode) {
			err = sm.drainAndDeleteNode(ctx, onDemandNode)
			if err != nil {
				return err
			}
		}
	}

	for {
		// Wait until the next schedule time or the context is cancelled
		now := time.Now()
		nextScheduleTime := parsedMigrationSchedule.Next(now)
		sleepDuration := nextScheduleTime.Sub(now)
		logger.WithValues("sleepDuration", sleepDuration.String()).Info("Waiting before next spot migration")
		select {
		case <-time.After(sleepDuration):
		case <-ctx.Done():
			return nil
		}

		err := sm.run(ctx)
		if err != nil {
			// We do not return the error to make sure other cost-manager processes/controllers
			// continue to run; we rely on Prometheus metrics to alert us to failures
			logger.Error(err, "Failed to run spot migration")
			spotMigratorOperationFailureTotal.Inc()
		}
	}
}

func parseMigrationSchedule(migrationSchedule string) (cron.Schedule, error) {
	return cron.ParseStandard(migrationSchedule)
}

// run runs spot migration
func (sm *spotMigrator) run(ctx context.Context) error {
	logger := log.FromContext(ctx)
	for {
		// If the context has been cancelled then return instead of continuing with the migration
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// List on-demand Nodes before draining
		beforeDrainOnDemandNodes, err := sm.listOnDemandNodes(ctx)
		if err != nil {
			return err
		}

		// If there are no on-demand Nodes then we are done
		if len(beforeDrainOnDemandNodes) == 0 {
			// Increment success metric since all workloads are already running on spot Nodes
			spotMigratorOperationSuccessTotal.Inc()
			return nil
		}

		// Select one of the on-demand Nodes to delete
		onDemandNode, err := selectNodeForDeletion(ctx, beforeDrainOnDemandNodes)
		if err != nil {
			return err
		}

		// Just before we drain and delete the Node we label it. If we happen to drain ourself this
		// will allow us to identify the Node again and continue after rescheduling
		err = sm.addSelectedForDeletionLabel(ctx, onDemandNode.Name)
		if err != nil {
			return err
		}

		// Drain and delete Node
		err = sm.drainAndDeleteNode(ctx, onDemandNode)
		if err != nil {
			return err
		}

		// List on-demand Nodes after draining
		afterDrainOnDemandNodes, err := sm.listOnDemandNodes(ctx)
		if err != nil {
			return err
		}

		// If any on-demand Nodes were created while draining then we assume that there are no more
		// spot VMs available and that spot migration is complete
		if nodeCreated(beforeDrainOnDemandNodes, afterDrainOnDemandNodes) {
			logger.Info("Spot migration complete")
			return nil
		}
	}
}

// listOnDemandNodes lists all Nodes that are not backed by a spot instance
func (sm *spotMigrator) listOnDemandNodes(ctx context.Context) ([]*corev1.Node, error) {
	nodeList, err := sm.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	onDemandNodes := []*corev1.Node{}
	for _, node := range nodeList.Items {
		// We always ignore control plane Nodes to make sure that we do not drain them
		if isControlPlaneNode(&node) {
			continue
		}
		isSpotInstance, err := sm.CloudProvider.IsSpotInstance(ctx, &node)
		if err != nil {
			return onDemandNodes, err
		}
		if !isSpotInstance {
			onDemandNodes = append(onDemandNodes, node.DeepCopy())
		}
	}
	return onDemandNodes, nil
}

// isControlPlaneNode returns true if the Node is part of the Kubernetes control plane
func isControlPlaneNode(node *corev1.Node) bool {
	_, ok := node.Labels[controlPlaneNodeRoleLabelKey]
	return ok
}

// drainAndDeleteNode drains the specified Node and deletes the underlying instance
func (sm *spotMigrator) drainAndDeleteNode(ctx context.Context, node *corev1.Node) error {
	logger := log.FromContext(ctx, "node", node.Name)

	logger.Info("Draining Node")
	err := kubernetes.DrainNode(ctx, sm.Clientset, node)
	if err != nil {
		return err
	}
	logger.Info("Drained Node successfully")

	logger.Info("Adding taint ToBeDeletedByClusterAutoscaler")
	err = sm.addToBeDeletedTaint(ctx, node)
	if err != nil {
		return err
	}
	logger.Info("Taint ToBeDeletedByClusterAutoscaler added successfully")

	logger.Info("Deleting instance")
	err = sm.CloudProvider.DeleteInstance(ctx, node)
	if err != nil {
		return err
	}
	logger.Info("Instance deleted successfully")

	// Since the underlying instance has been deleted we expect the Node object to be deleted from
	// the Kubernetes API server by the node controller:
	// https://kubernetes.io/docs/concepts/architecture/cloud-controller/#node-controller
	logger.Info("Waiting for Node object to be deleted")
	err = kubernetes.WaitForNodeToBeDeleted(ctx, sm.Clientset, node.Name)
	if err != nil {
		return err
	}
	logger.Info("Node deleted")

	// Increment success metric
	spotMigratorOperationSuccessTotal.Inc()

	return nil
}

func (sm *spotMigrator) addSelectedForDeletionLabel(ctx context.Context, nodeName string) error {
	patch := []byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"true"}}}`, nodeSelectedForDeletionLabelKey))
	_, err := sm.Clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return err
	}
	return nil
}

func isSelectedForDeletion(node *corev1.Node) bool {
	if node.Labels == nil {
		return false
	}
	value, ok := node.Labels[nodeSelectedForDeletionLabelKey]
	return ok && value == "true"
}

// addToBeDeletedTaint adds the ToBeDeletedByClusterAutoscaler taint to the Node to tell kube-proxy
// to start failing its healthz and subsequently load balancer health checks depending on provider:
// https://github.com/kubernetes/enhancements/tree/27ef0d9a740ae5058472aac4763483f0e7218c0e/keps/sig-network/3836-kube-proxy-improved-ingress-connectivity-reliability
func (sm *spotMigrator) addToBeDeletedTaint(ctx context.Context, node *corev1.Node) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := sm.Clientset.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		hasToBeDeletedTaint := false
		for _, taint := range node.Spec.Taints {
			if taint.Key == kubernetes.ToBeDeletedTaint {
				hasToBeDeletedTaint = true
				break
			}
		}
		if !hasToBeDeletedTaint {
			// https://github.com/kubernetes/autoscaler/blob/5bf33b23f2bcf5f9c8ccaf99d445e25366ee7f40/cluster-autoscaler/utils/taints/taints.go#L166-L174
			node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
				Key:    kubernetes.ToBeDeletedTaint,
				Value:  fmt.Sprint(time.Now().Unix()),
				Effect: corev1.TaintEffectNoSchedule,
			})
			_, err := sm.Clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// selectNodeForDeletion attempts the find the best Node to delete using the following algorithm:
// 1. If there are any Nodes that have previously been selected for deletion then return the oldest
// 2. Otherwise if there are any unschedulable Nodes then return the oldest
// 3. Otherwise if there are any Nodes marked for deletion by the cluster-autoscaler then return the oldest
// 4. Otherwise if there are any Nodes that are not running spot-migrator then return the oldest
// 5. Otherwise return the oldest Node
func selectNodeForDeletion(ctx context.Context, nodes []*corev1.Node) (*corev1.Node, error) {
	// There should always be at least 1 Node to select from
	if len(nodes) == 0 {
		return nil, errors.New("failed to select Node from empty list")
	}

	// Sort the Nodes in the order in which they were created
	sort.Slice(nodes, func(i, j int) bool {
		iTime := nodes[i].CreationTimestamp.Time
		jTime := nodes[j].CreationTimestamp.Time
		return iTime.Before(jTime)
	})

	// If any Nodes have previously been selected for deletion then return the first one. Note that
	// all such Nodes should have already been drained and deleted when spot-migrator started up
	for _, node := range nodes {
		if isSelectedForDeletion(node) {
			return node, nil
		}
	}

	// If any Nodes are unschedulable then return the first one; this reduces the chance of having
	// more than one unschedulable Node at any one time
	for _, node := range nodes {
		if node.Spec.Unschedulable {
			return node, nil
		}
	}

	// If any Nodes are about to be deleted by the cluster autoscaler then return the first one;
	// this reduces the chance of having more than one Node being drained at the same time
	for _, node := range nodes {
		for _, taint := range node.Spec.Taints {
			// https://github.com/kubernetes/autoscaler/blob/299c9637229fb2bf849c1d86243fe2948d14101e/cluster-autoscaler/utils/taints/taints.go#L119
			if taint.Key == kubernetes.ToBeDeletedTaint && taint.Effect == corev1.TaintEffectNoSchedule {
				return node, nil
			}
		}
	}

	// If any Nodes are candidates for deletion by the cluster autoscaler then return the first one;
	// this reduces the chance of having more than one Node being drained at the same time
	for _, node := range nodes {
		for _, taint := range node.Spec.Taints {
			// https://github.com/kubernetes/autoscaler/blob/299c9637229fb2bf849c1d86243fe2948d14101e/cluster-autoscaler/utils/taints/taints.go#L124
			if taint.Key == kubernetes.DeletionCandidateTaint && taint.Effect == corev1.TaintEffectPreferNoSchedule {
				return node, nil
			}
		}
	}

	// If any Nodes are not running spot-migrator then we return the first one; this reduces the
	// chance of spot-migrator draining itself and cancelling an ongoing migration operation. Note
	// that there is very small possibility that the Node that spot-migrator is running on is the
	// only on-demand Node remaining in the cluster that could be drained to trigger the addition of
	// a new spot Node (e.g. if all other on-demand Nodes are running Pods that cannot be scheduled
	// to spot Nodes) however this seems like the lesser evil compared to potentially repeatedly
	// cancelling migration operations
	for _, node := range nodes {
		if node.Name != os.Getenv("NODE_NAME") {
			return node, nil
		}
	}

	return nodes[0], nil
}

// nodeCreated compares the list of Nodes before and after to determine if any Nodes were created
func nodeCreated(beforeNodes, afterNodes []*corev1.Node) bool {
	for _, afterNode := range afterNodes {
		nodeCreated := true
		for _, beforeNode := range beforeNodes {
			// We compare the UID to detect if a Node object was recreated with the same name
			if beforeNode.UID == afterNode.UID {
				nodeCreated = false
				break
			}
		}
		if nodeCreated {
			return true
		}
	}
	return false
}
