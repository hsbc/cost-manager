package gcp

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/api/compute/v1"
	"knative.dev/pkg/ptr"
)

func TestGetManagedInstanceGroupFromInstance(t *testing.T) {
	instance := &compute.Instance{
		Metadata: &compute.Metadata{
			Items: []*compute.MetadataItems{
				{
					Key:   "instance-template",
					Value: ptr.String("projects/my-project-number/global/instanceTemplates/my-instance-template"),
				},
				{
					Key:   "created-by",
					Value: ptr.String("projects/my-project-number/zones/my-zone/instanceGroupManagers/my-managed-instance-group"),
				},
			},
		},
	}
	managedInstanceGroupName, err := getManagedInstanceGroupFromInstance(instance)
	require.Nil(t, err)
	require.Equal(t, "my-managed-instance-group", managedInstanceGroupName)
}
