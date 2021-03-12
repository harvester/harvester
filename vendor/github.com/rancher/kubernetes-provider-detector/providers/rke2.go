package providers

import (
	"context"
	"strings"

	"k8s.io/client-go/kubernetes"
)

const RKE2 = "rke2"

func IsRKE2(ctx context.Context, k8sClient kubernetes.Interface) (bool, error) {
	v, err := k8sClient.Discovery().ServerVersion()
	if err != nil {
		return false, err
	}
	if strings.Contains(v.GitVersion, "+rke2") {
		return true, nil
	}
	return false, nil
}
