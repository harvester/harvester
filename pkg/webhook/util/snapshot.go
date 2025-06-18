package util

import (
	"fmt"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhtypes "github.com/longhorn/longhorn-manager/types"
	longhornutil "github.com/longhorn/longhorn-manager/util"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

func CheckTotalSnapshotSizeOnVM(
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	engineCache ctllonghornv1.EngineCache,
	scCache ctlstoragev1.StorageClassCache,
	vm *kubevirtv1.VirtualMachine,
	totalSnapshotSizeQuota int64,
) error {
	if totalSnapshotSizeQuota == 0 {
		return nil
	}

	var totalSnapshotUsage int64
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}

		pvcNamespace := vm.Namespace
		pvcName := volume.PersistentVolumeClaim.ClaimName

		pvc, err := pvcCache.Get(pvcNamespace, pvcName)
		if err != nil {
			return werror.NewInternalError(fmt.Sprintf("failed to get PVC %s/%s, err: %s", pvcNamespace, pvcName, err))
		}

		if util.GetProvisionedPVCProvisioner(pvc, scCache) != lhtypes.LonghornDriverName {
			continue
		}

		engine, err := GetLHEngine(engineCache, pvc.Spec.VolumeName)
		if err != nil {
			return werror.NewInternalError(fmt.Sprintf("failed to get engine for PVC %s/%s, err: %s", pvcNamespace, pvcName, err))
		}
		for _, snapshot := range engine.Status.Snapshots {
			if snapshot.Removed {
				continue
			}

			snapshotSize, err := longhornutil.ConvertSize(snapshot.Size)
			if err != nil {
				return werror.NewInternalError(fmt.Sprintf("failed to convert snapshot size %s to bytes, err: %s", snapshot.Size, err))
			}
			totalSnapshotUsage += snapshotSize
		}
	}

	if totalSnapshotSizeQuota < totalSnapshotUsage {
		totalSnapshotSizeQuotaQuantity := resource.NewQuantity(totalSnapshotSizeQuota, resource.BinarySI)
		totalSnapshotUsageQuantity := resource.NewQuantity(totalSnapshotUsage, resource.BinarySI)
		return werror.NewInvalidError(fmt.Sprintf("total snapshot size limit %s is less than total snapshot usage %s", totalSnapshotSizeQuotaQuantity, totalSnapshotUsageQuantity), "spec.source.name")
	}
	return nil
}

func CheckTotalSnapshotSizeOnNamespace(
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	engineCache ctllonghornv1.EngineCache,
	scCache ctlstoragev1.StorageClassCache,
	namespaceName string,
	totalSnapshotSizeQuota int64,
) error {
	if totalSnapshotSizeQuota == 0 {
		return nil
	}

	pvcs, err := pvcCache.List(namespaceName, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to list PVCs in Namespace %s, err: %s", namespaceName, err))
	}

	var totalSnapshotUsage int64
	for _, pvc := range pvcs {
		if util.GetProvisionedPVCProvisioner(pvc, scCache) != lhtypes.LonghornDriverName {
			continue
		}

		engine, err := GetLHEngine(engineCache, pvc.Spec.VolumeName)
		if err != nil {
			return werror.NewInternalError(fmt.Sprintf("failed to get engine for PVC %s/%s, err: %s", pvc.Namespace, pvc.Name, err))
		}
		for _, snapshot := range engine.Status.Snapshots {
			if snapshot.Removed {
				continue
			}

			snapshotSize, err := longhornutil.ConvertSize(snapshot.Size)
			if err != nil {
				return werror.NewInternalError(fmt.Sprintf("failed to convert snapshot size %s to bytes, err: %s", snapshot.Size, err))
			}
			totalSnapshotUsage += snapshotSize
		}
	}
	if totalSnapshotSizeQuota < totalSnapshotUsage {
		totalSnapshotSizeQuotaQuantity := resource.NewQuantity(totalSnapshotSizeQuota, resource.BinarySI)
		totalSnapshotUsageQuantity := resource.NewQuantity(totalSnapshotUsage, resource.BinarySI)
		return werror.NewInvalidError(fmt.Sprintf("total snapshot size limit %s is less than total snapshot usage %s", totalSnapshotSizeQuotaQuantity, totalSnapshotUsageQuantity), "spec.source.name")
	}
	return nil
}

