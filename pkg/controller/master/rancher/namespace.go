package rancher

import (
	"context"
	"fmt"
	"reflect"
	"time"

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

const (
	projectAnnotationKey   = "field.cattle.io/projectId"
	defaultRecheckInterval = 300 * time.Second
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

// onNamespaceChanged watches changes to kube-system namespace which indicate if the namespace has been moved to a project
// and based on the project annotation, it will add/remove the projectId label to the harvester system namesapces.
func (h *Handler) onNamespaceChanged(key string, namespace *corev1.Namespace) (*corev1.Namespace, error) {
	if namespace == nil || namespace.DeletionTimestamp != nil || namespace.Name != util.KubeSystemNamespace {
		return namespace, nil
	}
	projectID, hasProjectAnnotation := namespace.Annotations[projectAnnotationKey]
	if hasProjectAnnotation {
		if err := h.addProjectIDToHarvesterSystemNamespaces(projectID); err != nil {
			return namespace, err
		}
	} else {
		if err := h.removeProjectIDFromHarvesterSystemNamespaces(); err != nil {
			return namespace, err
		}
	}

	h.NamespaceController.EnqueueAfter(key, defaultRecheckInterval)
	return namespace, nil
}

// addProjectIDToHarvesterSystemNamespaces adds the projectID to the harvester system namespaces if the kube-system namespace is added to a project.
func (h *Handler) addProjectIDToHarvesterSystemNamespaces(projectID string) error {
	for _, ns := range util.DefaultHarvesterNamespaceWhiteList {
		namespace, err := h.NamespaceCache.Get(ns)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("error getting namespace %s: %w", ns, err)
		}

		// namespace is being deleted, skip it
		if namespace.DeletionTimestamp != nil {
			continue
		}

		namespaceCopy := namespace.DeepCopy()
		if namespaceCopy.Annotations == nil {
			namespaceCopy.Annotations = make(map[string]string)
		}
		namespaceCopy.Annotations[projectAnnotationKey] = projectID
		if !reflect.DeepEqual(namespace, namespaceCopy) {
			_, err = h.NamespaceClient.Update(namespaceCopy)
			if err != nil {
				return fmt.Errorf("error updating namespace %s: %w", ns, err)
			}
		}
	}
	return nil
}

// removeProjectIDFromHarvesterSystemNamespaces removes the projectID from the harvester system namespaces if the kube-system namespace is removed from a project.
func (h *Handler) removeProjectIDFromHarvesterSystemNamespaces() error {
	for _, ns := range util.DefaultHarvesterNamespaceWhiteList {
		namespace, err := h.NamespaceCache.Get(ns)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("error getting namespace %s: %w", ns, err)
		}

		// namespace is being deleted, skip it
		if namespace.DeletionTimestamp != nil {
			continue
		}

		namespaceCopy := namespace.DeepCopy()
		if namespaceCopy.Annotations != nil {
			delete(namespaceCopy.Annotations, projectAnnotationKey)
		}
		if !reflect.DeepEqual(namespace, namespaceCopy) {
			_, err = h.NamespaceClient.Update(namespaceCopy)
			if err != nil {
				return fmt.Errorf("error updating namespace %s: %w", ns, err)
			}
		}
	}
	return nil
}
