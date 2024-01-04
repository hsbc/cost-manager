//go:build tools

package tools

// Import tools required by build scripts to force `go mod` to see them as dependencies
import (
	_ "k8s.io/code-generator/cmd/deepcopy-gen"
	_ "sigs.k8s.io/kubebuilder-release-tools/notes"
)
