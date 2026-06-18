package longhorn

import (
	"context"
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/longhorn/backupstore"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllonghornv2 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

const (
	longhornDriver         = "driver.longhorn.io"
	longhornBackupScheme   = "bak://"
	backupProgressComplete = 100
	lhBackupWatcherName    = "longhorn-backup-watcher"
	vmBackupKindName       = "VirtualMachineBackup"
)

var vmBackupKind = harvesterv1.SchemeGroupVersion.WithKind(vmBackupKindName)

type LonghornEngine struct {
	vmbo               common.VMBackupOperator
	vsHelper           *common.VolumeSnapshotHelper
	pvcCache           ctlcorev1.PersistentVolumeClaimCache
	scCache            ctlstoragev1.StorageClassCache
	lhbackupCache      ctllonghornv2.BackupCache
	lhbackupController ctllonghornv2.BackupController
	vmbCache           ctlharvesterv1.VirtualMachineBackupCache
}

func GetBackupEngine(
	vmbo common.VMBackupOperator,
	vsCache ctlsnapshotv1.VolumeSnapshotCache,
	vsClient ctlsnapshotv1.VolumeSnapshotClient,
	vscCache ctlsnapshotv1.VolumeSnapshotContentCache,
	vscClient ctlsnapshotv1.VolumeSnapshotContentClient,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	scCache ctlstoragev1.StorageClassCache,
	lhbackupCache ctllonghornv2.BackupCache,
	lhbackupController ctllonghornv2.BackupController,
	vmbCache ctlharvesterv1.VirtualMachineBackupCache,
) engine.BackupEngine {
	return &LonghornEngine{
		vmbo:               vmbo,
		vsHelper:           common.NewVolumeSnapshotHelper(vsCache, vsClient, vscCache, vscClient, vmbo, pvcCache, scCache),
		pvcCache:           pvcCache,
		scCache:            scCache,
		lhbackupCache:      lhbackupCache,
		lhbackupController: lhbackupController,
		vmbCache:           vmbCache,
	}
}

// vsContentName generates a VolumeSnapshotContent name from a VolumeBackup
func (le *LonghornEngine) vscName(vb *harvesterv1.VolumeBackup) string {
	volBackupName := le.vmbo.GetVolBackupName(vb)
	if volBackupName == nil {
		return ""
	}
	return fmt.Sprintf("%s-vsc", *volBackupName)
}

// getExistingVSContent retrieves existing VolumeSnapshotContent or returns nil if not found
func (le *LonghornEngine) getExistingVSC(vscName string) (*snapshotv1.VolumeSnapshotContent, error) {
	vsc, err := le.vsHelper.GetVolumeSnapshotContent(vscName)
	if err == nil {
		return vsc, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get VolumeSnapshotContent %s: %w", vscName, err)
	}
	return nil, nil
}

// getVolumeBackupNameOrDefault returns the volume backup name or a default string for logging
func (le *LonghornEngine) getVolumeBackupNameOrDefault(vb *harvesterv1.VolumeBackup) string {
	volBackupName := le.vmbo.GetVolBackupName(vb)
	if volBackupName == nil {
		return "<nil>"
	}
	return *volBackupName
}

// buildSnapshotHandle constructs the Longhorn backup snapshot handle
// Ref: https://longhorn.io/docs/1.2.3/snapshots-and-backups/csi-snapshot-support/restore-a-backup-via-csi/#restore-a-backup-that-has-no-associated-volumesnapshot
func (le *LonghornEngine) buildSnapshotHandle(volumeName, backupName string) string {
	return fmt.Sprintf("%s%s/%s", longhornBackupScheme, volumeName, backupName)
}

// validateAndGetLonghornBackup validates the volume backup and retrieves the Longhorn backup
func (le *LonghornEngine) validateAndGetLonghornBackup(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) (string, error) {
	if le.vmbo.GetVolBackupLHBackupName(vb) == nil {
		return "", fmt.Errorf("lh backup name is nil for volume backup %s in VMBackup %s/%s",
			le.getVolumeBackupNameOrDefault(vb), le.vmbo.GetNamespace(vmb), le.vmbo.GetName(vmb))
	}

	lhBackup, err := le.lhbackupCache.Get(util.LonghornSystemNamespaceName, *le.vmbo.GetVolBackupLHBackupName(vb))
	if err != nil {
		return "", fmt.Errorf("failed to get Longhorn backup %s: %w", *le.vmbo.GetVolBackupLHBackupName(vb), err)
	}

	if lhBackup.Status.VolumeName == "" {
		return "", fmt.Errorf("lh backup %s has empty volumeName for vmbackup %s/%s volume %s",
			lhBackup.Name, le.vmbo.GetNamespace(vmb), le.vmbo.GetName(vmb), le.getVolumeBackupNameOrDefault(vb))
	}

	snapshotHandle := le.buildSnapshotHandle(lhBackup.Status.VolumeName, lhBackup.Name)
	return snapshotHandle, nil
}

// buildVolumeSnapshotContent constructs a VolumeSnapshotContent object
func (le *LonghornEngine) buildVolumeSnapshotContent(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	vsClass *snapshotv1.VolumeSnapshotClass,
	vscName string,
	snapshotHandle string,
) *snapshotv1.VolumeSnapshotContent {
	volBackupName := le.vmbo.GetVolBackupName(vb)

	return &snapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:            vscName,
			OwnerReferences: []metav1.OwnerReference{le.vsHelper.BuildOwnerReference(vmb)},
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver:         longhornDriver,
			DeletionPolicy: snapshotv1.VolumeSnapshotContentDelete,
			Source: snapshotv1.VolumeSnapshotContentSource{
				SnapshotHandle: ptr.To(snapshotHandle),
			},
			VolumeSnapshotClassName: ptr.To(vsClass.Name),
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      *volBackupName,
				Namespace: le.vmbo.GetNamespace(vmb),
			},
		},
	}
}

