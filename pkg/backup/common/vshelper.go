package common

import (
	"context"
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/engine"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

// VolumeSnapshotHelper provides common operations for VolumeSnapshot management
type VolumeSnapshotHelper struct {
	vsCache   ctlsnapshotv1.VolumeSnapshotCache
	vsClient  ctlsnapshotv1.VolumeSnapshotClient
	vscCache  ctlsnapshotv1.VolumeSnapshotContentCache
	vscClient ctlsnapshotv1.VolumeSnapshotContentClient
	vmbo      VMBackupOperator
	pvcCache  ctlcorev1.PersistentVolumeClaimCache
	scCache   ctlstoragev1.StorageClassCache
}

// NewVolumeSnapshotHelper creates a new VolumeSnapshotHelper instance
func NewVolumeSnapshotHelper(
	vsCache ctlsnapshotv1.VolumeSnapshotCache,
	vsClient ctlsnapshotv1.VolumeSnapshotClient,
	vscCache ctlsnapshotv1.VolumeSnapshotContentCache,
	vscClient ctlsnapshotv1.VolumeSnapshotContentClient,
	vmbo VMBackupOperator,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	scCache ctlstoragev1.StorageClassCache,
) *VolumeSnapshotHelper {
	return &VolumeSnapshotHelper{
		vsCache:   vsCache,
		vsClient:  vsClient,
		vscCache:  vscCache,
		vscClient: vscClient,
		vmbo:      vmbo,
		pvcCache:  pvcCache,
		scCache:   scCache,
	}
}

// GetVolumeSnapshot retrieves a VolumeSnapshot by namespace and name
// Returns nil if not found, without error
func (h *VolumeSnapshotHelper) GetVolumeSnapshot(namespace, name string) (*snapshotv1.VolumeSnapshot, error) {
	vs, err := h.vsCache.Get(namespace, name)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return vs, nil
}

// CreateVolumeSnapshot creates a new VolumeSnapshot
func (h *VolumeSnapshotHelper) CreateVolumeSnapshot(vs *snapshotv1.VolumeSnapshot) (*snapshotv1.VolumeSnapshot, error) {
	return h.vsClient.Create(vs)
}

// CreateVolumeSnapshotContent creates a new VolumeSnapshotContent
func (h *VolumeSnapshotHelper) CreateVolumeSnapshotContent(vsc *snapshotv1.VolumeSnapshotContent) (*snapshotv1.VolumeSnapshotContent, error) {
	if h.vscClient == nil {
		return nil, fmt.Errorf("VolumeSnapshotContent client is not initialized")
	}
	return h.vscClient.Create(vsc)
}

// GetVolumeSnapshotContent retrieves a VolumeSnapshotContent by name
func (h *VolumeSnapshotHelper) GetVolumeSnapshotContent(name string) (*snapshotv1.VolumeSnapshotContent, error) {
	if h.vscCache == nil {
		return nil, fmt.Errorf("VolumeSnapshotContent cache is not initialized")
	}
	return h.vscCache.Get(name)
}

// GetLogFields returns structured log fields for a VM backup and optional volume backup
func (h *VolumeSnapshotHelper) GetLogFields(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) logrus.Fields {
	fields := logrus.Fields{
		"vmBackup":  h.vmbo.GetName(vmb),
		"namespace": h.vmbo.GetNamespace(vmb),
	}
	if vb != nil {
		if name := h.vmbo.GetVolBackupName(vb); name != nil {
			fields["volumeBackup"] = *name
		}
		if volName := h.vmbo.GetVolBackupVolumeName(vb); volName != "" {
			fields["volume"] = volName
		}
	}
	return fields
}

// BuildOwnerReference creates an owner reference for a VM backup
func (h *VolumeSnapshotHelper) BuildOwnerReference(vmb *harvesterv1.VirtualMachineBackup) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: harvesterv1.SchemeGroupVersion.String(),
		Kind:       h.vmbo.GetKind(vmb),
		Name:       h.vmbo.GetName(vmb),
		UID:        h.vmbo.GetUID(vmb),
		Controller: ptr.To(true),
	}
}

// CheckSnapshotDeletionStatus verifies if the snapshot is being deleted and requires requeue
func (h *VolumeSnapshotHelper) CheckSnapshotDeletionStatus(
	vs *snapshotv1.VolumeSnapshot,
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) error {
	if vs != nil && vs.DeletionTimestamp != nil {
		logrus.WithFields(h.GetLogFields(vmb, vb)).Info("VolumeSnapshot is being deleted, requeueing VMBackup")
		return engine.ErrRetryLater
	}
	return nil
}

// UpdateVolumeBackupStatus updates the volume backup status based on the snapshot status
func (h *VolumeSnapshotHelper) UpdateVolumeBackupStatus(
	vb *harvesterv1.VolumeBackup,
	vs *snapshotv1.VolumeSnapshot,
) error {
	if vs.Status == nil {
		return nil
	}

	if err := h.vmbo.SetVolBackupReadyToUse(vb, vs.Status.ReadyToUse); err != nil {
		return fmt.Errorf("failed to set volume backup ready to use: %w", err)
	}

	if err := h.vmbo.SetVolBackupCreationTime(vb, vs.Status.CreationTime); err != nil {
		return fmt.Errorf("failed to set volume backup creation time: %w", err)
	}

	if err := h.vmbo.SetVolBackupError(vb, vs.Status.Error); err != nil {
		return fmt.Errorf("failed to set volume backup error: %w", err)
	}

	return nil
}

