package providers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const Docker = "docker"

func IsDocker(ctx context.Context, k8sClient kubernetes.Interface) (bool, error) {
	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}

	// Docker is detected by only having one node and validating the node OS
	if len(nodes.Items) == 1 && nodes.Items[0].Status.NodeInfo.OSImage == "Docker Desktop" {
		return true, nil
	}
	return false, nil
}
