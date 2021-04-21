package ready

import (
	"github.com/rancher/wrangler/pkg/generated/controllers/apps"
	v1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func isDeploymentClean(appsFactory *apps.Factory, namespace, name string) (string, wait.ConditionFunc) {
	return "clean", func() (bool, error) {
		deploymentController := appsFactory.Apps().V1().Deployment()
		_, err := deploymentController.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return true, err
		}
		return false, nil
	}
}

func isDeploymentReady(appsFactory *apps.Factory, namespace, name string) (string, wait.ConditionFunc) {
	return "ready", func() (done bool, err error) {
		deploymentController := appsFactory.Apps().V1().Deployment()
		deployment, err := deploymentController.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return true, err
		}
		return isDeploymentStatusReady(&deployment.Status), nil
	}
}

// isDeploymentStatusReady checks if the Deployment is ready.
func isDeploymentStatusReady(status *v1.DeploymentStatus) bool {
	if status == nil {
		return false
	}
	return status.Replicas == status.AvailableReplicas
}
