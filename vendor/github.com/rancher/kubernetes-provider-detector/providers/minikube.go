package providers

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const Minikube = "minikube"

func IsMinikube(ctx context.Context, k8sClient kubernetes.Interface) (bool, error) {
	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}

	if len(nodes.Items) == 0 {
		return false, nil
	}

	// Newer versions of minikube apply labels
	labels := nodes.Items[0].Labels
	if _, ok := labels["minikube.k8s.io/name"]; ok {
		return true, nil
	}

	// Older versions don't have labels, check if a minikube image is being used
	for _, image := range nodes.Items[0].Status.Images {
		for _, name := range image.Names {
			if strings.HasPrefix(name, "gcr.io/k8s-minikube/") {
				return true, nil
			}
		}
	}
	return false, nil
}
