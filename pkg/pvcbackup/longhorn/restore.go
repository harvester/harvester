package longhorn

import (
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/pvcbackup/common"
	"github.com/harvester/harvester/pkg/pvcbackup/driver"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
)

type LHRestoreOperation struct {
	pro          common.PVCRestoreOperator
	pbo          common.PVCBackupOperator
	pbCache      ctlharvesterv1.PVCBackupCache
	vsCache      ctlsnapshotv1.VolumeSnapshotCache
	vsClient     ctlsnapshotv1.VolumeSnapshotClient
	vsClassCache ctlsnapshotv1.VolumeSnapshotClassCache
	vscCache     ctlsnapshotv1.VolumeSnapshotContentCache
	vscClient    ctlsnapshotv1.VolumeSnapshotContentClient
	pvcCache     ctlcorev1.PersistentVolumeClaimCache
	pvcClient    ctlcorev1.PersistentVolumeClaimClient
	scCache      ctlstoragev1.StorageClassCache
}

func GetLHRestoreOperation(
	pro common.PVCRestoreOperator,
	pbo common.PVCBackupOperator,
	pbCache ctlharvesterv1.PVCBackupCache,
	vsCache ctlsnapshotv1.VolumeSnapshotCache,
	vsClient ctlsnapshotv1.VolumeSnapshotClient,
	vsClassCache ctlsnapshotv1.VolumeSnapshotClassCache,
	vscCache ctlsnapshotv1.VolumeSnapshotContentCache,
	vscClient ctlsnapshotv1.VolumeSnapshotContentClient,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	pvcClient ctlcorev1.PersistentVolumeClaimClient,
	scCache ctlstoragev1.StorageClassCache,
) driver.RestoreOperation {
	return &LHRestoreOperation{
		pro:          pro,
		pbo:          pbo,
		pbCache:      pbCache,
		vsCache:      vsCache,
		vsClient:     vsClient,
		vsClassCache: vsClassCache,
		vscCache:     vscCache,
		vscClient:    vscClient,
		pvcCache:     pvcCache,
		pvcClient:    pvcClient,
		scCache:      scCache,
	}
}

// BuildOwnerReference creates an owner reference for a PVCRestore.
func (lbr *LHRestoreOperation) BuildOwnerReference(pr *harvesterv1.PVCRestore) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: harvesterv1.SchemeGroupVersion.String(),
		Kind:       lbr.pro.GetKind(pr),
		Name:       lbr.pro.GetName(pr),
		UID:        lbr.pro.GetUID(pr),
		Controller: ptr.To(true),
	}
}

func (lbr *LHRestoreOperation) Create(pr *harvesterv1.PVCRestore) error {
	if pr == nil {
		return fmt.Errorf("PVCRestore cannot be nil")
	}

	ready, err := lbr.readiness(pr)
	if err == nil {
		if !ready {
			return fmt.Errorf("resources for PVCRestore %s/%s already exist but not ready",
				lbr.pro.GetNamespace(pr), lbr.pro.GetName(pr))
		}
		return nil
	}

	// If error is not NotFound, return it
	if !apierrors.IsNotFound(err) {
		return err
	}

	// Resources don't exist, create them
	// Get the source PVCBackup
	pb, err := lbr.getSourcePVCBackup(pr)
	if err != nil {
		return err
	}

	// Get VolumeSnapshotClass information
	vsClassInfo, err := lbr.pro.GetVSClassInfo(pr)
	if err != nil {
		return err
	}

	vsClass, err := lbr.vsClassCache.Get(vsClassInfo.VolumeSnapshotClassName)
	if err != nil {
		return err
	}

	// Create VolumeSnapshotContent, VolumeSnapshot, and PVC
	return lbr.ensureRestoreResourcesExist(pr, pb, vsClass)
}

// createOrGet is a helper function that attempts to create a resource, and if it already exists,
// retrieves it from the cache instead. This reduces repetitive error handling code.
func createOrGet[T any](
	createFn func() (T, error),
	getFn func() (T, error),
) (T, error) {
	resource, err := createFn()
	if err == nil {
		return resource, nil
	}

	if apierrors.IsAlreadyExists(err) {
		return getFn()
	}

	var zero T
	return zero, err
}

