package kubernetes

import (
	"context"

	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
)

// CreateConfigMap creates a new ConfigMap in the given namespace.
// If the ConfigMap already exists, it will be returned.
func CreateConfigMap(kubeClient kubeclient.Interface, newConfigMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	log := logrus.WithFields(logrus.Fields{
		"kind":      "ConfigMap",
		"name":      newConfigMap.Name,
		"namespace": newConfigMap.Namespace,
	})
	log.Debug("Creating resource")

	configMap, err := kubeClient.CoreV1().ConfigMaps(newConfigMap.Namespace).Create(context.Background(), newConfigMap, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			logrus.WithError(err).Debug("Resource already exists")
			return GetConfigMap(kubeClient, newConfigMap.Namespace, newConfigMap.Name)
		}
		return nil, err
	}

	return configMap, nil
}

// DeleteConfigMap deletes the ConfigMap with the given name in the given namespace.
func DeleteConfigMap(kubeClient kubeclient.Interface, namespace, name string) error {
	log := logrus.WithFields(logrus.Fields{
		"kind": "ConfigMap",
		"name": name,
	})
	log.Debug("Deleting resource")

	err := kubeClient.CoreV1().ConfigMaps(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		logrus.WithError(err).Debug("Resource not found")
		return nil
	}
	return err
}

// GetConfigMap returns the ConfigMap with the given name in the given namespace.
func GetConfigMap(kubeClient kubeclient.Interface, namespace, name string) (*corev1.ConfigMap, error) {
	log := logrus.WithFields(logrus.Fields{
		"kind":      "ConfigMap",
		"name":      name,
		"namespace": namespace,
	})
	log.Trace("Getting resource")

	return kubeClient.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
}
