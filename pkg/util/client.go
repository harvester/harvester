package util

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"kubevirt.io/kubevirt/pkg/virt-operator/resource/generate/components"
)

func VirtClientUpdateVmi(ctx context.Context, client rest.Interface, managementNamespace, namespace, name string, obj runtime.Object) error {
	return client.Put().
		Namespace(namespace).
		SetHeader("Impersonate-User", fmt.Sprintf("system:serviceaccount:%s:%s", managementNamespace, components.ControllerServiceAccountName)).
		Resource("virtualmachineinstances").
		Name(name).
		Body(obj).
		Do(ctx).
		Error()
}

func getK8sClientsetInCluster() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func getK8sClientsetLocal() (*kubernetes.Clientset, error) {
	configStr := os.Getenv("KUBECONFIG")
	if configStr == "" {
		return nil, fmt.Errorf("unable to get local config, please make sure the kubeconfig is set")
	}
	config, err := clientcmd.BuildConfigFromFlags("", configStr)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func GetK8sClientset() (*kubernetes.Clientset, error) {
	if clientset, err := getK8sClientsetInCluster(); err == nil {
		return clientset, nil
	}
	logrus.Debugf("Unable to get in cluster config, trying local config ...")
	if clientset, err := getK8sClientsetLocal(); err == nil {
		return clientset, nil
	}
	return nil, fmt.Errorf("unable to get local config, please make sure the kubeconfig is set")
}
