package util

import (
	"reflect"

	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/name"
	corev1 "k8s.io/api/core/v1"
)

func GetRestoreVMConfigMapName(upgradeName string) string {
	return name.SafeConcatName(upgradeName, RestoreVMConfigMap)
}

// UpdateConfigMapData updates the given ConfigMap only when its Data field
// differs from the provided data map. If no change is needed, it returns
// the original ConfigMap unchanged.
func UpdateConfigMapData(cmClient v1.ConfigMapClient, cm *corev1.ConfigMap, data map[string]string) (*corev1.ConfigMap, error) {
	if !reflect.DeepEqual(cm.Data, data) {
		toUpdate := cm.DeepCopy()
		toUpdate.Data = data
		return cmClient.Update(toUpdate)
	}
	return cm, nil
}
