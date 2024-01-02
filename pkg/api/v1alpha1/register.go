package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const GroupName = "cost-manager.io"

var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}
