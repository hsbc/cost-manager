package cloudprovider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCloudProvider(t *testing.T) {
	_, err := NewCloudProvider(context.Background(), "")
	require.Error(t, err)
}
