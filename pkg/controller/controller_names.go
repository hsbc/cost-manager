package controller

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	// See the following for how controller names should be treated:
	// https://github.com/kubernetes/cloud-provider/blob/30270693811ff7d3c4646509eed7efd659332e72/names/controller_names.go
	AllControllerNames = []string{
		SpotMigratorControllerName,
	}
	// All controllers are disabled by default
	DisabledByDefaultControllerNames = sets.NewString(AllControllerNames...)
)
