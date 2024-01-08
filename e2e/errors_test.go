package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractNilErrorMessage(t *testing.T) {
	require.Equal(t, "", extractErrorMessage(nil))
}
