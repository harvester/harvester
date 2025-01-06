package kubernetes

import (
	"context"

	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
)

func CreateServiceAccount(kubeClient kubeclient.Interface, newServiceAccount *corev1.ServiceAccount) (*corev1.ServiceAccount, error) {
	if newServiceAccount.Namespace == "" {
		newServiceAccount.Namespace = metav1.NamespaceDefault
	}

	log := logrus.WithFields(logrus.Fields{
		"kind":      "ServiceAccount",
		"namespace": newServiceAccount.Namespace,
		"name":      newServiceAccount.Name,
	})
	log.Debug("Creating resource")

	serviceAccount, err := kubeClient.CoreV1().ServiceAccounts(newServiceAccount.Namespace).Create(context.Background(), newServiceAccount, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			logrus.WithError(err).Debug("Resource already exists")
			return GetServiceAccount(kubeClient, newServiceAccount.Namespace, newServiceAccount.Name)
		}
		return nil, err
	}

	return serviceAccount, nil
}

func DeleteServiceAccount(kubeClient kubeclient.Interface, namespace, name string) error {
	log := logrus.WithFields(logrus.Fields{
		"kind":      "ServiceAccount",
		"namespace": namespace,
		"name":      name,
	})
	log.Debug("Deleting resource")

	err := kubeClient.CoreV1().ServiceAccounts(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		logrus.WithError(err).Debug("Resource not found")
		return nil
	}
	return err
}

func GetServiceAccount(kubeClient kubeclient.Interface, namespace, name string) (*corev1.ServiceAccount, error) {
	log := logrus.WithFields(logrus.Fields{
		"kind":      "ServiceAccount",
		"namespace": namespace,
		"name":      name,
	})
	log.Trace("Getting resource")

	return kubeClient.CoreV1().ServiceAccounts(namespace).Get(context.Background(), name, metav1.GetOptions{})
}
