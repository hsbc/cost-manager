package config

import (
	"testing"

	"github.com/hsbc/cost-manager/pkg/api/v1alpha1"
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

func TestValidate(t *testing.T) {
	tests := map[string]struct {
		config *v1alpha1.CostManagerConfiguration
		valid  bool
	}{
		"valid": {
			config: &v1alpha1.CostManagerConfiguration{},
			valid:  true,
		},
		"unrecognisedController": {
			config: &v1alpha1.CostManagerConfiguration{
				Controllers: []string{"unrecognised-controller"},
			},
			valid: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := validate(test.config)
			if test.valid {
				require.Nil(t, err)
			} else {
				require.NotNil(t, err)
			}
		})
	}
}
