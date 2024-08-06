package util

import (
	"fmt"

	longhornutil "github.com/longhorn/longhorn-manager/util"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

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
		return werror.NewInvalidError(fmt.Sprintf("total snapshot size limit %d is less than total snapshot usage %d", totalSnapshotSizeQuota, totalSnapshotUsage), "spec.source.name")
	}
	return nil
}
