package util

import "github.com/rancher/wrangler/v3/pkg/name"

func GetRestoreVMConfigMapName(upgradeName string) string {
	return name.SafeConcatName(upgradeName, RestoreVMConfigMap)
}
