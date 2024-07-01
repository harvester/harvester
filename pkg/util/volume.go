package util

import (
	"encoding/json"
	"fmt"
	"strconv"

	lhutil "github.com/longhorn/longhorn-manager/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

const (
	AnnStorageProvisioner     = "volume.kubernetes.io/storage-provisioner"
	AnnBetaStorageProvisioner = "volume.beta.kubernetes.io/storage-provisioner"
	LonghornDataLocality      = "dataLocality"
)

var (
	PersistentVolumeClaimsKind = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}
)

// GetProvisionedPVCProvisioner do not use this function when the PVC is just created
func GetProvisionedPVCProvisioner(pvc *corev1.PersistentVolumeClaim) string {
	provisioner, ok := pvc.Annotations[AnnBetaStorageProvisioner]
	if !ok {
		provisioner = pvc.Annotations[AnnStorageProvisioner]
	}
	return provisioner
}

// LoadCSIDriverConfig loads the CSI driver configuration from settings.
func LoadCSIDriverConfig(settingCache ctlharvesterv1.SettingCache) (map[string]settings.CSIDriverInfo, error) {
	csiDriverConfigSetting, err := settingCache.Get(settings.CSIDriverConfigSettingName)
	if err != nil {
		return nil, fmt.Errorf("can't get %s setting, err: %w", settings.CSIDriverConfigSettingName, err)
	}
	csiDriverConfigSettingValue := csiDriverConfigSetting.Default
	if csiDriverConfigSetting.Value != "" {
		csiDriverConfigSettingValue = csiDriverConfigSetting.Value
	}
	csiDriverConfig := make(map[string]settings.CSIDriverInfo)
	if err := json.Unmarshal([]byte(csiDriverConfigSettingValue), &csiDriverConfig); err != nil {
		return nil, fmt.Errorf("can't parse %s setting, err: %w", settings.CSIDriverConfigSettingName, err)
	}
	return csiDriverConfig, nil
}

// GetSnapshotMaxCountAndSize returns snapshot max count and size from PVC annotations.
// If there is no snapshot max count annotation, return default value 250.
// If there is no snapshot max size annotation, return default value 0.
func GetSnapshotMaxCountAndSize(pvc *corev1.PersistentVolumeClaim) (snapshotMaxCount int, snapshotMaxSize int64, err error) {
	if snapshotMaxCountString, ok := pvc.Annotations[AnnotationSnapshotMaxCount]; ok {
		snapshotMaxCount, err = strconv.Atoi(snapshotMaxCountString)
		if err != nil {
			return 0, 0, fmt.Errorf("snapshot max count should be a number, err: %w", err)
		}
	} else {
		snapshotMaxCount = settings.DefaultSnapshotMaxCount.GetInt()
	}

	if snapshotMaxSizeString, ok := pvc.Annotations[AnnotationSnapshotMaxSize]; ok {
		snapshotMaxSize, err = lhutil.ConvertSize(snapshotMaxSizeString)
		if err != nil {
			return snapshotMaxCount, 0, fmt.Errorf("snapshot max size is not a valid resource quantity")
		}
	}
	return snapshotMaxCount, snapshotMaxSize, nil
}
