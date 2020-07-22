package server

import (
	"fmt"

	"github.com/rancher/wrangler/pkg/kubeconfig"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func GetConfig(kubeConfig string) (clientcmd.ClientConfig, error) {
	if isManual(kubeConfig) {
		return kubeconfig.GetNonInteractiveClientConfig(kubeConfig), nil
	}

	return getEmbedded()
}

func isManual(kubeConfig string) bool {
	if kubeConfig != "" {
		return true
	}
	_, inClusterErr := rest.InClusterConfig()
	return inClusterErr == nil
}

func getEmbedded() (clientcmd.ClientConfig, error) {
	return nil, fmt.Errorf("embedded only supported on linux")
}
