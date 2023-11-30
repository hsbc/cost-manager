package gcp

import (
	"fmt"
	"strings"

	"sigs.k8s.io/cluster-api-provider-gcp/cloud/providerid"
)

// ParseProviderID parses the node.Spec.ProviderID of a GKE Node. We assume the following format:
// https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/173d8a201d251cb78a76bf47ec613d0d10b3f2f7/cloud/providerid/providerid.go#L88
func ParseProviderID(providerID string) (string, string, string, error) {
	var project, zone, instanceName string
	if !strings.HasPrefix(providerID, providerid.Prefix) {
		return project, zone, instanceName, fmt.Errorf("provider ID does not have the expected prefix: %s", providerID)
	}
	tokens := strings.Split(strings.TrimPrefix(providerID, providerid.Prefix), "/")
	if len(tokens) != 3 {
		return project, zone, instanceName, fmt.Errorf("provider ID is not in the expected format: %s", providerID)
	}
	return tokens[0], tokens[1], tokens[2], nil
}
