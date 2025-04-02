package util

import (
	"encoding/json"
	"fmt"

	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
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
	IndexPodByPVC             = "indexPodByPVC"
)

var (
	PersistentVolumeClaimsKind = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}
)

func GetCSIProvisionerSnapshotCapability(provisioner string) bool {
	csiDriverConfig := make(map[string]settings.CSIDriverInfo)
	if err := json.Unmarshal([]byte(settings.CSIDriverConfig.Get()), &csiDriverConfig); err != nil {
		logrus.Warnf("Failed to unmarshal CSIDriverConfig setting, err: %v", err)
		return false
	}
	CSIConfigs, find := csiDriverConfig[provisioner]
	logrus.Debugf("Provisioner: %v, CSIConfigs: %+v", provisioner, CSIConfigs)
	if find && CSIConfigs.VolumeSnapshotClassName != "" {
		return true
	}

	return false
}

// GetProvisionedPVCProvisioner do not use this function when the PVC is just created
func GetProvisionedPVCProvisioner(pvc *corev1.PersistentVolumeClaim, scCache ctlstoragev1.StorageClassCache) string {
	provisioner, ok := pvc.Annotations[AnnBetaStorageProvisioner]
	if !ok {
		provisioner = pvc.Annotations[AnnStorageProvisioner]
	}
	if provisioner == "" {
		// fallback, fetch provisioner from storage class
		if pvc.Spec.StorageClassName == nil {
			logrus.Warnf("PVC %s/%s does not have storage class name, maybe just created", pvc.Namespace, pvc.Name)
			return provisioner
		}
		targetSCName := *pvc.Spec.StorageClassName
		sc, err := scCache.Get(targetSCName)
		if err != nil {
			logrus.Errorf("failed to get storage class %s, %v", targetSCName, err)
			return provisioner
		}
		provisioner = sc.Provisioner
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

func IndexPodByPVCFunc(pod *corev1.Pod) ([]string, error) {
	if pod.Status.Phase != corev1.PodRunning {
		return nil, nil
	}
	indexes := []string{}
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			index := fmt.Sprintf("%s-%s", pod.Namespace, volume.PersistentVolumeClaim.ClaimName)
			indexes = append(indexes, index)
		}
	}
	return indexes, nil
}