// buildRestoreSteps creates an array of functions that define the steps to create restore resources.
// Each step returns an error if the operation fails.
func (lbr *LHRestoreOperation) buildRestoreSteps(
	pr *harvesterv1.PVCRestore,
	pb *harvesterv1.PVCBackup,
	vsClass *snapshotv1.VolumeSnapshotClass,
) []func() error {
	var vsc *snapshotv1.VolumeSnapshotContent
	var vs *snapshotv1.VolumeSnapshot

	return []func() error{
		// Step 1: Create VolumeSnapshotContent
		func() error {
			var err error
			vsc, err = createOrGet(
				func() (*snapshotv1.VolumeSnapshotContent, error) {
					return lbr.createVSC(pr, pb, vsClass)
				},
				func() (*snapshotv1.VolumeSnapshotContent, error) {
					return lbr.vscCache.Get(lbr.pro.GetName(pr))
				},
			)
			return err
		},
		// Step 2: Create VolumeSnapshot
		func() error {
			if vsc == nil {
				return fmt.Errorf("VolumeSnapshotContent not available for creating VolumeSnapshot")
			}
			var err error
			vs, err = createOrGet(
				func() (*snapshotv1.VolumeSnapshot, error) {
					return lbr.createVolumeSnapshot(pr, vsc, vsClass)
				},
				func() (*snapshotv1.VolumeSnapshot, error) {
					return lbr.vsCache.Get(lbr.pro.GetNamespace(pr), lbr.pro.GetName(pr))
				},
			)
			return err
		},
		// Step 3: Create PVC from VolumeSnapshot
		func() error {
			if vs == nil {
				return fmt.Errorf("VolumeSnapshot not available for creating PVC")
			}
			_, err := createOrGet(
				func() (*corev1.PersistentVolumeClaim, error) {
					return lbr.createPVCFromSnapshot(pr, pb, vs)
				},
				func() (*corev1.PersistentVolumeClaim, error) {
					return lbr.pvcCache.Get(lbr.pro.GetNamespace(pr), lbr.pro.GetName(pr))
				},
			)
			return err
		},
	}
}

// ensureRestoreResourcesExist creates all necessary resources for a restore operation
func (lbr *LHRestoreOperation) ensureRestoreResourcesExist(
	pr *harvesterv1.PVCRestore,
	pb *harvesterv1.PVCBackup,
	vsClass *snapshotv1.VolumeSnapshotClass,
) error {
	// Build creation steps
	steps := lbr.buildRestoreSteps(pr, pb, vsClass)

	// Execute all steps sequentially
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}

	return nil
}

// getSourcePVCBackup retrieves the source PVCBackup referenced by the PVCRestore
func (lbr *LHRestoreOperation) getSourcePVCBackup(pr *harvesterv1.PVCRestore) (*harvesterv1.PVCBackup, error) {
	fromRef := lbr.pro.GetFrom(pr)
	namespace, name := ref.Parse(fromRef)

	// If namespace is empty, use the same namespace as PVCRestore
	if namespace == "" {
		namespace = lbr.pro.GetNamespace(pr)
	}

	pb, err := lbr.pbCache.Get(namespace, name)
	if err != nil {
		return nil, err
	}

	// Validate that the backup has a handle
	if lbr.pbo.GetHandle(pb) == "" {
		return nil, fmt.Errorf("source PVCBackup %s/%s has no handle", namespace, name)
	}

	return pb, nil
}

