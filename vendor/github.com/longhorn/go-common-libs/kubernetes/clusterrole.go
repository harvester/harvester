package kubernetes

import (
	"context"

	"github.com/sirupsen/logrus"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
)

// CreateClusterRole creates a new ClusterRole.
// If the ClusterRole already exists, it will be returned.
func CreateClusterRole(kubeClient kubeclient.Interface, newClusterRole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	log := logrus.WithFields(logrus.Fields{
		"kind": "ClusterRole",
		"name": newClusterRole.Name,
	})
	log.Debug("Creating resource")

	clusterRole, err := kubeClient.RbacV1().ClusterRoles().Create(context.Background(), newClusterRole, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			logrus.WithError(err).Debug("Resource already exists")
			return GetClusterRole(kubeClient, newClusterRole.Name)
		}
		return nil, err
	}

	return clusterRole, nil
}

// DeleteClusterRole deletes the ClusterRole with the given name.
func DeleteClusterRole(kubeClient kubeclient.Interface, name string) error {
	log := logrus.WithFields(logrus.Fields{
		"kind": "ClusterRole",
		"name": name,
	})
	log.Debug("Deleting resource")

	err := kubeClient.RbacV1().ClusterRoles().Delete(context.Background(), name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		logrus.WithError(err).Debug("Resource not found")
		return nil
	}
	return err
}

// GetClusterRole returns the ClusterRole with the given name.
func GetClusterRole(kubeClient kubeclient.Interface, name string) (*rbacv1.ClusterRole, error) {
	log := logrus.WithFields(logrus.Fields{
		"kind": "ClusterRole",
		"name": name,
	})
	log.Trace("Getting resource")

	return kubeClient.RbacV1().ClusterRoles().Get(context.Background(), name, metav1.GetOptions{})
}
