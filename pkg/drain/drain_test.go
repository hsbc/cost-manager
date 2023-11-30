package drain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWaitForNodeToBeDeletedWithTimeoutWithMissingNode(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()
	err := WaitForNodeToBeDeletedWithTimeout(ctx, clientset, "test")
	require.Nil(t, err)
}

func TestWaitForNodeToBeDeletedWithTimeoutWithCancelledContext(t *testing.T) {
	// Create cancelled context to timeout straight away
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	clientset := fake.NewSimpleClientset()
	_, err := clientset.CoreV1().Nodes().Create(ctx, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}, metav1.CreateOptions{})
	require.Nil(t, err)
	err = WaitForNodeToBeDeletedWithTimeout(ctx, clientset, "test")
	require.True(t, wait.Interrupted(err))
}
