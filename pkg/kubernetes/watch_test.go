package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	apiwatch "k8s.io/apimachinery/pkg/watch"
)

func TestParseWatchEventPodObject(t *testing.T) {
	event := apiwatch.Event{
		Object: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
		},
	}
	pod, err := ParseWatchEventObject[*corev1.Pod](event)
	require.Nil(t, err)
	require.Equal(t, "foo", pod.ObjectMeta.Name)
	require.Equal(t, "bar", pod.ObjectMeta.Namespace)
}

func TestParseWatchEventErrorObject(t *testing.T) {
	event := apiwatch.Event{
		Type: watch.Error,
		Object: &metav1.Status{
			Message: "message",
		},
	}
	_, err := ParseWatchEventObject[*corev1.Pod](event)
	require.NotNil(t, err)
	require.Equal(t, "watch failed with error: message", err.Error())
}
