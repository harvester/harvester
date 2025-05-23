package util

import (
	"fmt"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhtypes "github.com/longhorn/longhorn-manager/types"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	corev1 "k8s.io/api/core/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/util"
)

func CheckOnlineExpand(
	pvc *corev1.PersistentVolumeClaim,
	engineCache ctllonghornv1.EngineCache,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
) (bool, error) {
	provisioner, err := getPVCProvisioner(pvc, scCache)
	if err != nil {
		return false, fmt.Errorf("error determining provisioner for PVC %s/%s: %w", pvc.Namespace, pvc.Name, err)
	}

	expandable, err := util.GetCSIOnlineExpandValidation(provisioner, settingCache)
	if err != nil {
		return false, err
	}

	if provisioner != lhtypes.LonghornDriverName || !expandable {
		return expandable, nil
	}

	return checkLonghornExpandability(pvc, engineCache)
}

func getPVCProvisioner(pvc *corev1.PersistentVolumeClaim, scCache ctlstoragev1.StorageClassCache) (string, error) {
	provisioner := util.GetProvisionedPVCProvisioner(pvc, scCache)
	if provisioner == "" {
		return "", fmt.Errorf("failed to determine provisioner for PVC %s/%s", pvc.Namespace, pvc.Name)
	}
	return provisioner, nil
}

func checkLonghornExpandability(pvc *corev1.PersistentVolumeClaim, engineCache ctllonghornv1.EngineCache) (bool, error) {
	if pvc.Spec.VolumeName == "" {
		return false, fmt.Errorf("PVC %s/%s volume name is empty", pvc.Namespace, pvc.Name)
	}

	engine, err := GetLHEngine(engineCache, pvc.Spec.VolumeName)
	if err != nil {
		return false, fmt.Errorf("error getting Longhorn engine for volume %s: %w", pvc.Spec.VolumeName, err)
	}

	if engine.Spec.DataEngine == lhv1beta2.DataEngineTypeV2 {
		return false, nil
	}

	return true, nil
}
