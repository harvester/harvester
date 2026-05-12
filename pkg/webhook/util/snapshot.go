package util

import (
	"fmt"

	lhtypes "github.com/longhorn/longhorn-manager/types"
	longhornutil "github.com/longhorn/longhorn-manager/util"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
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

// ValidateProvisionerAndConfig checks that the PVC's CSI driver has the
// snapshot class required by the requested backup type configured in the
// CSIDriverConfig setting. The presence of a configured class is the real
// admission gate — a provisioner that can't satisfy the type would have no
// reason to have the class configured. (The earlier per-provisioner ability
// matrix was a legacy artifact from when Longhorn v2 lacked feature parity
// with v1; today both data engines support snapshot+backup, and third-party
// drivers self-declare capability by virtue of being configured here.)
func ValidateProvisionerAndConfig(pvc *corev1.PersistentVolumeClaim,
	scCache ctlstoragev1.StorageClassCache, bt v1beta1.BackupType,
	cdc map[string]settings.CSIDriverInfo) error {

	provisioner := util.GetProvisionedPVCProvisioner(pvc, scCache)
	c, ok := cdc[provisioner]
	if !ok {
		return fmt.Errorf("provisioner %s is not configured in the %s setting", provisioner, settings.CSIDriverConfigSettingName)
	}

	var requiredValue string
	switch {
	case bt.UsesRemoteBackupTarget():
		requiredValue = c.BackupVolumeSnapshotClassName
	case bt == v1beta1.Snapshot:
		requiredValue = c.VolumeSnapshotClassName
	}
	if requiredValue == "" {
		return fmt.Errorf("%s's snapshot class is not configured for provisioner %s in the %s setting",
			bt, provisioner, settings.CSIDriverConfigSettingName)
	}
	return nil
}

// While we try to recover a VMBackup remote, if the volumeBackup has no LonghornBackupName,
// it can be the volume backup is not completed or the volume backup is from third-party storage
// from the misleading items https://github.com/harvester/harvester/issues/7755#issue-2896409886.
// We should reject to recover such VMBackups
func IsLHBackupRelated(vmb *v1beta1.VirtualMachineBackup, vmbo common.VMBackupOperator) error {
	vbs := vmbo.GetVolBackups(vmb)
	for index, vb := range vbs {
		lhname := vmbo.GetVolBackupLHBackupName(&vb)
		if lhname != nil && *lhname != "" {
			continue
		}
		vbName := ""
		if name := vmbo.GetVolBackupName(&vb); name != nil {
			vbName = *name
		}
		return fmt.Errorf("vmbackup %s/%s vb %s at index %d not from LH, it can't be recovered",
			vmbo.GetNamespace(vmb), vmbo.GetName(vmb), vbName, index)
	}

	return nil
}
