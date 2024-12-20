package kubernetes

const (
	// https://github.com/kubernetes/autoscaler/blob/5bf33b23f2bcf5f9c8ccaf99d445e25366ee7f40/cluster-autoscaler/utils/taints/taints.go#L39-L40
	ToBeDeletedTaint = "ToBeDeletedByClusterAutoscaler"
	// https://github.com/kubernetes/autoscaler/blob/5bf33b23f2bcf5f9c8ccaf99d445e25366ee7f40/cluster-autoscaler/utils/taints/taints.go#L41-L42
	DeletionCandidateTaint = "DeletionCandidateOfClusterAutoscaler"
)
