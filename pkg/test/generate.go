package test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/kubectl/pkg/scheme"
)

const (
	maxResourceNameLength = 63
)

// GenerateResourceName generates a Kubernetes resource name from the name of the test; in
// particular this can be used to generate a unique Namespace name for each test that requires one
func GenerateResourceName(t *testing.T) string {
	r := regexp.MustCompile("[A-Z]+")
	resourceName := r.ReplaceAllStringFunc(t.Name(), func(s string) string {
		if len(s) > 1 {
			s = s[:len(s)-1] + "-" + s[len(s)-1:]
		}
		return "-" + strings.ToLower(s)
	})
	resourceName = strings.TrimPrefix(resourceName, "-")
	// Truncate string to avoid: metadata.name: Invalid value: must be no more than 63 characters
	if len(resourceName) > maxResourceNameLength {
		resourceName = resourceName[:maxResourceNameLength]
		resourceName = strings.TrimSuffix(resourceName, "-")
	}
	return resourceName
}

// GenerateDeployment generates a Deployment used for testing
func GenerateDeployment(deploymentNamespaceName, deploymentName string) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	deploymentManifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      terminationGracePeriodSeconds: 1
      containers:
      - name: %s
        image: nginx
        command:
        - /usr/bin/tail
        - -f`, deploymentName, deploymentNamespaceName, deploymentName, deploymentName, deploymentName)
	_, _, err := scheme.Codecs.UniversalDeserializer().Decode([]byte(deploymentManifest), nil, deployment)
	if err != nil {
		return nil, err
	}
	return deployment, nil
}
