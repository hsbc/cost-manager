package gcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseProviderID(t *testing.T) {
	providerID := "gce://my-project/my-zone/my-instance"
	project, zone, instanceName, err := ParseProviderID(providerID)
	require.Nil(t, err)
	require.Equal(t, "my-project", project)
	require.Equal(t, "my-zone", zone)
	require.Equal(t, "my-instance", instanceName)
}
