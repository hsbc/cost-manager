package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CostManagerConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	// TODO(dippynark): Support all generic controller fields:
	// https://github.com/kubernetes/controller-manager/blob/2a157ca0075be690e609881e5fdd3362cc62ecdc/config/v1alpha1/types.go#L24-L52
	Controllers             []string                 `json:"controllers,omitempty"`
	CloudProvider           CloudProvider            `json:"cloudProvider"`
	PodSafeToEvictAnnotator *PodSafeToEvictAnnotator `json:"podSafeToEvictAnnotator,omitempty"`
}

type CloudProvider struct {
	Name string `json:"name"`
}

type PodSafeToEvictAnnotator struct {
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}