func (le *LonghornEngine) createVolumeSnapshotContent(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	vsClass *snapshotv1.VolumeSnapshotClass,
) (*snapshotv1.VolumeSnapshotContent, error) {
	vscName := le.vscName(vb)
	if vscName == "" {
		return nil, fmt.Errorf("%w for VMBackup %s/%s",
			common.ErrVolumeBackupNameNil, le.vmbo.GetNamespace(vmb), le.vmbo.GetName(vmb))
	}

	logrus.Debugf("attempting to create VolumeSnapshotContent %s", vscName)

	// Check if VolumeSnapshotContent already exists
	existingContent, err := le.getExistingVSC(vscName)
	if err != nil {
		return nil, err
	}
	if existingContent != nil {
		logrus.Debugf("VolumeSnapshotContent %s already exists, reusing", vscName)
		return existingContent, nil
	}

	// Validate volume backup and get snapshot handle
	snapshotHandle, err := le.validateAndGetLonghornBackup(vmb, vb)
	if err != nil {
		return nil, err
	}

	// Validate volume backup name for VolumeSnapshotRef
	vbName := le.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return nil, fmt.Errorf("%w for VMBackup %s/%s",
			common.ErrVolumeBackupNameNil, le.vmbo.GetNamespace(vmb), le.vmbo.GetName(vmb))
	}

	fields := le.getLogFields(vmb, vb)
	fields["name"] = vscName
	fields["snapshotHandle"] = snapshotHandle
	logrus.WithFields(fields).Info("creating VolumeSnapshotContent")

	// Build and create VolumeSnapshotContent
	vsc := le.buildVolumeSnapshotContent(vmb, vb, vsClass, vscName, snapshotHandle)
	return le.vsHelper.CreateVolumeSnapshotContent(vsc)
}

func (le *LonghornEngine) createVSFromLHBackup(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	vsClass *snapshotv1.VolumeSnapshotClass) (*snapshotv1.VolumeSnapshot, error) {

	vsc, err := le.createVolumeSnapshotContent(vmb, vb, vsClass)
	if err != nil {
		return nil, fmt.Errorf("failed to create VolumeSnapshotContent: %w", err)
	}

	vs := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:            *le.vmbo.GetVolBackupName(vb),
			Namespace:       le.vmbo.GetNamespace(vmb),
			OwnerReferences: []metav1.OwnerReference{le.vsHelper.BuildOwnerReference(vmb)},
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				VolumeSnapshotContentName: &vsc.Name,
			},
			VolumeSnapshotClassName: ptr.To(vsClass.Name),
		},
	}

	logrus.WithFields(le.getLogFields(vmb, vb)).Info("creating VolumeSnapshot from Longhorn backup")
	return le.vsHelper.CreateVolumeSnapshot(vs)
}

func (le *LonghornEngine) createVSFromPVC(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	vsclass *snapshotv1.VolumeSnapshotClass) (*snapshotv1.VolumeSnapshot, error) {

	return le.vsHelper.CreateVolumeSnapshotFromPVC(vmb, vb, vsclass, le.vsHelper.BuildOwnerReference(vmb))
}