// createVSC creates a VolumeSnapshotContent from the backup handle
func (lbr *LHRestoreOperation) createVSC(
	pr *harvesterv1.PVCRestore,
	pb *harvesterv1.PVCBackup,
	vsClass *snapshotv1.VolumeSnapshotClass,
) (*snapshotv1.VolumeSnapshotContent, error) {
	vscName := lbr.pro.GetName(pr)
	vsName := lbr.pro.GetName(pr)
	vsNamespace := lbr.pro.GetNamespace(pr)
	handle := lbr.pbo.GetHandle(pb)

	vsc := &snapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: vscName,
			Annotations: map[string]string{
				driver.AnnotationPVCRestoreRef: fmt.Sprintf("%s/%s", vsNamespace, pr.Name),
			},
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			DeletionPolicy: snapshotv1.VolumeSnapshotContentDelete,
			Driver:         vsClass.Driver,
			Source: snapshotv1.VolumeSnapshotContentSource{
				SnapshotHandle: ptr.To(handle),
			},
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      vsName,
				Namespace: vsNamespace,
			},
			VolumeSnapshotClassName: ptr.To(vsClass.Name),
		},
	}

	return lbr.vscClient.Create(vsc)
}

// createVolumeSnapshot creates a VolumeSnapshot pointing to the VolumeSnapshotContent
func (lbr *LHRestoreOperation) createVolumeSnapshot(
	pr *harvesterv1.PVCRestore,
	vsc *snapshotv1.VolumeSnapshotContent,
	vsClass *snapshotv1.VolumeSnapshotClass,
) (*snapshotv1.VolumeSnapshot, error) {
	vsName := lbr.pro.GetName(pr)
	vsNamespace := lbr.pro.GetNamespace(pr)
	vscName := vsc.Name

	vs := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vsName,
			Namespace: vsNamespace,
			Annotations: map[string]string{
				driver.AnnotationPVCRestoreRef: fmt.Sprintf("%s/%s", vsNamespace, pr.Name),
			},
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				VolumeSnapshotContentName: ptr.To(vscName),
			},
			VolumeSnapshotClassName: ptr.To(vsClass.Name),
		},
	}

	return lbr.vsClient.Create(vs)
}

// createPVCFromSnapshot creates a new PVC from the VolumeSnapshot
func (lbr *LHRestoreOperation) createPVCFromSnapshot(
	pr *harvesterv1.PVCRestore,
	pb *harvesterv1.PVCBackup,
	vs *snapshotv1.VolumeSnapshot,
) (*corev1.PersistentVolumeClaim, error) {
	pvcName := lbr.pro.GetName(pr)
	pvcNamespace := lbr.pro.GetNamespace(pr)
	ownerRef := lbr.BuildOwnerReference(pr)

	// Get source spec from pb operator
	sourceSpec := lbr.pbo.GetSourceSpec(pb)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            pvcName,
			Namespace:       pvcNamespace,
			Annotations:     map[string]string{},
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      sourceSpec.AccessModes,
			Resources:        sourceSpec.Resources,
			StorageClassName: sourceSpec.StorageClassName,
			VolumeMode:       sourceSpec.VolumeMode,
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(snapshotv1.GroupName),
				Kind:     "VolumeSnapshot",
				Name:     vs.Name,
			},
		},
	}

	return lbr.pvcClient.Create(pvc)
}

// getVSC retrieves the VolumeSnapshotContent associated with a PVCRestore.
func (lbr *LHRestoreOperation) getVSCForRestore(pr *harvesterv1.PVCRestore) (*snapshotv1.VolumeSnapshotContent, error) {
	return lbr.vscCache.Get(lbr.pro.GetName(pr))
}

// getVolumeSnapshotForPVCRestore retrieves the VolumeSnapshot associated with a PVCRestore.
func (lbr *LHRestoreOperation) getVolumeSnapshotForPVCRestore(pr *harvesterv1.PVCRestore) (*snapshotv1.VolumeSnapshot, error) {
	return lbr.vsCache.Get(lbr.pro.GetNamespace(pr), lbr.pro.GetName(pr))
}

// getPVCForPVCRestore retrieves the PVC associated with a PVCRestore.
func (lbr *LHRestoreOperation) getPVCForPVCRestore(pr *harvesterv1.PVCRestore) (*corev1.PersistentVolumeClaim, error) {
	return lbr.pvcCache.Get(lbr.pro.GetNamespace(pr), lbr.pro.GetName(pr))
}

