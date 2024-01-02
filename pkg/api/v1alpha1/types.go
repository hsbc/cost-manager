package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CostManagerConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	CloudProvider   CloudProvider `json:"cloudProvider"`
	Controllers     []string      `json:"controllers,omitempty"`
}

type CloudProvider struct {
	Name string `json:"name"`
}