func (le *LonghornEngine) getLogFields(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) logrus.Fields {
	fields := le.vsHelper.GetLogFields(vmb, vb)
	if vb != nil {
		if lhName := le.vmbo.GetVolBackupLHBackupName(vb); lhName != nil {
			fields["longhornBackup"] = *lhName
		}
	}
	return fields
}

func (le *LonghornEngine) checkLHBackup(name string) (string, error) {
	lb, err := le.lhbackupCache.Get(util.LonghornSystemNamespaceName, name)
	if err != nil {
		return "", err
	}

	if lb.Status.State != lhv1beta2.BackupStateCompleted {
		return fmt.Sprintf("backup %s is not completed", name), nil
	}

	if lb.DeletionTimestamp != nil {
		return fmt.Sprintf("backup %s is being deleted", name), nil
	}
	return "", nil
}

func (le *LonghornEngine) checkVolInBackupTarget(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, t *settings.BackupTarget) (string, error) {
	// backupstore looks up by Longhorn volume name, which is the PV name.
	pvName := le.vmbo.GetVolBackupPVName(vb)
	vols, err := backupstore.List(pvName, backuputil.ConstructEndpoint(t), false)
	if err != nil {
		// The backup target may be offline. In this case, we don't want to trigger reconciliation.
		return err.Error(), nil
	}

	handleMissing := func(msg string) (string, error) {
		logrus.WithFields(le.getLogFields(vmb, vb)).Warn(msg + ", change the VMBackup to not ready")
		if err := le.vmbo.SetVolBackupReadyToUse(vb, ptr.To(false)); err != nil {
			return "", fmt.Errorf("failed to set volume backup ready to use: %w", err)
		}
		return msg, nil
	}

	if vols[pvName] == nil {
		return handleMissing(fmt.Sprintf("cannot find volume %s in the backup target", pvName))
	}

	lhBackupName := *le.vmbo.GetVolBackupLHBackupName(vb)
	if vols[pvName].Backups[lhBackupName] == nil {
		return handleMissing(fmt.Sprintf("cannot find longhorn backup %s in the backup target", lhBackupName))
	}
	return "", nil
}

func (le *LonghornEngine) shouldSkipVSUpdate(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) (bool, error) {
	if !le.vmbo.IsTransitToNonReady(vmb) {
		return false, nil
	}
	lhBackupName := le.vmbo.GetVolBackupLHBackupName(vb)
	if lhBackupName == nil {
		return false, nil
	}

	msg, err := le.checkLHBackup(*lhBackupName)
	if apierrors.IsNotFound(err) {
		// Don't return not found error here, because it changes the VMBackup status message and there will not have "Change back to non-ready" message.
		// In the next reconcile, the shouldSkipVolumeSnapshotUpdate will return false, so the volume backup will get updated from VolumeSnapshot.
		return true, nil
	}

	if err != nil {
		logrus.WithError(err).WithFields(le.getLogFields(vmb, vb)).Warn("cannot check longhorn backup")
		return true, err
	}

	if msg != "" {
		logrus.WithFields(le.getLogFields(vmb, vb)).WithField(logrus.ErrorKey, msg).Infof("longhorn backup is not ready")
		return true, nil
	}

	tValue := settings.BackupTargetSet.Get()
	t, err := settings.DecodeBackupTarget(tValue)
	if err != nil {
		logrus.WithError(err).WithFields(le.getLogFields(vmb, vb)).Warnf("failed to decode backup target %s", tValue)
		return true, err
	}

	msg, err = le.checkVolInBackupTarget(vmb, vb, t)
	if err != nil {
		logrus.WithError(err).WithFields(le.getLogFields(vmb, vb)).Warnf("failed to check volume in backup target")
		return true, err
	}

	if msg != "" {
		logrus.WithFields(le.getLogFields(vmb, vb)).WithField(logrus.ErrorKey, msg).Infof("volume is not in backup target")
		return true, nil
	}
	return false, nil
}

