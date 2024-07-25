package ready

import (
	"github.com/rancher/wrangler/v3/pkg/generated/controllers/apps"
	v1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func isDaemenSetClean(appsFactory *apps.Factory, namespace, name string) (string, wait.ConditionFunc) {
	return "clean", func() (bool, error) {
		daemonSetController := appsFactory.Apps().V1().DaemonSet()
		_, err := daemonSetController.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return true, err
		}
		return false, nil
	}
}

func isDaemenSetReady(appsFactory *apps.Factory, namespace, name string) (string, wait.ConditionFunc) {
	return "ready", func() (done bool, err error) {
		daemonSetController := appsFactory.Apps().V1().DaemonSet()
		daemonSet, err := daemonSetController.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return true, err
		}
		return isDaemonSetStatusReady(&daemonSet.Status), nil
	}
}

// isDaemonSetStatusReady checks if the DaemonSet is ready.
func isDaemonSetStatusReady(status *v1.DaemonSetStatus) bool {
	if status == nil {
		return false
	}
	return status.CurrentNumberScheduled == status.NumberReady
}
