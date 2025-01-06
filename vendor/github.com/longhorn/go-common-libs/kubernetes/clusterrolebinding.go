package kubernetes

import (
	"context"

	"github.com/sirupsen/logrus"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
)

// CreateClusterRoleBinding creates a new ClusterRoleBinding.
// If the ClusterRoleBinding already exists, it will be returned.
func CreateClusterRoleBinding(kubeClient kubeclient.Interface, newClusterRoleBinding *rbacv1.ClusterRoleBinding) (*rbacv1.ClusterRoleBinding, error) {
	log := logrus.WithFields(logrus.Fields{
		"kind": "ClusterRoleBinding",
		"name": newClusterRoleBinding.Name,
	})
	log.Debug("Creating resource")

	clusterRoleBinding, err := kubeClient.RbacV1().ClusterRoleBindings().Create(context.Background(), newClusterRoleBinding, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			logrus.WithError(err).Debug("Resource already exists")
			return GetClusterRoleBinding(kubeClient, newClusterRoleBinding.Name)
		}
		return nil, err
	}

	return clusterRoleBinding, nil
}

// DeleteClusterRoleBinding deletes the ClusterRoleBinding with the given name.
func DeleteClusterRoleBinding(kubeClient kubeclient.Interface, name string) error {
	log := logrus.WithFields(logrus.Fields{
		"kind": "ClusterRoleBinding",
		"name": name,
	})
	log.Debug("Deleting resource")

	err := kubeClient.RbacV1().ClusterRoleBindings().Delete(context.Background(), name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		logrus.WithError(err).Debug("Resource not found")
		return nil
	}
	return err
}

// GetClusterRoleBinding returns the ClusterRoleBinding with the given name.
func GetClusterRoleBinding(kubeClient kubeclient.Interface, name string) (*rbacv1.ClusterRoleBinding, error) {
	log := logrus.WithFields(logrus.Fields{
		"kind": "ClusterRoleBinding",
		"name": name,
	})
	log.Trace("Getting resource")

	return kubeClient.RbacV1().ClusterRoleBindings().Get(context.Background(), name, metav1.GetOptions{})
}
