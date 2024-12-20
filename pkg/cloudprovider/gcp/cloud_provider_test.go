package gcp

import (
	"context"
	"fmt"
	"testing"
	"time"

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

func TestTimeSinceToBeDeletedTaintAdded(t *testing.T) {
	tests := map[string]struct {
		node                           *corev1.Node
		now                            time.Time
		timeSinceToBeDeletedTaintAdded time.Duration
	}{
		"missingTaint": {
			node:                           &corev1.Node{},
			now:                            time.Now(),
			timeSinceToBeDeletedTaintAdded: 0,
		},
		"recentTaint": {
			node: &corev1.Node{
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "ToBeDeletedByClusterAutoscaler",
							Value:  fmt.Sprint(time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC).Unix()),
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			now:                            time.Date(0, 0, 0, 0, 1, 0, 0, time.UTC),
			timeSinceToBeDeletedTaintAdded: time.Minute,
		},
		"futureTaint": {
			node: &corev1.Node{
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "ToBeDeletedByClusterAutoscaler",
							Value:  fmt.Sprint(time.Date(0, 0, 0, 0, 1, 0, 0, time.UTC).Unix()),
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			now:                            time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
			timeSinceToBeDeletedTaintAdded: 0,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			timeSinceToBeDeletedTaintAdded := timeSinceToBeDeletedTaintAdded(test.node, test.now)
			require.Equal(t, test.timeSinceToBeDeletedTaintAdded, timeSinceToBeDeletedTaintAdded)
		})
	}
}
