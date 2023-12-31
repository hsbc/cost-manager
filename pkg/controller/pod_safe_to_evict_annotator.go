package controller

import (
	"context"

	"github.com/hsbc/cost-manager/pkg/api/v1alpha1"
	"github.com/hsbc/cost-manager/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	podSafeToEvictAnnotatorControllerName = "pod-safe-to-evict-annotator"

	// We copy the annotation key to avoid depending on the autoscaler respository:
	// https://github.com/kubernetes/autoscaler/blob/389914758265a33e36683d6df7dbecf91de81802/cluster-autoscaler/utils/drain/drain.go#L33-L35
	podSafeToEvictAnnotationKey = "cluster-autoscaler.kubernetes.io/safe-to-evict"
)

// podSafeToEvictAnnotator adds the `cluster-autoscaler.kubernetes.io/safe-to-evict: "true"`
// annotation to Pods to ensure that they do not prevent cluster scale down:
// https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#what-types-of-pods-can-prevent-ca-from-removing-a-node
type podSafeToEvictAnnotator struct {
	Config *v1alpha1.PodSafeToEvictAnnotator
	Client client.Client
}

var _ reconcile.Reconciler = &podSafeToEvictAnnotator{}

func (r *podSafeToEvictAnnotator) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}

func (r *podSafeToEvictAnnotator) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// We do nothing if the Namespace does not match the Namespace selector...
	namespace := &corev1.Namespace{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: request.Namespace}, namespace)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}
	namespaceSelectorMatchesLabels, err := r.namespaceSelectorMatchesLabels(namespace)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !namespaceSelectorMatchesLabels {
		return reconcile.Result{}, nil
	}

	// ...otherwise we continue to process the Pod
	pod := &corev1.Pod{}
	err = r.Client.Get(ctx, request.NamespacedName, pod)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	// If the annotation is not already set then we set it to true
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	_, ok := pod.Annotations[podSafeToEvictAnnotationKey]
	if ok {
		return reconcile.Result{}, nil
	}
	// https://github.com/kubernetes/autoscaler/blob/389914758265a33e36683d6df7dbecf91de81802/cluster-autoscaler/utils/drain/drain.go#L118-L121
	pod.Annotations[podSafeToEvictAnnotationKey] = "true"

	err = r.Client.Update(ctx, pod)
	// If the Pod has been deleted or there was a conflict then we ignore the error since there must
	// be another event queued for reconciliation
	if errors.IsNotFound(err) || errors.IsConflict(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *podSafeToEvictAnnotator) namespaceSelectorMatchesLabels(namespace *corev1.Namespace) (bool, error) {
	// If the Namespace selector is nil then we match all Namespaces...
	if r.Config == nil || r.Config.NamespaceSelector == nil {
		return true, nil
	}
	// ...otherwise we match the Namespace against the selector
	return kubernetes.SelectorMatchesLabels(r.Config.NamespaceSelector, namespace.Labels)
}
