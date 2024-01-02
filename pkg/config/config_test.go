package config

import (
	"testing"

	"github.com/hsbc/cost-manager/pkg/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDecodeValidConfiguration(t *testing.T) {
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

func TestDecodeInvalidConfiguration(t *testing.T) {
	configData := []byte(`
apiVersion: cost-manager.io/v1alpha1
kind: CostManagerConfiguration
foo: bar
`)
	_, err := decode(configData)
	require.NotNil(t, err)
}

func TestDecode(t *testing.T) {
	tests := map[string]struct {
		configData []byte
		valid      bool
		config     *v1alpha1.CostManagerConfiguration
	}{
		"validCloudProviderField": {
			configData: []byte(`
apiVersion: cost-manager.io/v1alpha1
kind: CostManagerConfiguration
cloudProvider:
  name: gcp
`),
			valid: true,
			config: &v1alpha1.CostManagerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cost-manager.io/v1alpha1",
					Kind:       "CostManagerConfiguration",
				},
				CloudProvider: v1alpha1.CloudProvider{
					Name: "gcp",
				},
			},
		},
		"validControllersField": {
			configData: []byte(`
apiVersion: cost-manager.io/v1alpha1
kind: CostManagerConfiguration
controllers:
- spot-migrator
`),
			valid: true,
			config: &v1alpha1.CostManagerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cost-manager.io/v1alpha1",
					Kind:       "CostManagerConfiguration",
				},
				Controllers: []string{"spot-migrator"},
			},
		},
		"unknownField": {
			configData: []byte(`
apiVersion: cost-manager.io/v1alpha1
kind: CostManagerConfiguration
foo: bar
`),
			valid: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config, err := decode(test.configData)
			if test.valid {
				require.Nil(t, err)
				require.Equal(t, test.config, config)
			} else {
				require.NotNil(t, err)
			}
		})
	}
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
		"knownController": {
			config: &v1alpha1.CostManagerConfiguration{
				Controllers: []string{"spot-migrator"},
			},
			valid: true,
		},
		"unknownController": {
			config: &v1alpha1.CostManagerConfiguration{
				Controllers: []string{"unknown-controller"},
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
