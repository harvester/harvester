package providers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const RKE = "rke"

func IsRKE(ctx context.Context, k8sClient kubernetes.Interface) (bool, error) {
	// if there are windows nodes then this should not be counted as rke.linux
	windowsNodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		Limit:         1,
		LabelSelector: "kubernetes.io/os=windows",
	})
	if err != nil {
		return false, err
	}
	if len(windowsNodes.Items) != 0 {
		return false, nil
	}

	// Any node created by RKE should have the annotation, so just grab 1
	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return false, err
	}

	if len(nodes.Items) == 0 {
		return false, nil
	}

	annos := nodes.Items[0].Annotations
	if _, ok := annos["rke.cattle.io/external-ip"]; ok {
		return true, nil
	}
	if _, ok := annos["rke.cattle.io/internal-ip"]; ok {
		return true, nil
	}
	return false, nil
}