func (le *LonghornEngine) Reconcile(
	vmb *harvesterv1.VirtualMachineBackup,
	volIndex int,
	vsClassMap map[string]snapshotv1.VolumeSnapshotClass,
) error {
	logrus.Debugf("LonghornEngine Reconcile called for VMBackup %s/%s volume index %d",
		le.vmbo.GetNamespace(vmb), le.vmbo.GetName(vmb), volIndex)

	vb := le.vmbo.GetVolBackup(vmb, volIndex)
	if vb == nil {
		return fmt.Errorf("volume backup at index %d not found", volIndex)
	}

	vsName := *le.vmbo.GetVolBackupName(vb)
	vs, err := le.vsHelper.GetVolumeSnapshot(le.vmbo.GetNamespace(vmb), vsName)
	if err != nil {
		return err
	}

	if err := le.vsHelper.CheckSnapshotDeletionStatus(vs, vmb, vb); err != nil {
		return err
	}

	if vs != nil {
		return le.updateVolumeBackupFromSnapshot(vmb, vb, vs)
	}

	_, err = le.ensureVolumeSnapshotExists(vmb, vb, vsClassMap)
	return err
}

// ensureVolumeSnapshotExists creates a new volume snapshot if it doesn't exist
func (le *LonghornEngine) ensureVolumeSnapshotExists(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	vsClassMap map[string]snapshotv1.VolumeSnapshotClass,
) (*snapshotv1.VolumeSnapshot, error) {
	csiDriver := le.vmbo.GetVolBackupCSIDriver(vb)
	vsClass, exists := vsClassMap[csiDriver]
	if !exists {
		return nil, fmt.Errorf("VolumeSnapshotClass not found for CSI driver %s", csiDriver)
	}

	// Recover-from-remote path: a Longhorn backup already exists, so we just
	// bind a VS/VSC to it. No live snapshot is taken, so freezing the source
	// VM's filesystem would be wasteful (and possibly incorrect — the source
	// VM may be unrelated or absent at recover time).
	if le.vmbo.GetVolBackupLHBackupName(vb) != nil {
		return le.createVSFromLHBackup(vmb, vb, &vsClass)
	}

	// Fresh-backup path: freeze the source filesystem before snapshotting for
	// crash-consistency.
	if err := le.vsHelper.TryFreezeFS(context.Background(), vmb); err != nil {
		return nil, err
	}
	return le.createVSFromPVC(vmb, vb, &vsClass)
}

// updateVolumeBackupFromSnapshot updates the volume backup status from the snapshot,
// checking if the update should be skipped first
func (le *LonghornEngine) updateVolumeBackupFromSnapshot(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	vs *snapshotv1.VolumeSnapshot,
) error {
	skip, err := le.shouldSkipVSUpdate(vmb, vb)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	return le.vsHelper.UpdateVolumeBackupStatus(vb, vs)
}

func (le *LonghornEngine) UpdateProgress(vb *harvesterv1.VolumeBackup) (int64, error) {
	if vb == nil {
		return 0, nil
	}

	if le.vmbo.GetVolBackupReadyToUse(vb) {
		return backupProgressComplete, nil
	}

	if le.vmbo.GetVolBackupLHBackupName(vb) == nil {
		return int64(le.vmbo.GetVolBackupProgress(vb)), nil
	}

	lhBackup, err := le.lhbackupCache.Get(util.LonghornSystemNamespaceName, *le.vmbo.GetVolBackupLHBackupName(vb))
	if err != nil {
		return 0, err
	}

	return int64(lhBackup.Status.Progress), nil
}

// ForceDelete removes the VolumeSnapshot and any VolumeSnapshotContent that
// references it, clearing finalizers so Kubernetes can free them immediately.
// Used when the backup target changed: the underlying Longhorn snapshot delete
// would otherwise fail and leave the VSC stuck Terminating.
//
// Important: VSCs must be cleaned even when the VS is already gone. Owner-ref
// GC can delete the VS before this fires (the VS is namespaced; the VSC is
// cluster-scoped and cannot be GC'd via owner refs). We look up VSCs by their
// VolumeSnapshotRef so cleanup works in either order.
func (le *LonghornEngine) ForceDelete(vmb *harvesterv1.VirtualMachineBackup, volIndex int) error {
	vb := le.vmbo.GetVolBackup(vmb, volIndex)
	vbName := le.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return fmt.Errorf("%w for VMBackup %s/%s at index %d",
			common.ErrVolumeBackupNameNil, le.vmbo.GetNamespace(vmb), le.vmbo.GetName(vmb), volIndex)
	}

	vsName := *vbName
	namespace := le.vmbo.GetNamespace(vmb)

	if err := le.vsHelper.ForceDeleteVS(namespace, vsName); err != nil {
		return fmt.Errorf("failed to force delete VolumeSnapshot: %w", err)
	}

	if err := le.vsHelper.ForceDeleteOrphanVSCs(namespace, vsName); err != nil {
		return fmt.Errorf("failed to force delete orphan VolumeSnapshotContents: %w", err)
	}

	return nil
}

