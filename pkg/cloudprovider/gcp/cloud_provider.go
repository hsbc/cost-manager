package gcp

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hsbc/cost-manager/pkg/kubernetes"
	"github.com/pkg/errors"
	"google.golang.org/api/compute/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// https://cloud.google.com/kubernetes-engine/docs/concepts/spot-vms#scheduling-workloads
	spotNodeLabelKey = "cloud.google.com/gke-spot"
	// https://cloud.google.com/kubernetes-engine/docs/how-to/preemptible-vms#use_nodeselector_to_schedule_pods_on_preemptible_vms
	preemptibleNodeLabelKey = "cloud.google.com/gke-preemptible"
)

type CloudProvider struct {
	computeService *compute.Service
}

// NewCloudProvider creates a new GCP cloud provider
func NewCloudProvider(ctx context.Context) (*CloudProvider, error) {
	computeService, err := compute.NewService(ctx)
	if err != nil {
		return nil, err
	}
	return &CloudProvider{
		computeService,
	}, nil
}

// DeleteInstance drains any connections from GCP load balancers, retrieves the underlying compute
// instance of the Kubernetes Node and then deletes it from its managed instance group
func (gcp *CloudProvider) DeleteInstance(ctx context.Context, node *corev1.Node) error {
	// GCP Network Load Balancer health checks have an interval of 8 seconds with a timeout of 1
	// second and an unhealthy threshold of 3 so we wait for 3 * 8 + 1 = 25 seconds for instances to
	// be marked as unhealthy which triggers connection draining. We add an additional 30 seconds
	// since this is the connection draining timeout used when GKE subsetting is enabled. We then
	// add an additional 5 seconds to allow processing time for the various components involved
	// (e.g. GCP probes and kube-proxy):
	// https://github.com/kubernetes/ingress-gce/blob/2a08b1e4111a21c71455bbb2bcca13349bb6f4c0/pkg/healthchecksl4/healthchecksl4.go#L42
	time.Sleep(time.Minute - timeSinceToBeDeletedTaintAdded(node, time.Now()))

	// Retrieve instance details from the provider ID
	project, zone, instanceName, err := parseProviderID(node.Spec.ProviderID)
	if err != nil {
		return err
	}

	// Validate that the provider ID details match with the Node. This should not be necessary but
	// it provides an extra level of validation that we are retrieving the expected instance
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

	// Retrieve the compute instance corresponding to the Node
	instance, err := gcp.computeService.Instances.Get(project, zone, instanceName).Do()
	if err != nil {
		return errors.Wrapf(err, "failed to get compute instance: %s/%s/%s", project, zone, instanceName)
	}

	// Determine the managed instance group that created the instance
	managedInstanceGroupName, err := getManagedInstanceGroupFromInstance(instance)
	if err != nil {
		return err
	}

	// Delete the instance from the managed instance group
	instanceGroupManagedsDeleteInstancesRequest := &compute.InstanceGroupManagersDeleteInstancesRequest{
		Instances: []string{instance.SelfLink},
		// Do not error if the instance has already been deleted or is being deleted
		SkipInstancesOnValidationError: true,
	}
	r, err := gcp.computeService.InstanceGroupManagers.DeleteInstances(project, zone, managedInstanceGroupName, instanceGroupManagedsDeleteInstancesRequest).Do()
	if err != nil {
		return errors.Wrap(err, "failed to delete managed instance")
	}
	err = gcp.waitForZonalComputeOperation(project, zone, r.Name)
	if err != nil {
		return errors.Wrap(err, "failed to wait for compute operation to complete successfully")
	}
	err = gcp.waitForManagedInstanceGroupStability(project, zone, managedInstanceGroupName)
	if err != nil {
		return errors.Wrap(err, "failed to wait for managed instance group stability")
	}

	return nil
}

// IsSpotInstance determines whether the underlying compute instance is a spot VM. We consider
// preemptible VMs to be spot VMs to align with the cluster autoscaler:
// https://github.com/kubernetes/autoscaler/blob/10fafe758c118adeb55b28718dc826511cc5ba40/cluster-autoscaler/cloudprovider/gce/gce_price_model.go#L220-L230
func (gcp *CloudProvider) IsSpotInstance(ctx context.Context, node *corev1.Node) (bool, error) {
	if node.Labels == nil {
		return false, nil
	}
	return node.Labels[spotNodeLabelKey] == "true" || node.Labels[preemptibleNodeLabelKey] == "true", nil
}

func timeSinceToBeDeletedTaintAdded(node *corev1.Node, now time.Time) time.Duration {
	// Retrieve taint value
	toBeDeletedTaintAddedValue := ""
	for _, taint := range node.Spec.Taints {
		if taint.Key == kubernetes.ToBeDeletedTaint && taint.Effect == corev1.TaintEffectNoSchedule {
			toBeDeletedTaintAddedValue = taint.Value
			break
		}
	}

	// Attempt to parse taint value as Unix timestamp
	unixTimeSeconds, err := strconv.ParseInt(toBeDeletedTaintAddedValue, 10, 64)
	if err != nil {
		return 0
	}

	timeSinceToBeDeletedTaintAdded := now.Sub(time.Unix(unixTimeSeconds, 0))
	// Ignore negative durations to avoid waiting for an unbounded amount of time
	if timeSinceToBeDeletedTaintAdded < 0 {
		return 0
	}
	return timeSinceToBeDeletedTaintAdded
}
