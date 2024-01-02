package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/hsbc/cost-manager/pkg/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubectl/pkg/scheme"
)

func Load(configFilePath string) (*v1alpha1.CostManagerConfiguration, error) {
	if configFilePath == "" {
		return nil, errors.New("configuration file not specified")
	}

	configData, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %s", err)
	}

	return decode(configData)
}

func decode(configData []byte) (*v1alpha1.CostManagerConfiguration, error) {
	config := &v1alpha1.CostManagerConfiguration{}
	decoder := scheme.Codecs.UniversalDecoder(v1alpha1.SchemeGroupVersion)

	err := runtime.DecodeInto(decoder, configData, config)
	if err != nil {
		return nil, fmt.Errorf("failed to decode configuration: %s", err)
	}

	return config, nil
}
