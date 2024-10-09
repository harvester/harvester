package util

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
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
	csiDriverConfigSettingDefault := csiDriverConfigSetting.Default
	csiDriverConfig := make(map[string]settings.CSIDriverInfo)
	if err := json.Unmarshal([]byte(csiDriverConfigSettingDefault), &csiDriverConfig); err != nil {
		return nil, fmt.Errorf("can't parse %s setting, err: %w", settings.CSIDriverConfigSettingName, err)
	}
	if csiDriverConfigSetting.Value != "" {
		csiDriverConfigSettingValue := csiDriverConfigSetting.Value
		tmpDriverConfig := make(map[string]settings.CSIDriverInfo)
		if err := json.Unmarshal([]byte(csiDriverConfigSettingValue), &tmpDriverConfig); err != nil {
			return nil, fmt.Errorf("can't parse %s setting, err: %w", settings.CSIDriverConfigSettingName, err)
		}
		for key, val := range tmpDriverConfig {
			if _, ok := csiDriverConfig[key]; !ok {
				csiDriverConfig[key] = val
			} else {
				logrus.Infof("CSI driver %s is already configured in the Default setting, skip it", key)
			}
		}
	}
	return csiDriverConfig, nil
}
