package backup

import (
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util/datamover"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

func (c repositoryCleaner) onBackup(
	key string, obj *harvesterv1.VirtualMachineBackup,
) (*harvesterv1.VirtualMachineBackup, error) {
	if obj != nil {
		return obj, nil
	}
	return nil, c.deletedParent(key, datamover.BackupKind)
}

func (c repositoryCleaner) onRestore(
	key string, obj *harvesterv1.VirtualMachineRestore,
) (*harvesterv1.VirtualMachineRestore, error) {
	if obj != nil {
		return obj, nil
	}
	return nil, c.deletedParent(key, datamover.RestoreKind)
}

func (c repositoryCleaner) onJob(_ string, obj *batchv1.Job) (*batchv1.Job, error) {
	if obj == nil {
		return nil, nil
	}
	return obj, c.cleanup(obj.ObjectMeta)
}

func (c repositoryCleaner) onClaim(
	_ string, obj *corev1.PersistentVolumeClaim,
) (*corev1.PersistentVolumeClaim, error) {
	if obj == nil || obj.Namespace != joblifecycle.Namespace {
		return obj, nil
	}
	return obj, c.cleanup(obj.ObjectMeta)
}

func (c repositoryCleaner) onVolume(_ string, obj *corev1.PersistentVolume) (*corev1.PersistentVolume, error) {
	if obj == nil {
		return nil, nil
	}
	return obj, c.cleanup(obj.ObjectMeta)
}

func (c repositoryCleaner) onSnapshot(_ string, obj *snapshotv1.VolumeSnapshot) (*snapshotv1.VolumeSnapshot, error) {
	if obj == nil {
		return nil, nil
	}
	return obj, c.cleanup(obj.ObjectMeta)
}

func (c repositoryCleaner) onContent(
	_ string, obj *snapshotv1.VolumeSnapshotContent,
) (*snapshotv1.VolumeSnapshotContent, error) {
	if obj == nil {
		return nil, nil
	}
	return obj, c.cleanup(obj.ObjectMeta)
}
