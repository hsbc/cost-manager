package gcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/compute/v1"
)

const (
	operationPollInterval = 10 * time.Second
)

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
