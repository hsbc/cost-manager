package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWaitForNodeToBeDeletedWithMissingNode(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()
	err := WaitForNodeToBeDeleted(ctx, clientset, "test")
	require.Nil(t, err)
}
