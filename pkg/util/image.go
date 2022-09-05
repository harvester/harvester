package util

import (
	"fmt"

	longhorntypes "github.com/longhorn/longhorn-manager/types"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func GetBackingImageName(image *harvesterv1.VirtualMachineImage) string {
	return fmt.Sprintf("%s-%s", image.Namespace, image.Name)
}

func GetImageStorageClassName(imageName string) string {
	return fmt.Sprintf("longhorn-%s", imageName)
}

func GetImageStorageClassParameters(image *harvesterv1.VirtualMachineImage) map[string]string {
	params := map[string]string{
		LonghornOptionBackingImageName: GetBackingImageName(image),
	}
	for k, v := range image.Spec.StorageClassParameters {
		params[k] = v
	}
	return params
}

func GetImageDefaultStorageClassParameters() map[string]string {
	return map[string]string{
		longhorntypes.OptionNumberOfReplicas:    "3",
		longhorntypes.OptionStaleReplicaTimeout: "30",
		LonghornOptionMigratable:                "true",
	}
}
