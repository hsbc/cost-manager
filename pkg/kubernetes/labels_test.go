package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMatchesLabels(t *testing.T) {
	tests := map[string]struct {
		selector *metav1.LabelSelector
		labels   map[string]string
		matches  bool
	}{
		"nilSelectorNilLabels": {
			selector: nil,
			labels:   nil,
			matches:  true,
		},
		"emptySelectorNilLabels": {
			selector: &metav1.LabelSelector{},
			labels:   nil,
			matches:  true,
		},
		"nilSelectorEmptyLabels": {
			selector: nil,
			labels:   map[string]string{},
			matches:  true,
		},
		"emptySelectorEmptyLabels": {
			selector: &metav1.LabelSelector{},
			labels:   map[string]string{},
			matches:  true,
		},
		"nilSelectorNonEmptyLabels": {
			selector: nil,
			labels: map[string]string{
				"kubernetes.io/metadata.name": "kube-system",
			},
			matches: true,
		},
		"emptySelectorNonEmptyLabels": {
			selector: &metav1.LabelSelector{},
			labels: map[string]string{
				"kubernetes.io/metadata.name": "kube-system",
			},
			matches: true,
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
			matches: true,
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
			matches: false,
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
			labels:  nil,
			matches: false,
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
			labels:  nil,
			matches: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			matches, err := SelectorMatchesLabels(test.selector, test.labels)
			require.Nil(t, err)
			require.Equal(t, test.matches, matches)
		})
	}
}
