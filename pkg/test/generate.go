package test

import (
	"regexp"
	"strings"
	"testing"
)

const (
	maxResourceNameLength = 63
)

// GenerateResourceName generates a Kubernetes resource name from the name of a test; in particular
// this can be used to generate a unique Namespace name for each test that requires one
func GenerateResourceName(t *testing.T) string {
	r := regexp.MustCompile("[A-Z+]")
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
