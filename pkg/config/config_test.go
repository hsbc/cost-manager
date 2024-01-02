package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecode(t *testing.T) {
	configData := []byte(`
apiVersion: cost-manager.io/v1alpha1
kind: CostManagerConfiguration
cloudProvider:
  name: gcp
`)
	config, err := decode(configData)
	require.Nil(t, err)
	require.Equal(t, "gcp", config.CloudProvider.Name)
}
