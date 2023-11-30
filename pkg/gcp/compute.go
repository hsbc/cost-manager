package gcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/api/compute/v1"
)

const (
	operationPollInterval = 10 * time.Second
)

func DeleteInstanceFromManagedInstanceGroup(ctx context.Context, computeService *compute.Service, project, zone, managedInstanceGroupName, instance string) error {
	instanceGroupManagedsDeleteInstancesRequest := &compute.InstanceGroupManagersDeleteInstancesRequest{
		Instances: []string{instance},
	}
	r, err := computeService.InstanceGroupManagers.DeleteInstances(project, zone, managedInstanceGroupName, instanceGroupManagedsDeleteInstancesRequest).Do()
	if err != nil {
		return errors.Wrap(err, "failed to delete managed instance")
	}
	err = waitForZonalComputeOperation(ctx, computeService, project, zone, r.Name)
	// We ignore the error if it indicates that the instance is already being deleted
	if err != nil && !strings.Contains(err.Error(), "Instance is already being deleted.") {
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
// projects/[PROJECT_ID]/zones/[ZONE]/instanceGroupManagers/[INSTANCE_GROUP_MANAGER_NAME]
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
