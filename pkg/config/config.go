package config

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/hsbc/cost-manager/pkg/api/v1alpha1"
	"github.com/hsbc/cost-manager/pkg/controller"
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

	config, err := decode(configData)
	if err != nil {
		return config, fmt.Errorf("failed to decode configuration: %s", err)
	}

	err = validate(config)
	if err != nil {
		return config, fmt.Errorf("failed to validate configuration: %s", err)
	}

	return config, nil
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

func validate(config *v1alpha1.CostManagerConfiguration) error {
	for _, controllerName := range config.Controllers {
		if !slices.Contains(controller.AllControllerNames, controllerName) {
			return fmt.Errorf("unknown controller: %s", controllerName)
		}
	}

	return nil
}
