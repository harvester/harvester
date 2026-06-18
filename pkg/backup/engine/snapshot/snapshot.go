package snapshot

import (
	"context"
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
)

type SnapshotEngine struct {
	vmbo     common.VMBackupOperator
	vsHelper *common.VolumeSnapshotHelper
	pvcCache ctlcorev1.PersistentVolumeClaimCache
	scCache  ctlstoragev1.StorageClassCache
}

func GetBackupEngine(
	vmbo common.VMBackupOperator,
	vsCache ctlsnapshotv1.VolumeSnapshotCache,
	vsClient ctlsnapshotv1.VolumeSnapshotClient,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	scCache ctlstoragev1.StorageClassCache,
) engine.BackupEngine {
	return &SnapshotEngine{
		vmbo:     vmbo,
		vsHelper: common.NewVolumeSnapshotHelper(vsCache, vsClient, nil, nil, vmbo, pvcCache, scCache),
		pvcCache: pvcCache,
		scCache:  scCache,
	}
}

func (se *SnapshotEngine) Reconcile(
	vmb *harvesterv1.VirtualMachineBackup,
	volIndex int,
	vsClassMap map[string]snapshotv1.VolumeSnapshotClass,
) error {
	logrus.Infof("SnapshotEngine Reconcile called for VMBackup %s/%s volume index %d",
		se.vmbo.GetNamespace(vmb), se.vmbo.GetName(vmb), volIndex)

	vb := se.vmbo.GetVolBackup(vmb, volIndex)
	if vb == nil {
		return fmt.Errorf("volume backup at index %d not found", volIndex)
	}

	vsName := *se.vmbo.GetVolBackupName(vb)
	vs, err := se.vsHelper.GetVolumeSnapshot(se.vmbo.GetNamespace(vmb), vsName)
	if err != nil {
		return err
	}

	if err := se.vsHelper.CheckSnapshotDeletionStatus(vs, vmb, vb); err != nil {
		return err
	}

	if vs != nil {
		return se.vsHelper.UpdateVolumeBackupStatus(vb, vs)
	}

	_, err = se.ensureVolumeSnapshotExists(vmb, vb, vsClassMap)
	return err
}

// ensureVolumeSnapshotExists creates a new volume snapshot if it doesn't exist
func (se *SnapshotEngine) ensureVolumeSnapshotExists(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	vscMap map[string]snapshotv1.VolumeSnapshotClass,
) (*snapshotv1.VolumeSnapshot, error) {
	if err := se.vsHelper.TryFreezeFS(context.Background(), vmb); err != nil {
		return nil, err
	}

	vsClass := vscMap[se.vmbo.GetVolBackupCSIDriver(vb)]
	ownerRef := se.vsHelper.BuildOwnerReference(vmb)

	return se.vsHelper.CreateVolumeSnapshotFromPVC(vmb, vb, &vsClass, ownerRef)
}

func (se *SnapshotEngine) UpdateProgress(vb *harvesterv1.VolumeBackup) (int64, error) {
	if se.vmbo.GetVolBackupReadyToUse(vb) {
		return 100, nil
	}
	return 0, nil
}

func (se *SnapshotEngine) ForceDelete(vmb *harvesterv1.VirtualMachineBackup, volIndex int) error {
	return nil
}

// RegisterWatchers is a no-op for the snapshot engine; it owns no external
// resources whose changes need to feed back into the VMBackup reconcile loop.
func (se *SnapshotEngine) RegisterWatchers(_ context.Context, _ func(string, string)) {}
