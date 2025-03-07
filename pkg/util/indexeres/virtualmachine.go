package indexeres

import (
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/ref"
)

// The file contains the indexers which are used by controller and webhook.
const (
	VMByPVCIndex        = "harvesterhci.io/vm-by-pvc"
	VMByHotplugPVCIndex = "harvesterhci.io/vm-by-hp-pvc"
)

func VMByPVC(obj *kubevirtv1.VirtualMachine) ([]string, error) {
	var results []string
	if obj == nil || obj.Spec.Template == nil {
		return results, nil
	}

	for _, volume := range obj.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName != "" {
			results = append(results, ref.Construct(obj.Namespace, volume.PersistentVolumeClaim.ClaimName))
		}
	}
	return results, nil
}

func isVolumeHotplugged(volume kubevirtv1.Volume) bool {
	return volume.PersistentVolumeClaim != nil &&
		volume.PersistentVolumeClaim.ClaimName != "" &&
		volume.PersistentVolumeClaim.Hotpluggable
}

func VMByHotplugPVC(obj *kubevirtv1.VirtualMachine) ([]string, error) {
	if obj == nil || obj.Spec.Template == nil {
		return nil, nil
	}

	var results []string
	for _, volume := range obj.Spec.Template.Spec.Volumes {
		if isVolumeHotplugged(volume) {
			results = append(results, ref.Construct(obj.Namespace, volume.PersistentVolumeClaim.ClaimName))
		}
	}
	return results, nil
}
