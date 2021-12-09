package providers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const Harvester = "harvester"

func IsHarvester(ctx context.Context, k8sClient kubernetes.Interface) (bool, error) {
	// Look for nodes that have an Harvester specific label
	listOpts := metav1.ListOptions{
		LabelSelector: "harvesterhci.io/managed",
		// Only need one
		Limit: 1,
	}

	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, listOpts)
	if err != nil {
		return false, err
	}

	return len(nodes.Items) > 0, nil
}
