package kubeconfig

import (
	"io"
	"os"
	"path/filepath"

	"k8s.io/client-go/tools/clientcmd"
)

func GetNonInteractiveClientConfig(kubeConfig string) clientcmd.ClientConfig {
	return GetClientConfig(kubeConfig, nil)
}

func GetNonInteractiveClientConfigWithContext(kubeConfig, currentContext string) clientcmd.ClientConfig {
	return GetClientConfigWithContext(kubeConfig, currentContext, nil)
}

func GetInteractiveClientConfig(kubeConfig string) clientcmd.ClientConfig {
	return GetClientConfig(kubeConfig, os.Stdin)
}

func GetClientConfigWithContext(kubeConfig, currentContext string, reader io.Reader) clientcmd.ClientConfig {
	loadingRules := GetLoadingRules(kubeConfig)
	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults, CurrentContext: currentContext}
	return clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, overrides, reader)
}

func GetClientConfig(kubeConfig string, reader io.Reader) clientcmd.ClientConfig {
	return GetClientConfigWithContext(kubeConfig, "", reader)
}

func GetLoadingRules(kubeConfig string) *clientcmd.ClientConfigLoadingRules {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	if kubeConfig != "" {
		loadingRules.ExplicitPath = kubeConfig
	}

	var otherFiles []string
	homeDir, err := os.UserHomeDir()
	if err == nil {
		otherFiles = append(otherFiles, filepath.Join(homeDir, ".kube", "k3s.yaml"))
	}
	otherFiles = append(otherFiles, "/etc/rancher/k3s/k3s.yaml")
	loadingRules.Precedence = append(loadingRules.Precedence, canRead(otherFiles)...)

	return loadingRules
}

func canRead(files []string) (result []string) {
	for _, f := range files {
		_, err := os.ReadFile(f)
		if err == nil {
			result = append(result, f)
		}
	}
	return
}
