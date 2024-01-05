package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMatchesLabels(t *testing.T) {
	tests := map[string]struct {
		selector    *metav1.LabelSelector
		labels      map[string]string
		shouldMatch bool
	}{
		"nilSelectorNilLabels": {
			selector:    nil,
			labels:      nil,
			shouldMatch: true,
		},
		"emptySelectorNilLabels": {
			selector:    &metav1.LabelSelector{},
			labels:      nil,
			shouldMatch: true,
		},
		"nilSelectorEmptyLabels": {
			selector:    nil,
			labels:      map[string]string{},
			shouldMatch: true,
		},
		"emptySelectorEmptyLabels": {
			selector:    &metav1.LabelSelector{},
			labels:      map[string]string{},
			shouldMatch: true,
		},
		"nilSelectorNonEmptyLabels": {
			selector: nil,
			labels: map[string]string{
				"kubernetes.io/metadata.name": "kube-system",
			},
			shouldMatch: true,
		},
		"emptySelectorNonEmptyLabels": {
			selector: &metav1.LabelSelector{},
			labels: map[string]string{
				"kubernetes.io/metadata.name": "kube-system",
			},
			shouldMatch: true,
		},
		"nameSelectorDoesMatchNameLabel": {
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "kubernetes.io/metadata.name",
						Operator: "In",
						Values: []string{
							"kube-system",
						},
					},
				},
			},
			labels: map[string]string{
				"kubernetes.io/metadata.name": "kube-system",
			},
			shouldMatch: true,
		},
		"nameSelectorDoesNotMatchNameLabel": {
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "kubernetes.io/metadata.name",
						Operator: "In",
						Values: []string{
							"kube-system",
						},
					},
				},
			},
			labels: map[string]string{
				"kubernetes.io/metadata.name": "kube-public",
			},
			shouldMatch: false,
		},
		"nameSelectorDoesNotMatchNilLabels": {
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "kubernetes.io/metadata.name",
						Operator: "In",
						Values: []string{
							"kube-system",
						},
					},
				},
			},
			labels:      nil,
			shouldMatch: false,
		},
		"reverseNameSelectorDoesMatchNilLabels": {
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "kubernetes.io/metadata.name",
						Operator: "NotIn",
						Values: []string{
							"kube-system",
						},
					},
				},
			},
			labels:      nil,
			shouldMatch: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			matches, err := SelectorMatchesLabels(test.selector, test.labels)
			require.Nil(t, err)
			require.Equal(t, test.shouldMatch, matches)
		})
	}
}
