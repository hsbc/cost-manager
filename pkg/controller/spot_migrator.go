package controller

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/hsbc/cost-manager/pkg/domain"
	"github.com/hsbc/cost-manager/pkg/drain"
	"github.com/hsbc/cost-manager/pkg/gcp"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/api/compute/v1"
	"gopkg.in/robfig/cron.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// Running spot migration hourly seems like a good tradeoff between cluster stability and
	// reactivity to spot availability. Note that this will schedule on the hour rather than
	// relative to the current time; this ensures that spot migration still has a good chance of
	// running even if cost-manager is being restarted regularly:
	// https://pkg.go.dev/github.com/robfig/cron#hdr-Predefined_schedules
	cronSpec = "@hourly"
	// https://cloud.google.com/kubernetes-engine/docs/concepts/spot-vms#scheduling-workloads
	onDemandNodeLabelSelector = "cloud.google.com/gke-spot!=true"
)

var (
	spotMigratorOperationSuccessTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cost_manager_spot_migrator_operation_success_total",
		Help: "The total number of successful spot-migrator operations",
	})
	// Annotation to add to Nodes before draining to allow them to be identified if restarted
	nodeCordonedByAnnotationKey = fmt.Sprintf("%s/%s", domain.Name, "cordoned-by")
)

// spot-migrator periodically drains on-demand Nodes in an attempt to migrate workloads to spot
// Nodes; this works because draining Nodes will eventually trigger cluster scale up and the cluster
// autoscaler attempts to scale up the least expensive node pool, taking into account the reduced
// cost of spot Nodes:
// https://cloud.google.com/kubernetes-engine/docs/concepts/cluster-autoscaler#operating_criteria
type SpotMigrator struct {
	Clientset kubernetes.Interface
	Logger    logr.Logger
}

var _ manager.Runnable = &SpotMigrator{}

// Start starts the spot-migrator and blocks until the context is cancelled
func (sm *SpotMigrator) Start(ctx context.Context) error {
	// Register Prometheus metrics
	metrics.Registry.MustRegister(spotMigratorOperationSuccessTotal)

	// If spot-migrator drains itself it will leave an unschedulable Node in the cluster so we begin
	// by draining all unschedulable on-demand Nodes that have been annotated
	onDemandNodes, err := sm.listOnDemandNodes(ctx)
	if err != nil {
		return err
	}
	for _, onDemandNode := range onDemandNodes {
		if onDemandNode.Spec.Unschedulable && hasCordonedByAnnotation(onDemandNode) {
			err = sm.drainAndDeleteNode(ctx, onDemandNode)
			if err != nil {
				return err
			}
		}
	}

	// Parse cron spec
	cronSchedule, err := cron.Parse(cronSpec)
	if err != nil {
		return err
	}

	for {
		// Wait until the next schedule time or the context is cancelled
		now := time.Now()
		nextSchedule := cronSchedule.Next(now)
		sleepDuration := nextSchedule.Sub(now)
		sm.Logger.WithValues("sleepDuration", sleepDuration.String()).Info("Waiting before next spot migration")
		select {
		case <-time.After(sleepDuration):
		case <-ctx.Done():
			return nil
		}

		err := sm.run(ctx)
		if err != nil {
			// We do not return the error to make sure other cost-manager processes/controllers
			// continue to run; we rely on Prometheus metrics to alert us to failures
			sm.Logger.Error(err, "Failed to run spot migration")
		}
	}
}

func (sm *SpotMigrator) run(ctx context.Context) error {
	for {
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

		// Drain one of the on-demand Nodes
		onDemandNode, err := chooseNodeToDrain(ctx, beforeDrainOnDemandNodes)
		if err != nil {
			return err
		}
		err = sm.drainAndDeleteNode(ctx, onDemandNode)
		if err != nil {
			return err
		}

		// If the context has been cancelled then return instead of continuing with the migration
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// List on-demand Nodes after draining
		afterDrainOnDemandNodes, err := sm.listOnDemandNodes(ctx)
		if err != nil {
			return err
		}

		// If any on-demand Nodes were created while draining then we assume that there are no more
		// spot Nodes available and that spot migration is complete
		if nodeCreated(beforeDrainOnDemandNodes, afterDrainOnDemandNodes) {
			sm.Logger.Info("Node was created while draining")
			return nil
		}
	}
}

func (sm *SpotMigrator) listOnDemandNodes(ctx context.Context) ([]*corev1.Node, error) {
	onDemandNodes := []*corev1.Node{}
	onDemandNodeList, err := sm.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: onDemandNodeLabelSelector})
	if err != nil {
		return onDemandNodes, err
	}
	for _, node := range onDemandNodeList.Items {
		onDemandNodes = append(onDemandNodes, node.DeepCopy())
	}
	return onDemandNodes, nil
}

