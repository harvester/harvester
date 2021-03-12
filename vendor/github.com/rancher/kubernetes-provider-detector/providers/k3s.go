package providers

import (
	"context"
	"strings"

	"k8s.io/client-go/kubernetes"
)

const K3s = "k3s"

func IsK3s(ctx context.Context, k8sClient kubernetes.Interface) (bool, error) {
	v, err := k8sClient.Discovery().ServerVersion()
	if err != nil {
		return false, err
	}
	if strings.Contains(v.GitVersion, "+k3s") {
		return true, nil
	}
	return false, nil
}