// isVSCDeleting checks if a VolumeSnapshotContent is being deleted.
func (lbr *LHRestoreOperation) isVSCDeleting(vsc *snapshotv1.VolumeSnapshotContent) bool {
	return vsc != nil && vsc.DeletionTimestamp != nil
}

// isVolumeSnapshotDeleting checks if a VolumeSnapshot is being deleted.
func (lbr *LHRestoreOperation) isVolumeSnapshotDeleting(vs *snapshotv1.VolumeSnapshot) bool {
	return vs != nil && vs.DeletionTimestamp != nil
}

// isPVCDeleting checks if a PVC is being deleted.
func (lbr *LHRestoreOperation) isPVCDeleting(pvc *corev1.PersistentVolumeClaim) bool {
	return pvc != nil && pvc.DeletionTimestamp != nil
}

// checkVSCError checks if a VolumeSnapshotContent is in an error state.
func (lbr *LHRestoreOperation) checkVSCError(vsc *snapshotv1.VolumeSnapshotContent) error {
	if vsc.Status == nil || vsc.Status.Error == nil {
		return nil
	}

	errorMsg := "VolumeSnapshotContent is in error state"
	if vsc.Status.Error.Message != nil {
		errorMsg = *vsc.Status.Error.Message
	}
	return fmt.Errorf("%s", errorMsg)
}

// checkVolumeSnapshotError checks if a VolumeSnapshot is in an error state.
func (lbr *LHRestoreOperation) checkVolumeSnapshotError(vs *snapshotv1.VolumeSnapshot) error {
	if vs.Status == nil || vs.Status.Error == nil {
		return nil
	}

	errorMsg := "VolumeSnapshot is in error state"
	if vs.Status.Error.Message != nil {
		errorMsg = *vs.Status.Error.Message
	}
	return fmt.Errorf("%s", errorMsg)
}

// checkVSCReady checks if the VolumeSnapshotContent is ready.
// Returns an error if the VSC is in an error state or being deleted.
// Returns nil if the VSC is ready, or nil if it's not yet ready (caller should retry).
func (lbr *LHRestoreOperation) checkVSCReadiness(pr *harvesterv1.PVCRestore) (*snapshotv1.VolumeSnapshotContent, error) {
	vsc, err := lbr.getVSCForRestore(pr)
	if err != nil {
		return nil, err
	}

	if lbr.isVSCDeleting(vsc) {
		return nil, common.ErrRetryLater
	}

	if err := lbr.checkVSCError(vsc); err != nil {
		return nil, err
	}

	if vsc.Status == nil || vsc.Status.ReadyToUse == nil || !*vsc.Status.ReadyToUse {
		return nil, nil
	}

	return vsc, nil
}

// checkVolumeSnapshotReadiness checks if the VolumeSnapshot is ready.
// Returns an error if the VS is in an error state or being deleted.
// Returns nil if the VS is ready, or nil if it's not yet ready (caller should retry).
func (lbr *LHRestoreOperation) checkVolumeSnapshotReadiness(pr *harvesterv1.PVCRestore) (*snapshotv1.VolumeSnapshot, error) {
	vs, err := lbr.getVolumeSnapshotForPVCRestore(pr)
	if err != nil {
		return nil, err
	}

	if lbr.isVolumeSnapshotDeleting(vs) {
		return nil, common.ErrRetryLater
	}

	if err := lbr.checkVolumeSnapshotError(vs); err != nil {
		return nil, err
	}

	if !util.IsVolumeSnapshotReady(vs) {
		return nil, nil
	}

	return vs, nil
}

// checkPVCReadiness checks if the PVC is ready.
// Returns true if ready, false if not yet ready, or error if failed.
// For WaitForFirstConsumer binding mode, considers the PVC ready once created.
// For Immediate binding mode, waits for the PVC to be bound.
func (lbr *LHRestoreOperation) checkPVCReadiness(pr *harvesterv1.PVCRestore) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := lbr.getPVCForPVCRestore(pr)
	if err != nil {
		return nil, err
	}

	if lbr.isPVCDeleting(pvc) {
		return nil, common.ErrRetryLater
	}

	// Check the StorageClass's volume binding mode
	if pvc.Spec.StorageClassName != nil {
		sc, err := lbr.scCache.Get(*pvc.Spec.StorageClassName)
		if err != nil {
			return nil, err
		}

		if sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
			// For WaitForFirstConsumer, PVC creation is sufficient
			return pvc, nil
		}
	}

	// For Immediate binding mode (or when not specified), check if PVC is bound
	if pvc.Status.Phase != corev1.ClaimBound {
		return nil, nil
	}

	return pvc, nil
}

