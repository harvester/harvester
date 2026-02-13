package longhorn

import (
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/volumeremotebackup/common"
	"github.com/harvester/harvester/pkg/volumeremotebackup/driver"
)

type LHBackupOperation struct {
	bo           common.BackupOperator
	vsCache      ctlsnapshotv1.VolumeSnapshotCache
	vsClient     ctlsnapshotv1.VolumeSnapshotClient
	vsClassCache ctlsnapshotv1.VolumeSnapshotClassCache
	vscCache     ctlsnapshotv1.VolumeSnapshotContentCache
	pvcCache     ctlcorev1.PersistentVolumeClaimCache
	scCache      ctlstoragev1.StorageClassCache
}

func GetLHBackupOperation(
	bo common.BackupOperator,
	vsCache ctlsnapshotv1.VolumeSnapshotCache,
	vsClient ctlsnapshotv1.VolumeSnapshotClient,
	vsClassCache ctlsnapshotv1.VolumeSnapshotClassCache,
	vscCache ctlsnapshotv1.VolumeSnapshotContentCache,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	scCache ctlstoragev1.StorageClassCache,
) driver.BackupOperation {
	return &LHBackupOperation{
		bo:           bo,
		vsCache:      vsCache,
		vsClient:     vsClient,
		vsClassCache: vsClassCache,
		vscCache:     vscCache,
		pvcCache:     pvcCache,
		scCache:      scCache,
	}
}

func (lbo *LHBackupOperation) getVolumeSnapshotForRemoteBackup(vrb *harvesterv1.VolumeRemoteBackup) (*snapshotv1.VolumeSnapshot, error) {
	return lbo.vsCache.Get(lbo.bo.GetNamespace(vrb), lbo.bo.GetName(vrb))
}

// isVolumeSnapshotDeleting checks if a VolumeSnapshot is being deleted.
func (lbo *LHBackupOperation) isVolumeSnapshotDeleting(vs *snapshotv1.VolumeSnapshot) bool {
	return vs != nil && vs.DeletionTimestamp != nil
}

// checkVolumeSnapshotError checks if a VolumeSnapshot is in an error state and returns an error if so.
func (lbo *LHBackupOperation) checkVolumeSnapshotError(vs *snapshotv1.VolumeSnapshot) error {
	if vs.Status == nil || vs.Status.Error == nil {
		return nil
	}

	errorMsg := "VolumeSnapshot is in error state"
	if vs.Status.Error.Message != nil {
		errorMsg = *vs.Status.Error.Message
	}
	return fmt.Errorf("%s", errorMsg)
}

// ensureVolumeSnapshotExists creates a new volume snapshot if it doesn't exist.
func (lbo *LHBackupOperation) ensureVolumeSnapshotExists(
	vrb *harvesterv1.VolumeRemoteBackup,
	vsClass snapshotv1.VolumeSnapshotClass,
	ownerRef metav1.OwnerReference,
) (*snapshotv1.VolumeSnapshot, error) {
	pvcNamespace := lbo.bo.GetNamespace(vrb)
	pvcName := lbo.bo.GetSource(vrb)

	pvc, err := lbo.pvcCache.Get(pvcNamespace, pvcName)
	if err != nil {
		return nil, err
	}

	// Extract PVC metadata
	srcSCName := ptr.Deref(pvc.Spec.StorageClassName, "")
	srcImageID := pvc.Annotations[util.AnnotationImageID]
	srcProvisioner := util.GetProvisionedPVCProvisioner(pvc, lbo.scCache)

	// Build VolumeSnapshot
	vs := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:            lbo.bo.GetName(vrb),
			Namespace:       lbo.bo.GetNamespace(vrb),
			OwnerReferences: []metav1.OwnerReference{ownerRef},
			Annotations:     make(map[string]string),
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: &pvcName,
			},
			VolumeSnapshotClassName: ptr.To(vsClass.Name),
		},
	}

	// Add metadata annotations
	if srcSCName != "" {
		vs.Annotations[util.AnnotationStorageClassName] = srcSCName
	}
	if srcImageID != "" {
		vs.Annotations[util.AnnotationImageID] = srcImageID
	}
	if srcProvisioner != "" {
		vs.Annotations[util.AnnotationStorageProvisioner] = srcProvisioner
	}

	return lbo.vsClient.Create(vs)
}

