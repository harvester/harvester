package client

import (
	"fmt"

	"github.com/rancher/wrangler/pkg/generated/controllers/core"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	restclient "k8s.io/client-go/rest"
)

// CreateNamespace create namespace by kubeConfig
func CreateNamespace(kubeConfig *restclient.Config, namespace string) error {
	coreFactory, err := core.NewFactoryFromConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("faield to create core factory from kubernetes config, %v", err)
	}

	namespaceController := coreFactory.Core().V1().Namespace()
	_, err = namespaceController.Create(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func GetPodNodeIP(coreFactory *core.Factory, namespace, labelAppValue string) (string, error) {
	podController := coreFactory.Core().V1().Pod()
	nodeController := coreFactory.Core().V1().Node()

	podList, err := podController.List(namespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", labelAppValue),
	})
	if err != nil {
		return "", err
	}

	pod := podList.Items[0]
	nodeInfo, err := nodeController.Get(pod.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	var nodeIP string
	for _, addr := range nodeInfo.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			nodeIP = addr.Address
			break
		}
	}
	return nodeIP, nil
}

func GetNodePortEndPoint(kubeConfig *restclient.Config, namespace, deploymentName, serviceName string) (string, error) {
	coreFactory, err := core.NewFactoryFromConfig(kubeConfig)
	if err != nil {
		return "", fmt.Errorf("faield to create core factory from kubernetes config, %v", err)
	}

	serviceController := coreFactory.Core().V1().Service()
	service, err := serviceController.Get(namespace, serviceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get service %s in %s, %v", serviceName, namespace, err)
	}

	if service.Spec.Type != corev1.ServiceTypeNodePort {
		return "", fmt.Errorf("service type is %s, must be %s", service.Spec.Type, corev1.ServiceTypeNodePort)
	}
	nodePort := service.Spec.Ports[0].NodePort
	nodeIP, err := GetPodNodeIP(coreFactory, namespace, deploymentName)
	if err != nil {
		return "", fmt.Errorf("failed to get pod's node ip in %s, %v", namespace, err)
	}

	return fmt.Sprintf("%s:%d", nodeIP, nodePort), nil
}
