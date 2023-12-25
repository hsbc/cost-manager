package gcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/api/compute/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	operationPollInterval = 10 * time.Second
)

func (gcp *CloudProvider) getInstanceFromNode(node *corev1.Node) (string, string, *compute.Instance, error) {
	// Retrieve instance details from the provider ID
	project, zone, instanceName, err := parseProviderID(node.Spec.ProviderID)
	if err != nil {
		return "", "", nil, err
	}

	// Validate that the provider ID details match with the Node. This should not be necessary but
	// it provides an extra level of validation that we are retrieving the expected instance
	if instanceName != node.Name {
		return "", "", nil, fmt.Errorf("provider ID instance name \"%s\" does not match with Node name \"%s\"", instanceName, node.Name)
	}
	if node.Labels == nil {
		return "", "", nil, fmt.Errorf("failed to determine zone for Node %s", node.Name)
	}
	nodeZone, ok := node.Labels[corev1.LabelTopologyZone]
	if !ok {
		return "", "", nil, fmt.Errorf("failed to determine zone for Node %s", node.Name)
	}
	if zone != nodeZone {
		return "", "", nil, fmt.Errorf("provider ID zone \"%s\" does not match with Node zone \"%s\"", zone, nodeZone)
	}

	// Retrieve the compute instance corresponding to the Node
	instance, err := gcp.computeService.Instances.Get(project, zone, instanceName).Do()
	if err != nil {
		return "", "", nil, errors.Wrapf(err, "failed to get compute instance: %s/%s/%s", project, zone, instanceName)
	}
	return project, zone, instance, nil
}

// getManagedInstanceGroupFromInstance determines the managed instance group that created the
// instance; instances created by managed instance groups should have a metadata label with key
// `created-by` and a value of the form:
// projects/[PROJECT_ID]/zones/[ZONE]/instanceGroupManagers/[INSTANCE_GROUP_MANAGER_NAME]:
// https://cloud.google.com/compute/docs/instance-groups/getting-info-about-migs#checking_if_a_vm_instance_is_part_of_a_mig
func getManagedInstanceGroupFromInstance(instance *compute.Instance) (string, error) {
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

func (gcp *CloudProvider) waitForManagedInstanceGroupStability(ctx context.Context, project, zone, managedInstanceGroupName string) error {
	for {
		r, err := gcp.computeService.InstanceGroupManagers.Get(project, zone, managedInstanceGroupName).Do()
		if err != nil {
			return err
		}
		if r.Status != nil && r.Status.IsStable {
			return nil
		}
		time.Sleep(operationPollInterval)
	}
}

func (gcp *CloudProvider) waitForZonalComputeOperation(ctx context.Context, project, zone, operationName string) error {
	return waitForComputeOperation(ctx, project, func() (*compute.Operation, error) {
		return gcp.computeService.ZoneOperations.Get(project, zone, operationName).Do()
	})
}

func waitForComputeOperation(ctx context.Context, project string, getOperation func() (*compute.Operation, error)) error {
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