// RegisterWatchers wires up a Longhorn-Backup → VMBackup event mapping so
// LH-backup status changes feed the VMBackup reconcile loop. Lives on the
// engine (not the controller) because the lookup chain
// LHBackup → VolumeSnapshotContent → VolumeSnapshot → VMBackup is specific
// to the Longhorn engine.
func (le *LonghornEngine) RegisterWatchers(ctx context.Context, enqueueVMBackup func(namespace, name string)) {
	le.lhbackupController.OnChange(ctx, lhBackupWatcherName, func(_ string, lhBackup *lhv1beta2.Backup) (*lhv1beta2.Backup, error) {
		return le.onLHBackupChanged(lhBackup, enqueueVMBackup)
	})
}

func (le *LonghornEngine) onLHBackupChanged(
	lhBackup *lhv1beta2.Backup,
	enqueueVMBackup func(namespace, name string),
) (*lhv1beta2.Backup, error) {
	if lhBackup == nil || lhBackup.DeletionTimestamp != nil || lhBackup.Status.SnapshotName == "" {
		return nil, nil
	}

	vmb, err := le.getVMBackupFromLHBackup(lhBackup)
	if err != nil || vmb == nil {
		return nil, err
	}

	if le.vmbo.GetBackupTarget(vmb) == nil {
		return nil, nil
	}

	vsc, err := le.vsHelper.GetVolumeSnapshotContent(backuputil.LHSnapToVSCName(lhBackup.Status.SnapshotName))
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	vs, err := le.vsHelper.GetVolumeSnapshot(vsc.Spec.VolumeSnapshotRef.Namespace, vsc.Spec.VolumeSnapshotRef.Name)
	if err != nil {
		return nil, err
	}

	updated, err := le.updateVolumeBackupLHNames(vmb, vs.Name, lhBackup.Name)
	if err != nil {
		return nil, err
	}

	// Only enqueue if we actually made changes to trigger progress update in updateConditions()
	if updated {
		enqueueVMBackup(le.vmbo.GetNamespace(vmb), le.vmbo.GetName(vmb))
	}
	return nil, nil
}

func (le *LonghornEngine) getVMBackupFromLHBackup(lhBackup *lhv1beta2.Backup) (*harvesterv1.VirtualMachineBackup, error) {
	vsc, err := le.vsHelper.GetVolumeSnapshotContent(backuputil.LHSnapToVSCName(lhBackup.Status.SnapshotName))
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	vs, err := le.vsHelper.GetVolumeSnapshot(vsc.Spec.VolumeSnapshotRef.Namespace, vsc.Spec.VolumeSnapshotRef.Name)
	if err != nil {
		return nil, err
	}

	controllerRef := metav1.GetControllerOf(vs)
	if controllerRef == nil {
		return nil, nil
	}

	return le.resolveVolSnapshotRef(vs.Namespace, controllerRef), nil
}

// resolveVolSnapshotRef returns the VMBackup referenced by a ControllerRef, or
// nil if the ControllerRef points to something else or the UID doesn't match.
func (le *LonghornEngine) resolveVolSnapshotRef(
	namespace string,
	controllerRef *metav1.OwnerReference,
) *harvesterv1.VirtualMachineBackup {
	if controllerRef.Kind != vmBackupKind.Kind {
		return nil
	}
	vmb, err := le.vmbCache.Get(namespace, controllerRef.Name)
	if err != nil {
		return nil
	}
	if le.vmbo.GetUID(vmb) != controllerRef.UID {
		return nil
	}
	return vmb
}

// updateVolumeBackupLHNames stamps the Longhorn backup name on the matching
// VolumeBackup entry. Returns whether a change was persisted so the caller
// can decide whether to enqueue.
func (le *LonghornEngine) updateVolumeBackupLHNames(vmb *harvesterv1.VirtualMachineBackup, vsName, lhBackupName string) (bool, error) {
	vmbCpy := vmb.DeepCopy()
	vbs := le.vmbo.GetVolBackups(vmbCpy)

	for index := range vbs {
		vb := le.vmbo.GetVolBackup(vmbCpy, index)
		vbName := le.vmbo.GetVolBackupName(vb)

		if vbName == nil || *vbName != vsName {
			continue
		}

		if err := le.vmbo.SetVolBackupLHBackupName(vb, lhBackupName); err != nil {
			return false, fmt.Errorf("failed to set volume backup LH backup name: %w", err)
		}

		_, err := le.vmbo.UpdateByStatus(vmb, vmbCpy)
		return err == nil, err
	}

	return false, nil
}
