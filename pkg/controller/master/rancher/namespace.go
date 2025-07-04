package rancher

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) onNamespaceRemoved(_ string, namespace *corev1.Namespace) (*corev1.Namespace, error) {
	if namespace == nil {
		return namespace, nil
	}

	value := settings.RancherCluster.Get()
	if value == "" {
		return namespace, nil
	}

	rancherClusterConfig, err := settings.DecodeConfig[settings.RancherClusterConfig](value)
	if err != nil {
		return namespace, err
	}

	if !rancherClusterConfig.RemoveUpstreamClusterWhenNamespaceIsDeleted {
		return namespace, nil
	}

	secret, err := h.SecretCache.Get(util.HarvesterSystemNamespaceName, util.RancherClusterConfigSecretName)
	if err != nil {
		return nil, err
	}

	if secret.Data == nil || len(secret.Data["kubeConfig"]) == 0 {
		return nil, fmt.Errorf("kubeConfig is empty in secret %s/%s", util.HarvesterSystemNamespaceName, util.RancherClusterConfigSecretName)
	}

	kc, err := clientcmd.RESTConfigFromKubeConfig(secret.Data["kubeConfig"])
	if err != nil {
		return nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(kc)
	if err != nil {
		return nil, err
	}

	harvesterConfigsGroupVersion := k8sschema.GroupVersionResource{Group: "rke-machine-config.cattle.io", Version: "v1", Resource: "harvesterconfigs"}
	provisioningClusterGroupVersion := k8sschema.GroupVersionResource{Group: "provisioning.cattle.io", Version: "v1", Resource: "clusters"}
	harvesterConfigs, err := dynamicClient.Resource(harvesterConfigsGroupVersion).Namespace("fleet-default").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		logrus.WithError(err).Error("Fail to list harvesterConfigs")
		return nil, err
	}

	for _, harvesterConfig := range harvesterConfigs.Items {
		vmNamespace, ok := harvesterConfig.Object["vmNamespace"].(string)
		if !ok {
			continue
		}
		if vmNamespace == namespace.Name {
			metadata, ok := harvesterConfig.Object["metadata"].(map[string]interface{})
			if !ok {
				continue
			}
			ownerReferences, ok := metadata["ownerReferences"].([]interface{})
			if !ok {
				continue
			}
			for _, o := range ownerReferences {
				ownerReference, ok := o.(map[string]interface{})
				if !ok {
					continue
				}
				clusterName, ok := ownerReference["name"].(string)
				if !ok {
					continue
				}

				logrus.WithFields(logrus.Fields{
					"harvester.namespace": namespace.Name,
					"cluster":             clusterName,
				}).Info("Namespace is removed, removing upstream cluster")
				err = dynamicClient.Resource(provisioningClusterGroupVersion).Namespace("fleet-default").Delete(context.TODO(), clusterName, metav1.DeleteOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						return namespace, nil
					}
					logrus.WithError(err).WithFields(logrus.Fields{
						"harvester.namespace": namespace.Name,
						"cluster":             clusterName,
					}).Info("Fail to remove upstream cluster")
					return nil, err
				}
			}
		}
	}
	return namespace, nil
}
