package kubernetes

import (
	"context"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/labels"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
)

// GetDeployment returns the Deployment with the given name in the given namespace.
func GetDeployment(kubeClient kubeclient.Interface, namespace, name string) (*appsv1.Deployment, error) {
	log := logrus.WithFields(logrus.Fields{
		"kind":      "Deployment",
		"namespace": namespace,
		"name":      name,
	})
	log.Trace("Getting resource")

	return kubeClient.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

// ListDeployments lists Kubernetes deployments in a specified namespace that match the provided label selectors.
//
// Parameters:
// - kubeClient: The Kubernetes client interface used to interact with the cluster.
// - namespace: The namespace in which to list the deployments.
// - labelSelectors: A map of label selectors to filter the deployments. If empty, all deployments in the namespace are listed.
//
// Returns:
// - A DeploymentList containing the deployments that match the label selectors.
// - An error if there is any issue in listing the deployments.
func ListDeployments(kubeClient kubeclient.Interface, namespace string, labelSelectors map[string]string) (*appsv1.DeploymentList, error) {
	log := logrus.WithFields(logrus.Fields{
		"kind":           "Deployment",
		"namespace":      namespace,
		"labelSelectors": labelSelectors,
	})
	log.Trace("Listing resources")

	var err error
	selector := labels.Everything()
	if len(labelSelectors) != 0 {
		selector, err = NewLabelSelectorFromMap(labelSelectors)
		if err != nil {
			return nil, err
		}
	}

	return kubeClient.AppsV1().Deployments(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: selector.String()})
}
