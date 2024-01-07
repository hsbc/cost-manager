package test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateResourceName(t *testing.T) {
	require.Equal(t, "test-generate-resource-name", GenerateResourceName(t))
}

func TestGenerateLongResourceNameTruncatedTo63CharactersXxxxx(t *testing.T) {
	actual := GenerateResourceName(t)
	require.Equal(t, 63, len(actual))

	expected := "test-generate-long-resource-name-truncated-to63-characters-xxxx"
	require.Equal(t, expected, actual)
}

// The capital X at the end of the test name will cause the 63rd character of the resource name to
// be a hyphen which gets truncated
func TestGenerateLongResourceNameTruncatedTo62CharactersXxxX(t *testing.T) {
	actual := GenerateResourceName(t)
	require.Equal(t, 62, len(actual))

	expected := "test-generate-long-resource-name-truncated-to62-characters-xxx"
	require.Equal(t, expected, actual)
}
func TestGenerateDeployment(t *testing.T) {
	_, err := GenerateDeployment("test", "test")
	require.Nil(t, err)
}
