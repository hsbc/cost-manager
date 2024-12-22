package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewScheme(t *testing.T) {
	_, err := NewScheme()
	require.NoError(t, err)
}
