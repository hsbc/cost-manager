package kubernetes

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	apiwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func WaitUntilDeploymentAvailable(ctx context.Context, kubeClient client.WithWatch, deploymentNamespace, deploymentName string) error {
	listerWatcher := NewListerWatcher(ctx, kubeClient, &appsv1.DeploymentList{}, client.InNamespace(deploymentNamespace))
	condition := func(event apiwatch.Event) (bool, error) {
		deployment, err := ParseWatchEventObject[*appsv1.Deployment](event)
		if err != nil {
			return false, err
		}
		return deployment.Name == deploymentName &&
			deployment.Status.AvailableReplicas > 0 &&
			deployment.Generation == deployment.Status.ObservedGeneration, nil
	}
	_, err := watch.UntilWithSync(ctx, listerWatcher, &appsv1.Deployment{}, nil, condition)
	return err
}

func WaitUntilDeploymentUnavailable(ctx context.Context, kubeClient client.WithWatch, deploymentNamespace, deploymentName string) error {
	listerWatcher := NewListerWatcher(ctx, kubeClient, &appsv1.DeploymentList{}, client.InNamespace(deploymentNamespace))
	condition := func(event apiwatch.Event) (bool, error) {
		deployment, err := ParseWatchEventObject[*appsv1.Deployment](event)
		if err != nil {
			return false, err
		}
		return deployment.Name == deploymentName &&
			deployment.Status.AvailableReplicas == 0 &&
			deployment.Generation == deployment.Status.ObservedGeneration, nil
	}
	_, err := watch.UntilWithSync(ctx, listerWatcher, &appsv1.Deployment{}, nil, condition)
	return err
}