type snapshotBackupAbility struct {
	snapshot bool
	backup   bool
}

func (s snapshotBackupAbility) IsSupported(backupType v1beta1.BackupType) bool {
	switch backupType {
	case v1beta1.Backup:
		return s.backup
	case v1beta1.Snapshot:
		return s.snapshot
	default:
		return false
	}
}

// We currently only defines the snapshot/backup ability for LH v1 and v2,
// third-party storage get default ability
// TODO: if we can generate the matrix by leveraging CDI StorageProfile?
func genAbilityMatrix() map[string]snapshotBackupAbility {
	return map[string]snapshotBackupAbility{
		string(lhv1beta2.DataEngineTypeV1) + "." + lhtypes.LonghornDriverName: {true, true},
		string(lhv1beta2.DataEngineTypeV2) + "." + lhtypes.LonghornDriverName: {false, false},
	}
}

func defaultAbility() snapshotBackupAbility {
	return snapshotBackupAbility{snapshot: true, backup: false}
}

func getAbilityForProvisioner(provisioner string) snapshotBackupAbility {
	abilityMatrix := genAbilityMatrix()
	if ability, exists := abilityMatrix[provisioner]; exists {
		return ability
	}
	return defaultAbility()
}

func provisionerWrapper(pvc *corev1.PersistentVolumeClaim, engineCache ctllonghornv1.EngineCache, scCache ctlstoragev1.StorageClassCache) (string, error) {
	provisioner := util.GetProvisionedPVCProvisioner(pvc, scCache)
	if provisioner != lhtypes.LonghornDriverName {
		return provisioner, nil
	}

	engine, err := GetLHEngine(engineCache, pvc.Spec.VolumeName)
	if err != nil {
		return "", err
	}

	return string(engine.Spec.DataEngine) + "." + provisioner, nil
}

func ValidateProvisionerAndConfig(pvc *corev1.PersistentVolumeClaim,
	engineCache ctllonghornv1.EngineCache, scCache ctlstoragev1.StorageClassCache, bt v1beta1.BackupType,
	cdc map[string]settings.CSIDriverInfo) error {

	// Get wrapper provisioner for checking snapshot/backup ability
	provisioner, err := provisionerWrapper(pvc, engineCache, scCache)
	if err != nil {
		return err
	}

	ability := getAbilityForProvisioner(provisioner)
	if !ability.IsSupported(bt) {
		return fmt.Errorf("provisioner %s is not supported for type %s", provisioner, bt)
	}

	// Get origin provisioner and the CSI configuration
	provisioner = util.GetProvisionedPVCProvisioner(pvc, scCache)
	c, ok := cdc[provisioner]
	if !ok {
		return fmt.Errorf("provisioner %s is not configured in the %s setting", provisioner, settings.CSIDriverConfigSettingName)
	}

	// Determine which configuration value is required based on the backup type.
	var requiredValue string
	switch bt {
	case v1beta1.Backup:
		requiredValue = c.BackupVolumeSnapshotClassName
	case v1beta1.Snapshot:
		requiredValue = c.VolumeSnapshotClassName
	}

	if requiredValue == "" {
		return fmt.Errorf("%s's VolumeSnapshotClassName is not configured for provisioner %s in the %s setting",
			bt, provisioner, settings.CSIDriverConfigSettingName)
	}

	return nil
}

// While we try to recover a VMBackup remote, if the volumeBackup has no LonghornBackupName,
// it can be the volume backup is not completed or the volume backup is from third-party storage
// from the misleading items https://github.com/harvester/harvester/issues/7755#issue-2896409886.
// We should reject to recover such VMBackups
func IsLHBackupRelated(vmb *v1beta1.VirtualMachineBackup) error {
	for _, vb := range vmb.Status.VolumeBackups {
		if vb.LonghornBackupName == nil || *vb.LonghornBackupName == "" {
			return fmt.Errorf("vmbackup %s/%s vb %s not from LH, it can't be recovered",
				vmb.Namespace, vmb.Name, *vb.Name)
		}
	}
	return nil
}
