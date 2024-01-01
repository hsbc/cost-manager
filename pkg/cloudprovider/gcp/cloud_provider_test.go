package gcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsSpotInstance(t *testing.T) {
	tests := map[string]struct {
		node           *corev1.Node
		isSpotInstance bool
	}{
		"hasSpotLabelSetToTrue": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"cloud.google.com/gke-spot": "true",
					},
				},
			},
			isSpotInstance: true,
		},
		"hasPreemptibleLabelSetToTrue": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"cloud.google.com/gke-preemptible": "true",
					},
				},
			},
			isSpotInstance: true,
		},
		"hasSpotLabelSetToFalse": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"cloud.google.com/gke-spot": "false",
					},
				},
			},
			isSpotInstance: false,
		},
		"hasPreemptibleLabelSetToFalse": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"cloud.google.com/gke-preemptible": "false",
					},
				},
			},
			isSpotInstance: false,
		},
		"hasOtherLabel": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			isSpotInstance: false,
		},
		"hasNoLabels": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{},
			},
			isSpotInstance: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cloudProvider := &CloudProvider{}
			isSpotInstance, err := cloudProvider.IsSpotInstance(context.Background(), test.node)
			require.Nil(t, err)
			require.Equal(t, test.isSpotInstance, isSpotInstance)
		})
	}
}