// BuildOwnerReference creates an owner reference for a PVC backup.
func (lbo *LHBackupOperation) BuildOwnerReference(vrb *harvesterv1.VolumeRemoteBackup) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: harvesterv1.SchemeGroupVersion.String(),
		Kind:       lbo.bo.GetKind(vrb),
		Name:       lbo.bo.GetName(vrb),
		UID:        lbo.bo.GetUID(vrb),
		Controller: ptr.To(true),
	}
}

// ensureHandleIsSet retrieves the snapshot handle from VolumeSnapshotContent and updates the PVCBackup.
// Returns ErrRetryLater if the handle is not yet available.
func (lbo *LHBackupOperation) ensureHandleIsSet(vrb *harvesterv1.VolumeRemoteBackup, vs *snapshotv1.VolumeSnapshot) (bool, error) {
	if vs == nil {
		return false, fmt.Errorf("VolumeSnapshot is nil when checking handle for VolumeRemoteRestore %s/%s",
			lbo.bo.GetNamespace(vrb), lbo.bo.GetName(vrb))
	}

	// Skip if handle is already set
	if vrb.Status.Handle != "" {
		return true, nil
	}

	// Check if VolumeSnapshotContent is bound
	if vs.Status.BoundVolumeSnapshotContentName == nil {
		return false, nil
	}

	vscName := *vs.Status.BoundVolumeSnapshotContentName
	vsc, err := lbo.vscCache.Get(vscName)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// Check if SnapshotHandle is available
	if vsc.Status == nil || vsc.Status.SnapshotHandle == nil || *vsc.Status.SnapshotHandle == "" {
		return false, nil
	}

	// Update handle
	handle := *vsc.Status.SnapshotHandle
	if _, err := lbo.bo.SetHandle(vrb, handle); err != nil {
		return false, err
	}

	return true, nil
}

// readiness checks if the VolumeSnapshot is ready for the given PVCBackup.
// Returns true if ready, false with ErrRetryLater if not yet ready, or false with error if failed.
func (lbo *LHBackupOperation) readiness(vrb *harvesterv1.VolumeRemoteBackup) (bool, error) {
	// Check if VolumeSnapshot exists
	vs, err := lbo.getVolumeSnapshotForRemoteBackup(vrb)
	if err != nil {
		return false, err
	}

	// Check if VolumeSnapshot is being deleted
	if lbo.isVolumeSnapshotDeleting(vs) {
		return false, common.ErrRetryLater
	}

	// Check if VolumeSnapshot is in error state
	if err := lbo.checkVolumeSnapshotError(vs); err != nil {
		return false, err
	}

	// Check if VolumeSnapshot is ready
	if !util.IsVolumeSnapshotReady(vs) {
		return false, nil
	}

	// Ensure snapshot handle is set in PVCBackup status
	return lbo.ensureHandleIsSet(vrb, vs)
}

func (lbo *LHBackupOperation) Create(vrb *harvesterv1.VolumeRemoteBackup) error {
	if vrb == nil {
		return fmt.Errorf("RemoteBackup cannot be nil")
	}

	ready, err := lbo.readiness(vrb)
	if err == nil {
		if !ready {
			return fmt.Errorf("resources for RemoteBackup %s/%s already exist but not ready",
				lbo.bo.GetNamespace(vrb), lbo.bo.GetName(vrb))
		}
		return nil
	}

	if !apierrors.IsNotFound(err) {
		return err
	}

	// VolumeSnapshot doesn't exist, create it
	// Get VolumeSnapshotClass information
	vsClassInfo, err := lbo.bo.GetSnapshotClassInfo(vrb)
	if err != nil {
		return err
	}

	vsClass, err := lbo.vsClassCache.Get(vsClassInfo.BackupVolumeSnapshotClassName)
	if err != nil {
		return err
	}

	// Create new VolumeSnapshot
	_, err = lbo.ensureVolumeSnapshotExists(vrb, *vsClass, lbo.BuildOwnerReference(vrb))
	return err
}

func (lbo *LHBackupOperation) Readiness(vrb *harvesterv1.VolumeRemoteBackup) (bool, error) {
	if vrb == nil {
		return false, fmt.Errorf("PVCBackup cannot be nil")
	}
	return lbo.readiness(vrb)
}

func (lbo *LHBackupOperation) Delete(vrb *harvesterv1.VolumeRemoteBackup) error {
	// owner references will handle garbage collection
	return nil
}
