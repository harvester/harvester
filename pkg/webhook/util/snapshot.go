package util

import (
	"fmt"

	longhorntypes "github.com/longhorn/longhorn-manager/types"
	longhornutil "github.com/longhorn/longhorn-manager/util"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

func CheckTotalSnapshotSizeOnVM(
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	engineCache ctllonghornv1.EngineCache,
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

		if util.GetProvisionedPVCProvisioner(pvc) != longhorntypes.LonghornDriverName {
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
		if util.GetProvisionedPVCProvisioner(pvc) != longhorntypes.LonghornDriverName {
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
