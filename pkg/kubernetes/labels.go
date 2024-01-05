package kubernetes

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func SelectorMatchesLabels(labelSelector *metav1.LabelSelector, resourceLabels map[string]string) (bool, error) {
	// A nil selector matches everything (the same as an empty selector) to avoid surprises
	if labelSelector == nil {
		return true, nil
	}
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return false, err
	}
	return selector.Matches(labels.Set(resourceLabels)), nil
}