// drainNode drains the specified Node and deletes the underlying instance
func (sm *SpotMigrator) drainAndDeleteNode(ctx context.Context, node *corev1.Node) error {
	logger := sm.Logger.WithValues("node", node.Name)

	// Retrieve instance details from the provider ID
	project, zone, instanceName, err := gcp.ParseProviderID(node.Spec.ProviderID)
	if err != nil {
		return err
	}

	// Validate that the provider ID details match with the Node. This should not be necessary but
	// it helps avoid deleting the wrong instance if there is a bug in our logic
	if instanceName != node.Name {
		return fmt.Errorf("provider ID instance name \"%s\" does not match with Node name \"%s\"", instanceName, node.Name)
	}
	if node.Labels == nil {
		return fmt.Errorf("failed to determine zone for Node %s", node.Name)
	}
	nodeZone, ok := node.Labels[corev1.LabelTopologyZone]
	if !ok {
		return fmt.Errorf("failed to determine zone for Node %s", node.Name)
	}
	if zone != nodeZone {
		return fmt.Errorf("provider ID zone \"%s\" does not match with Node zone \"%s\"", zone, nodeZone)
	}

	// Create compute service for interacting with GCP compute API
	computeService, err := compute.NewService(ctx)
	if err != nil {
		return err
	}

	// Retrieve the compute instance corresponding to Node
	instance, err := computeService.Instances.Get(project, zone, node.Name).Do()
	if err != nil {
		return fmt.Errorf("failed to find compute instance corresponding to Node %s", node.Name)
	}

	// Determine the managed instance group that created the instance
	managedInstanceGroupName, err := gcp.GetManagedInstanceGroupFromInstance(instance)
	if err != nil {
		return err
	}

	// Before we cordon and drain the Node we annotate it. If we happen to drain ourself this will
	// allow us to identify the Node again and continue draining after restarting
	err = sm.annotateNode(ctx, node.Name)
	if err != nil {
		return err
	}

	logger.Info("Draining Node")
	err = drain.DrainNode(ctx, sm.Clientset, node)
	if err != nil {
		return err
	}
	logger.Info("Drained Node successfully")

	// Delete instance from managed instance group
	logger.Info("Deleting managed instance")
	err = gcp.DeleteInstanceFromManagedInstanceGroup(ctx, computeService, project, zone, managedInstanceGroupName, fmt.Sprintf("zones/%s/instances/%s", zone, instance.Name))
	if err != nil {
		return err
	}
	logger.Info("Managed instance deleted successfully")

	// Wait for Node to be removed from the Kubernetes API server
	logger.Info("Waiting for Node to be deleted")
	err = drain.WaitForNodeToBeDeletedWithTimeout(ctx, sm.Clientset, node.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to wait for Node %s to be deleted", node.Name)
	}
	logger.Info("Node deleted")

	// Increment success metric
	spotMigratorOperationSuccessTotal.Inc()

	return nil
}

func (sm *SpotMigrator) annotateNode(ctx context.Context, nodeName string) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, nodeCordonedByAnnotationKey, hostname))
	_, err = sm.Clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return err
	}
	return nil
}

// chooseNodeToDrain attempts the find the best Node to drain using the following algorithm:
// 1. If there are any unschedulable Nodes then return the oldest
// 2. Otherwise if there are any schedulable Nodes marked for deletion by the cluster-autoscaler then return the oldest
// 3. Otherwise return the oldest schedulable Node
func chooseNodeToDrain(ctx context.Context, nodes []*corev1.Node) (*corev1.Node, error) {
	// There should always be at least 1 Node to choose from
	if len(nodes) == 0 {
		return nil, errors.New("failed to choose Node from empty list")
	}

	// Sort the Nodes in the order in which they were created
	sort.Slice(nodes, func(i, j int) bool {
		iTime := nodes[i].CreationTimestamp.Time
		jTime := nodes[j].CreationTimestamp.Time
		return iTime.Before(jTime)
	})

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
			if taint.Key == "ToBeDeletedByClusterAutoscaler" && taint.Effect == corev1.TaintEffectNoSchedule {
				return node, nil
			}
		}
	}

	// If any Nodes are candidates for deletion by the cluster autoscaler then return the first one;
	// this reduces the chance of having more than one Node being drained at the same time
	for _, node := range nodes {
		for _, taint := range node.Spec.Taints {
			// https://github.com/kubernetes/autoscaler/blob/299c9637229fb2bf849c1d86243fe2948d14101e/cluster-autoscaler/utils/taints/taints.go#L124
			if taint.Key == "DeletionCandidateOfClusterAutoscaler" && taint.Effect == corev1.TaintEffectPreferNoSchedule {
				return node, nil
			}
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

func hasCordonedByAnnotation(node *corev1.Node) bool {
	if node.Annotations == nil {
		return false
	}
	_, ok := node.Annotations[nodeCordonedByAnnotationKey]
	return ok
}