// readiness checks if all resources for the PVCRestore are ready.
// Returns true if ready, false if not yet ready, or false with error if failed.
func (lbr *LHRestoreOperation) readiness(pr *harvesterv1.PVCRestore) (bool, error) {
	// Step 1: Check VolumeSnapshotContent
	vsc, err := lbr.checkVSCReadiness(pr)
	if err != nil {
		return false, err
	}
	if vsc == nil {
		return false, nil
	}

	// Step 2: Check VolumeSnapshot
	vs, err := lbr.checkVolumeSnapshotReadiness(pr)
	if err != nil {
		return false, err
	}
	if vs == nil {
		return false, nil
	}

	// Step 3: Check PVC
	pvc, err := lbr.checkPVCReadiness(pr)
	if err != nil {
		return false, err
	}
	if pvc == nil {
		return false, nil
	}

	return true, nil
}

func (lbr *LHRestoreOperation) Readiness(pr *harvesterv1.PVCRestore) (bool, error) {
	if pr == nil {
		return false, fmt.Errorf("PVCRestore cannot be nil")
	}
	return lbr.readiness(pr)
}

// deleteVolumeSnapshot deletes the VolumeSnapshot associated with a PVCRestore
func (lbr *LHRestoreOperation) deleteVolumeSnapshot(pr *harvesterv1.PVCRestore) error {
	vs, err := lbr.getVolumeSnapshotForPVCRestore(pr)
	if apierrors.IsNotFound(err) {
		// VolumeSnapshot already deleted or doesn't exist
		return nil
	}
	if err != nil {
		return err
	}

	if err := lbr.vsClient.Delete(vs.Namespace, vs.Name, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

// hasSnapshotHandle checks if a VolumeSnapshotContent has a snapshot handle.
func (lbr *LHRestoreOperation) hasSnapshotHandle(vsc *snapshotv1.VolumeSnapshotContent) bool {
	return vsc.Status != nil && vsc.Status.SnapshotHandle != nil && *vsc.Status.SnapshotHandle != ""
}

func (lbr *LHRestoreOperation) Delete(pr *harvesterv1.PVCRestore) error {
	// Check if the related VolumeSnapshotContent exists
	vsc, err := lbr.getVSCForRestore(pr)
	if apierrors.IsNotFound(err) {
		// VSC doesn't exist, check and delete VolumeSnapshot
		return lbr.deleteVolumeSnapshot(pr)
	}
	if err != nil {
		return err
	}

	// SnapshotHandle doesn't exist, delete the related VolumeSnapshot
	if !lbr.hasSnapshotHandle(vsc) {
		return lbr.deleteVolumeSnapshot(pr)
	}

	// Clear the SnapshotHandle from the VSC status to preserve the source backup.
	// When a VolumeSnapshotContent with a SnapshotHandle is deleted, Longhorn's CSI driver
	// will delete the underlying backup. Since deleting a PVCRestore should not affect
	// the source PVCBackup, we must clear the SnapshotHandle before allowing deletion.
	//
	// References:
	// - https://github.com/longhorn/longhorn-manager/blob/v1.10.1/csi/controller_server.go#L1186
	// - https://github.com/longhorn/longhorn-manager/blob/v1.10.1/csi/controller_server.go#L1204-L1211
	vscCopy := vsc.DeepCopy()
	if vscCopy.Status == nil {
		vscCopy.Status = &snapshotv1.VolumeSnapshotContentStatus{}
	}
	vscCopy.Status.SnapshotHandle = nil

	if _, err := lbr.vscClient.UpdateStatus(vscCopy); err != nil {
		return err
	}
	return common.ErrRetryLater
}
