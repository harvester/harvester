package data

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util"
)

func addNodeCPUModelConfigMap(mgmt *config.Management) error {
	configMaps := mgmt.CoreFactory.Core().V1().ConfigMap()

	nodeCPUModelConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-cpu-model-configuration",
			Namespace: util.HarvesterSystemNamespaceName,
		},
		Data: map[string]string{
			"cpuModels": `totalNodes: 0
globalModels: []
models: {}`,
		},
	}

	if _, err := configMaps.Create(nodeCPUModelConfigMap); err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "Failed to create configmap %s/%s", nodeCPUModelConfigMap.Namespace, nodeCPUModelConfigMap.Name)
	}

	return nil
}
