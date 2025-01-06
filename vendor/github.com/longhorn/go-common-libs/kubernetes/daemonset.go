package kubernetes

import (
	"context"

	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
)

// CreateDaemonSet creates a new DaemonSet in the given namespace.
// If the DaemonSet already exists, it will be returned.
func CreateDaemonSet(kubeClient kubeclient.Interface, newDaemonSet *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	if newDaemonSet.Namespace == "" {
		newDaemonSet.Namespace = metav1.NamespaceDefault
	}

	log := logrus.WithFields(logrus.Fields{
		"kind":      "DaemonSet",
		"namespace": newDaemonSet.Namespace,
		"name":      newDaemonSet.Name,
	})
	log.Debug("Creating resource")

	daemonSet, err := kubeClient.AppsV1().DaemonSets(newDaemonSet.Namespace).Create(context.Background(), newDaemonSet, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			logrus.WithError(err).Debug("Resource already exists")
			return GetDaemonSet(kubeClient, newDaemonSet.Namespace, newDaemonSet.Name)
		}
		return nil, err
	}

	return daemonSet, nil
}

// DeleteDaemonSet deletes the DaemonSet with the given name in the given namespace.
func DeleteDaemonSet(kubeClient kubeclient.Interface, namespace, name string) error {
	log := logrus.WithFields(logrus.Fields{
		"kind":      "DaemonSet",
		"namespace": namespace,
		"name":      name,
	})
	log.Debug("Deleting resource")

	err := kubeClient.AppsV1().DaemonSets(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		logrus.WithError(err).Debug("Resource not found")
		return nil
	}
	return err
}

// GetDaemonSet returns the DaemonSet with the given name in the given namespace.
func GetDaemonSet(kubeClient kubeclient.Interface, namespace, name string) (*appsv1.DaemonSet, error) {
	log := logrus.WithFields(logrus.Fields{
		"kind":      "DaemonSet",
		"namespace": namespace,
		"name":      name,
	})
	log.Trace("Getting resource")

	return kubeClient.AppsV1().DaemonSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

// IsDaemonSetReady checks if the DaemonSet is ready by comparing the number of ready pods with the desired number of pods.
func IsDaemonSetReady(daemonSet *appsv1.DaemonSet) bool {
	return daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled
}
