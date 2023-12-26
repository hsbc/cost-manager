package gcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	corev1 "k8s.io/api/core/v1"
)

const (
	// https://cloud.google.com/kubernetes-engine/docs/concepts/spot-vms#scheduling-workloads
	spotVMLabelKey   = "cloud.google.com/gke-spot"
	spotVMLabelValue = "true"

	// After kube-proxy starts failing its health check GCP load balancers should mark the instance
	// as unhealthy within 24 seconds but we wait for slightly longer to give in-flight connections
	// time to complete before we delete the underlying instance:
	// https://github.com/kubernetes/ingress-gce/blob/2a08b1e4111a21c71455bbb2bcca13349bb6f4c0/pkg/healthchecksl4/healthchecksl4.go#L48
	externalLoadBalancerConnectionDrainingPeriod   = 30 * time.Second
	externalLoadBalancerConnectionDrainingInterval = 5 * time.Second
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

// DeleteInstance retrieves the underlying compute instance of the Kubernetes Node, drains any
// connections from GCP load balancers and then deletes it from its managed instance group
func (gcp *CloudProvider) DeleteInstance(ctx context.Context, node *corev1.Node) error {
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

	// Until KEP-3836 has been released we explicitly remove the instance from any instance groups
	// managed by the GCP Cloud Controller Manager to trigger connection draining. This is required
	// since there seems to be a bug in the GCP Cloud Controller Manager where it does not remove an
	// instance from a backend instance group if it is the last instance in the group; in this case
	// it updates the backend service to remove the instance group as a backend which does not seem
	// to trigger connection draining:
	// https://cloud.google.com/load-balancing/docs/enabling-connection-draining
	// https://github.com/kubernetes/cloud-provider-gcp/issues/643
	instanceGroupsListCall := gcp.computeService.InstanceGroups.List(project, zone)
	for {
		instanceGroups, err := instanceGroupsListCall.Do()
		if err != nil {
			return err
		}
		for _, instanceGroup := range instanceGroups.Items {
			// Only consider instance groups managed by the GCP Cloud Controller Manager:
			// https://github.com/kubernetes/cloud-provider-gcp/blob/398b1a191aa49b7c67ed5e4677400b73243904e2/providers/gce/gce_loadbalancer_naming.go#L35-L43
			// TODO(dippynark): Use the cluster ID:
			// https://github.com/kubernetes/cloud-provider-gcp/blob/398b1a191aa49b7c67ed5e4677400b73243904e2/providers/gce/gce_clusterid.go#L43-L50
			if !strings.HasPrefix(instanceGroup.Name, "k8s-ig--") {
				continue
			}
			// Ignore empty instance groups
			if instanceGroup.Size == 0 {
				continue
			}
			instanceGroupInstances, err := gcp.computeService.InstanceGroups.ListInstances(project, zone, instanceGroup.Name, &compute.InstanceGroupsListInstancesRequest{}).Do()
			if err != nil {
				return err
			}
			for _, instanceGroupInstance := range instanceGroupInstances.Items {
				if instanceGroupInstance.Instance == instance.SelfLink {
					// There is a small chance that the GCP Cloud Controller Manager is currently
					// processing an old list of Nodes so that after we remove the instance from its
					// instance group the GCP Cloud Controller Manager will add it back. We mitigate
					// this by periodically attempting to remove the instance. Once KEP-3836 has
					// been released we will not need to remove the instance, we will just need to
					// wait for load balancer health checks to mark the instance as unhealthy, so we
					// periodically attempt removal for the same length of time as we will need to
					// wait so that connection draining works before and after KEP-3836
					removeInstance := func() error {
						operation, err := gcp.computeService.InstanceGroups.RemoveInstances(project, zone, instanceGroup.Name,
							&compute.InstanceGroupsRemoveInstancesRequest{Instances: []*compute.InstanceReference{{Instance: instance.SelfLink}}}).Do()
						// Ignore the error if the instance has already been removed
						if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == http.StatusBadRequest &&
							len(apiErr.Errors) == 1 && apiErr.Errors[0].Reason == "memberNotFound" {
							return nil
						}
						if err != nil {
							return err
						}
						err = gcp.waitForZonalComputeOperation(ctx, project, zone, operation.Name)
						if err != nil {
							return err
						}
						return nil
					}
					// Once KEP-3836 has been released we can simply sleep for the connection
					// draining period instead of periodically attempting to remove the instance
					ctxWithTimeout, cancel := context.WithTimeout(ctx, externalLoadBalancerConnectionDrainingPeriod)
					defer cancel()
					err := removeInstance()
					if err != nil {
						return err
					}
				removeInstanceLoop:
					for {
						select {
						case <-ctxWithTimeout.Done():
							break removeInstanceLoop
						case <-time.After(externalLoadBalancerConnectionDrainingInterval):
							err := removeInstance()
							if err != nil {
								return err
							}
						}
					}
				}
			}
		}
		// Continue if there is another page of results...
		if len(instanceGroups.NextPageToken) > 0 {
			instanceGroupsListCall.PageToken(instanceGroups.NextPageToken)
			continue
		}
		// ...otherwise we are done
		break
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
	err = gcp.waitForZonalComputeOperation(ctx, project, zone, r.Name)
	if err != nil {
		return errors.Wrap(err, "failed to wait for compute operation to complete successfully")
	}
	err = gcp.waitForManagedInstanceGroupStability(ctx, project, zone, managedInstanceGroupName)
	if err != nil {
		return errors.Wrap(err, "failed to wait for managed instance group stability")
	}

	return nil
}

// IsSpotInstance determines whether the underlying compute instance is a spot VM
func (gcp *CloudProvider) IsSpotInstance(ctx context.Context, node *corev1.Node) (bool, error) {
	if node.Labels == nil {
		return false, nil
	}
	value, ok := node.Labels[spotVMLabelKey]
	if !ok {
		return false, nil
	}
	return value == spotVMLabelValue, nil
}
