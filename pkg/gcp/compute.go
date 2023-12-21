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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	operationPollInterval = 10 * time.Second

	// https://github.com/kubernetes/autoscaler/blob/5bf33b23f2bcf5f9c8ccaf99d445e25366ee7f40/cluster-autoscaler/utils/taints/taints.go#L39-L40
	toBeDeletedTaint = "ToBeDeletedByClusterAutoscaler"

	// After kube-proxy starts failing its health check GCP load balancers should mark the instance
	// as unhealthy within 24 seconds but we wait for slightly longer to give in-flight connections
	// time to complete before we delete the underlying instance:
	// https://github.com/kubernetes/ingress-gce/blob/2a08b1e4111a21c71455bbb2bcca13349bb6f4c0/pkg/healthchecksl4/healthchecksl4.go#L48
	externalLoadBalancerConnectionDrainingPeriod   = 30 * time.Second
	externalLoadBalancerConnectionDrainingInterval = 5 * time.Second
)

// DrainExternalLoadBalancerConnections drains connections from GCP load balancers
func DrainExternalLoadBalancerConnections(ctx context.Context, clientset kubernetes.Interface, computeService *compute.Service, node *corev1.Node, project, zone, instance string) error {
	// Once KEP-3836 has been implemented the cluster autoscaler taint will start failing the
	// kube-proxy health check to trigger connection draining:
	// https://github.com/kubernetes/enhancements/tree/27ef0d9a740ae5058472aac4763483f0e7218c0e/keps/sig-network/3836-kube-proxy-improved-ingress-connectivity-reliability
	// Currently it will tell the KCCM service controller to exclude the node from load balancing:
	// https://github.com/kubernetes/kubernetes/blob/b5ba7bc4f5f49760c821cae2f152a8000922e72e/staging/src/k8s.io/cloud-provider/controllers/service/controller.go#L1043-L1051
	hasToBeDeletedTaint := false
	for _, taint := range node.Spec.Taints {
		if taint.Key == toBeDeletedTaint {
			hasToBeDeletedTaint = true
			break
		}
	}
	if !hasToBeDeletedTaint {
		// https://github.com/kubernetes/autoscaler/blob/5bf33b23f2bcf5f9c8ccaf99d445e25366ee7f40/cluster-autoscaler/utils/taints/taints.go#L166-L174
		patch := []byte(fmt.Sprintf(`[{"op": "add", "path": "/spec/taints/-", "value": {"key": "%s", "value": "%s", "effect": "%s"}}]`,
			toBeDeletedTaint, fmt.Sprint(time.Now().Unix()), corev1.TaintEffectNoSchedule))
		_, err := clientset.CoreV1().Nodes().Patch(ctx, node.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to mark Node %s for deletion", node.Name)
		}
	}

	// Until KEP-3836 has been implemented we explicitly remove the instance from any instance
	// groups managed by the GCP Cloud Controller Manager to trigger connection draining. This is
	// required since there seems to be a bug in the GCP Cloud Controller Manager where it does not
	// remove an instance from a backend instance group if it is the last instance in the group; in
	// this case it updates the backend service to remove the instance group as a backend which does
	// not trigger connection draining:
	// https://cloud.google.com/load-balancing/docs/enabling-connection-draining
	// https://github.com/kubernetes/cloud-provider-gcp/issues/643
	instanceGroupsListCall := computeService.InstanceGroups.List(project, zone)
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
			instanceGroupInstances, err := computeService.InstanceGroups.ListInstances(project, zone, instanceGroup.Name, &compute.InstanceGroupsListInstancesRequest{}).Do()
			if err != nil {
				return err
			}
			for _, instanceGroupInstance := range instanceGroupInstances.Items {
				if instanceGroupInstance.Instance == instance {
					// There is a very small chance that the GCP Cloud Controller Manager is currently
					// processing an old list of Nodes before we excluded the Node we are draining
					// causing it to add back the instance after we remove it. We mitigate this by
					// periodically attempting to remove the instance for long enough to be confident
					// that the GCP Cloud Controller Manager has observed the exclusion label
					removeInstance := func() error {
						operation, err := computeService.InstanceGroups.RemoveInstances(project, zone, instanceGroup.Name,
							&compute.InstanceGroupsRemoveInstancesRequest{Instances: []*compute.InstanceReference{{Instance: instance}}}).Do()
						// Ignore the error if the instance has already been removed
						if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == http.StatusBadRequest &&
							len(apiErr.Errors) == 1 && apiErr.Errors[0].Reason == "memberNotFound" {
							return nil
						}
						if err != nil {
							return err
						}
						err = waitForZonalComputeOperation(ctx, computeService, project, zone, operation.Name)
						if err != nil {
							return err
						}
						return nil
					}
					// Once KEP-3836 has been implemented we can simply sleep for the connection
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

	return nil
}

func DeleteInstanceFromManagedInstanceGroup(ctx context.Context, computeService *compute.Service, project, zone, managedInstanceGroupName, instance string) error {
	instanceGroupManagedsDeleteInstancesRequest := &compute.InstanceGroupManagersDeleteInstancesRequest{
		Instances: []string{instance},
		// Do not error if the instance has already been deleted or is being deleted
		SkipInstancesOnValidationError: true,
	}
	r, err := computeService.InstanceGroupManagers.DeleteInstances(project, zone, managedInstanceGroupName, instanceGroupManagedsDeleteInstancesRequest).Do()
	if err != nil {
		return errors.Wrap(err, "failed to delete managed instance")
	}
	err = waitForZonalComputeOperation(ctx, computeService, project, zone, r.Name)
	if err != nil {
		return errors.Wrap(err, "failed to wait for compute operation to complete successfully")
	}
	err = waitForManagedInstanceGroupStability(ctx, computeService, project, zone, managedInstanceGroupName)
	if err != nil {
		return errors.Wrap(err, "failed to wait for managed instance group stability")
	}
	return nil
}

func waitForManagedInstanceGroupStability(ctx context.Context, computeService *compute.Service, project, zone, managedInstanceGroupName string) error {
	for {
		r, err := computeService.InstanceGroupManagers.Get(project, zone, managedInstanceGroupName).Do()
		if err != nil {
			return err
		}
		if r.Status != nil && r.Status.IsStable {
			return nil
		}
		time.Sleep(operationPollInterval)
	}
}

func waitForZonalComputeOperation(ctx context.Context, computeService *compute.Service, project, zone, operationName string) error {
	return waitForComputeOperation(ctx, computeService, project, func() (*compute.Operation, error) {
		return computeService.ZoneOperations.Get(project, zone, operationName).Do()
	})
}

func waitForComputeOperation(ctx context.Context, computeService *compute.Service, project string, getOperation func() (*compute.Operation, error)) error {
	for {
		operation, err := getOperation()
		if err != nil {
			return err
		}
		if operation.Status == "DONE" {
			if operation.Error != nil {
				var operationErrorErrors []string
				for _, operationErrorError := range operation.Error.Errors {
					operationErrorErrors = append(operationErrorErrors, operationErrorError.Message)
				}
				return fmt.Errorf("compute operation failed with errors: %s", strings.Join(operationErrorErrors, ", "))
			}
			return nil
		}
		time.Sleep(operationPollInterval)
	}
}

// Determine the managed instance group that created the instance; instances created by managed
// instance groups should have a metadata label with key `created-by` and a value of the form:
// projects/[PROJECT_ID]/zones/[ZONE]/instanceGroupManagers/[INSTANCE_GROUP_MANAGER_NAME]:
// https://cloud.google.com/compute/docs/instance-groups/getting-info-about-migs#checking_if_a_vm_instance_is_part_of_a_mig
func GetManagedInstanceGroupFromInstance(instance *compute.Instance) (string, error) {
	if instance.Metadata != nil {
		for _, item := range instance.Metadata.Items {
			if item != nil && item.Key == "created-by" && item.Value != nil {
				createdBy := *item.Value
				tokens := strings.Split(createdBy, "/")
				if len(tokens) > 2 && tokens[len(tokens)-2] == "instanceGroupManagers" {
					return tokens[len(tokens)-1], nil
				}
			}
		}
	}
	return "", fmt.Errorf("failed to determine managed instance group for instance %s", instance.Name)
}