// TryFreezeFS attempts to freeze the filesystem for a VM backup
func (h *VolumeSnapshotHelper) TryFreezeFS(ctx context.Context, vmb *harvesterv1.VirtualMachineBackup) error {
	if err := h.vmbo.TryFreezeFS(ctx, vmb); err != nil {
		logrus.WithError(err).WithFields(h.GetLogFields(vmb, nil)).Error("failed to freeze filesystem")
		return err
	}
	return nil
}

// clearFinalizersPatch is a JSON merge patch that empties metadata.finalizers
// without touching any other field. We patch instead of Update because the
// snapshot client (v4) does not know about newer spec fields like
// SourceVolumeMode — a round-trip via DeepCopy/Update would silently drop
// unknown fields and the API server would reject the change.
var clearFinalizersPatch = []byte(`{"metadata":{"finalizers":[]}}`)

// ForceDeleteVS removes finalizers and deletes a VolumeSnapshot
func (h *VolumeSnapshotHelper) ForceDeleteVS(namespace, name string) error {
	logFields := logrus.Fields{"namespace": namespace, "name": name}

	logrus.WithFields(logFields).Debug("removing finalizers from VolumeSnapshot")
	if _, err := h.vsClient.Patch(namespace, name, types.MergePatchType, clearFinalizersPatch); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to remove finalizers: %w", err)
	}

	logrus.WithFields(logFields).Debug("deleting VolumeSnapshot")
	if err := h.vsClient.Delete(namespace, name, metav1.NewDeleteOptions(0)); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete: %w", err)
	}
	return nil
}

// ForceDeleteVSC removes finalizers and deletes a VolumeSnapshotContent
func (h *VolumeSnapshotHelper) ForceDeleteVSC(name string) error {
	logrus.WithField("name", name).Debug("removing finalizers from VolumeSnapshotContent")
	if _, err := h.vscClient.Patch(name, types.MergePatchType, clearFinalizersPatch); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to remove finalizers: %w", err)
	}

	logrus.WithField("name", name).Debug("deleting VolumeSnapshotContent")
	if err := h.vscClient.Delete(name, metav1.NewDeleteOptions(0)); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete: %w", err)
	}
	return nil
}

// ForceDeleteOrphanVSCs finds every VolumeSnapshotContent whose
// VolumeSnapshotRef points at the given (namespace, name) and force-deletes
// it. Use this when the VolumeSnapshot itself may already be GC'd — VSCs are
// cluster-scoped and cannot be cleaned up via owner references.
func (h *VolumeSnapshotHelper) ForceDeleteOrphanVSCs(namespace, name string) error {
	vscs, err := h.vscCache.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list VolumeSnapshotContents: %w", err)
	}
	for _, vsc := range vscs {
		ref := vsc.Spec.VolumeSnapshotRef
		if ref.Namespace != namespace || ref.Name != name {
			continue
		}
		if err := h.ForceDeleteVSC(vsc.Name); err != nil {
			return err
		}
	}
	return nil
}

// CreateVolumeSnapshotFromPVC creates a VolumeSnapshot from a PVC with metadata annotations
func (h *VolumeSnapshotHelper) CreateVolumeSnapshotFromPVC(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	vsClass *snapshotv1.VolumeSnapshotClass,
	ownerRef metav1.OwnerReference,
) (*snapshotv1.VolumeSnapshot, error) {
	vbName := h.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return nil, ErrVolumeBackupNameNil
	}

	pvcNamespace := h.vmbo.GetVolBackupPVCNameSpace(vb)
	pvcName := h.vmbo.GetVolBackupPVCName(vb)

	pvc, err := h.pvcCache.Get(pvcNamespace, pvcName)
	if err != nil {
		return nil, fmt.Errorf("failed to get PVC %s/%s: %w", pvcNamespace, pvcName, err)
	}

	// Extract PVC metadata
	srcSCName := ptr.Deref(pvc.Spec.StorageClassName, "")
	srcImageID := pvc.Annotations[util.AnnotationImageID]
	srcProvisioner := util.GetProvisionedPVCProvisioner(pvc, h.scCache)

	// Build VolumeSnapshot
	vs := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:            *vbName,
			Namespace:       h.vmbo.GetNamespace(vmb),
			OwnerReferences: []metav1.OwnerReference{ownerRef},
			Annotations:     make(map[string]string),
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: ptr.To(pvcName),
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

	logFields := h.GetLogFields(vmb, vb)
	logFields["sourcePVC"] = pvcName
	logFields["storageClass"] = srcSCName
	logFields["provisioner"] = srcProvisioner
	logrus.WithFields(logFields).Info("creating VolumeSnapshot from PVC")

	return h.vsClient.Create(vs)
}
