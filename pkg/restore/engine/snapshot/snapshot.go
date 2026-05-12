package snapshot

import (
	"context"
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/restore/engine"
	"github.com/harvester/harvester/pkg/restore/pvchelper"
)

// SnapshotRestoreEngine implements RestoreEngine for local CSI snapshot restores
type SnapshotRestoreEngine struct {
	vmbo      common.VMBackupOperator
	vmro      restorecommon.VMRestoreOperator
	pvcCache  ctlcorev1.PersistentVolumeClaimCache
	pvcClient ctlcorev1.PersistentVolumeClaimClient
	vsCache   ctlsnapshotv1.VolumeSnapshotCache
}

func GetRestoreEngine(
	vmbo common.VMBackupOperator,
	vmro restorecommon.VMRestoreOperator,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	pvcClient ctlcorev1.PersistentVolumeClaimClient,
	snapshotCache ctlsnapshotv1.VolumeSnapshotCache,
) engine.RestoreEngine {
	return &SnapshotRestoreEngine{
		vmbo:      vmbo,
		vmro:      vmro,
		pvcCache:  pvcCache,
		pvcClient: pvcClient,
		vsCache:   snapshotCache,
	}
}

func (sre *SnapshotRestoreEngine) Reconcile(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	volIndex int,
) error {
	vr := sre.vmro.GetVolRestore(vmr, volIndex)
	if vr == nil {
		return fmt.Errorf("volume restore at index %d not found", volIndex)
	}

	pvcName := sre.vmro.GetVolRestorePVCName(vr)
	namespace := sre.vmro.GetNamespace(vmr)

	// Check if PVC already exists
	pvc, err := sre.pvcCache.Get(namespace, pvcName)
	if apierrors.IsNotFound(err) {
		// Create PVC from VolumeSnapshot
		vb := sre.vmbo.GetVolBackup(vmb, volIndex)
		return sre.createPVCFromSnapshot(vmr, vr, vb)
	}

	if err != nil {
		return fmt.Errorf("failed to get PVC %s/%s: %w", namespace, pvcName, err)
	}

	// PVC exists, check its status
	return sre.checkPVCStatus(pvc)
}

func (sre *SnapshotRestoreEngine) createPVCFromSnapshot(
	vmr *harvesterv1.VirtualMachineRestore,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) error {
	vsName, err := sre.getSnapshotName(vmr, vb)
	if err != nil {
		return err
	}

	return sre.createPVC(vmr, vr, vb, vsName)
}

func (sre *SnapshotRestoreEngine) getSnapshotName(
	vmr *harvesterv1.VirtualMachineRestore,
	vb *harvesterv1.VolumeBackup,
) (string, error) {
	vbName := sre.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return "", fmt.Errorf("%w in volume backup metadata", common.ErrVolumeBackupNameNil)
	}

	if err := sre.validateNamespaces(vmr, vb); err != nil {
		return "", err
	}

	if err := sre.validateSnapshot(vmr, *vbName); err != nil {
		return "", err
	}

	return *vbName, nil
}

func (sre *SnapshotRestoreEngine) validateNamespaces(
	vmr *harvesterv1.VirtualMachineRestore,
	vb *harvesterv1.VolumeBackup,
) error {
	restoreNamespace := sre.vmro.GetNamespace(vmr)
	backupPVCNamespace := sre.vmbo.GetVolBackupPVCNameSpace(vb)

	if restoreNamespace != backupPVCNamespace {
		return fmt.Errorf(
			"snapshot restore across namespaces is not supported for local snapshots: "+
				"restore namespace %s differs from backup PVC namespace %s",
			restoreNamespace, backupPVCNamespace,
		)
	}
	return nil
}

func (sre *SnapshotRestoreEngine) validateSnapshot(
	vmr *harvesterv1.VirtualMachineRestore,
	vbName string,
) error {
	namespace := sre.vmro.GetNamespace(vmr)
	snapshot, err := sre.vsCache.Get(namespace, vbName)

	if apierrors.IsNotFound(err) {
		return fmt.Errorf(
			"VolumeSnapshot %s not found in namespace %s (referenced by volume backup)",
			vbName, namespace,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to get VolumeSnapshot %s/%s: %w", namespace, vbName, err)
	}

	if snapshot.Status == nil || snapshot.Status.ReadyToUse == nil || !*snapshot.Status.ReadyToUse {
		return fmt.Errorf("VolumeSnapshot %s/%s is not ready to use", namespace, vbName)
	}

	return nil
}

func (sre *SnapshotRestoreEngine) createPVC(
	vmr *harvesterv1.VirtualMachineRestore,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
	vsName string,
) error {
	pvc := sre.buildPVC(vmr, vr, vb, vsName)
	_, err := sre.pvcClient.Create(pvc)
	return err
}

func (sre *SnapshotRestoreEngine) buildPVC(
	vmr *harvesterv1.VirtualMachineRestore,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
	vsName string,
) *corev1.PersistentVolumeClaim {
	pvcName := sre.vmro.GetVolRestorePVCName(vr)
	namespace := sre.vmro.GetNamespace(vmr)
	pvcSpec := sre.vmbo.GetVolBackupPVCSpec(vb)
	annotations := sre.buildAnnotations(vmr, vb)
	labels := sre.buildLabels(vb)

	return pvchelper.BuildPVCFromSnapshot(namespace, pvcName, vsName, labels, annotations, pvcSpec)
}

func (sre *SnapshotRestoreEngine) buildAnnotations(
	vmr *harvesterv1.VirtualMachineRestore,
	vb *harvesterv1.VolumeBackup,
) map[string]string {
	sourceAnnotations := sre.vmbo.GetVolBackupPVCAnnotations(vb)
	restoreName := sre.vmro.GetName(vmr)
	return pvchelper.BuildRestoreAnnotations(sourceAnnotations, restoreName, restorecommon.RestoreNameAnnotation)
}

func (sre *SnapshotRestoreEngine) buildLabels(vb *harvesterv1.VolumeBackup) map[string]string {
	// Strip CDI ownership markers so CDI doesn't latch onto the restored PVC.
	return pvchelper.BuildRestoreLabels(sre.vmbo.GetVolBackupPVCLabels(vb))
}

func (sre *SnapshotRestoreEngine) checkPVCStatus(pvc *corev1.PersistentVolumeClaim) error {
	return pvchelper.CheckPVCStatus(pvc)
}

func (sre *SnapshotRestoreEngine) UpdateProgress(vr *harvesterv1.VolumeRestore) (int64, error) {
	// For local snapshots, we don't have detailed progress tracking
	// Return 100 if it's complete (handled by PVC status check in Create)
	return 100, nil
}

func (sre *SnapshotRestoreEngine) Delete(vmr *harvesterv1.VirtualMachineRestore, volIndex int) error {
	// Cleanup is handled by owner references
	return nil
}

// RegisterWatchers is a no-op: the snapshot restore engine has no extra
// external resources to watch beyond what the controller already wires up.
func (sre *SnapshotRestoreEngine) RegisterWatchers(_ context.Context, _ func(string, string)) {}
