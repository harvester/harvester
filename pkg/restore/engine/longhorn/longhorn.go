package longhorn

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	lhdatastore "github.com/longhorn/longhorn-manager/datastore"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
	lhutil "github.com/longhorn/longhorn-manager/util"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/rancher/wrangler/v3/pkg/name"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllonghornv2 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/restore/engine"
	"github.com/harvester/harvester/pkg/restore/pvchelper"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	longhornDriver           = "driver.longhorn.io"
	longhornBackupScheme     = "bak://"
	pvNamePrefix             = "pvc"
	volumeNameUUIDNoTruncate = -1
	snapRevise               = "-snap-revise"

	// CSI snapshot types from Longhorn
	csiSnapshotTypeLonghornBackingImage     = "bi"
	csiSnapshotTypeLonghornBackup           = "bak"
	deprecatedCSISnapshotTypeLonghornBackup = "bs"
)

type LonghornRestoreEngine struct {
	vmbo                common.VMBackupOperator
	vmro                restorecommon.VMRestoreOperator
	pvcCache            ctlcorev1.PersistentVolumeClaimCache
	pvcClient           ctlcorev1.PersistentVolumeClaimClient
	scCache             ctlstoragev1.StorageClassCache
	vsCache             ctlsnapshotv1.VolumeSnapshotCache
	vsClient            ctlsnapshotv1.VolumeSnapshotClient
	vscCache            ctlsnapshotv1.VolumeSnapshotContentCache
	vscClient           ctlsnapshotv1.VolumeSnapshotContentClient
	lhBackupCache       ctllonghornv2.BackupCache
	lhBackupVolumeCache ctllonghornv2.BackupVolumeCache
	lhEngineCache       ctllonghornv2.EngineCache
	lhEngineController  ctllonghornv2.EngineController
	volumeCache         ctllonghornv2.VolumeCache
	vmBackupController  ctlharvesterv1.VirtualMachineBackupController
	vmBackupCache       ctlharvesterv1.VirtualMachineBackupCache
	vmrCache            ctlharvesterv1.VirtualMachineRestoreCache
}

func GetRestoreEngine(
	vmbo common.VMBackupOperator,
	vmro restorecommon.VMRestoreOperator,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	pvcClient ctlcorev1.PersistentVolumeClaimClient,
	scCache ctlstoragev1.StorageClassCache,
	vsCache ctlsnapshotv1.VolumeSnapshotCache,
	vsClient ctlsnapshotv1.VolumeSnapshotClient,
	vscCache ctlsnapshotv1.VolumeSnapshotContentCache,
	vscClient ctlsnapshotv1.VolumeSnapshotContentClient,
	lhBackupCache ctllonghornv2.BackupCache,
	lhBackupVolumeCache ctllonghornv2.BackupVolumeCache,
	lhEngineCache ctllonghornv2.EngineCache,
	lhEngineController ctllonghornv2.EngineController,
	volumeCache ctllonghornv2.VolumeCache,
	vmBackupController ctlharvesterv1.VirtualMachineBackupController,
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache,
	vmrCache ctlharvesterv1.VirtualMachineRestoreCache,
) engine.RestoreEngine {
	return &LonghornRestoreEngine{
		vmbo:                vmbo,
		vmro:                vmro,
		pvcCache:            pvcCache,
		pvcClient:           pvcClient,
		scCache:             scCache,
		vsCache:             vsCache,
		vsClient:            vsClient,
		vscCache:            vscCache,
		vscClient:           vscClient,
		lhBackupCache:       lhBackupCache,
		lhBackupVolumeCache: lhBackupVolumeCache,
		lhEngineCache:       lhEngineCache,
		lhEngineController:  lhEngineController,
		volumeCache:         volumeCache,
		vmBackupController:  vmBackupController,
		vmBackupCache:       vmBackupCache,
		vmrCache:            vmrCache,
	}
}

func (lre *LonghornRestoreEngine) Reconcile(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	volIndex int,
) error {
	vr := lre.vmro.GetVolRestore(vmr, volIndex)
	if vr == nil {
		return fmt.Errorf("volume restore at index %d not found", volIndex)
	}

	pvcName := lre.vmro.GetVolRestorePVCName(vr)
	namespace := lre.vmro.GetNamespace(vmr)

	// Check if PVC already exists
	pvc, err := lre.pvcCache.Get(namespace, pvcName)
	if apierrors.IsNotFound(err) {
		vb := lre.vmbo.GetVolBackup(vmb, volIndex)
		return lre.createPVCFromSnapshot(vmr, vmb, vr, vb)
	}

	if err != nil {
		return fmt.Errorf("failed to get PVC %s/%s: %w", namespace, pvcName, err)
	}

	// PVC exists, check its status
	return lre.checkPVCStatus(pvc)
}

func (lre *LonghornRestoreEngine) createPVCFromSnapshot(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) error {
	// Get or create VolumeSnapshot
	vsName, err := lre.getOrCreateVolumeSnapshot(vmr, vmb, vb)
	if err != nil {
		return err
	}

	// Create PVC
	return lre.createPVC(vmr, vr, vb, vsName)
}

func (lre *LonghornRestoreEngine) getOrCreateVolumeSnapshot(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) (string, error) {
	namespace := lre.vmro.GetNamespace(vmr)
	backupNamespace := lre.vmbo.GetNamespace(vmb)

	// If same namespace, validate and use existing snapshot from backup
	if namespace == backupNamespace {
		return lre.validateAndGetDataSourceSameNs(vmb, vb)
	}

	// Different namespace, create new VolumeSnapshot and VolumeSnapshotContent
	return lre.createVolumeSnapshotForRestore(vmr, vb)
}

// validateAndGetDataSourceSameNs validates VolumeSnapshotContent and returns the VolumeSnapshot name
// This is the mechanism from the original restore controller
func (lre *LonghornRestoreEngine) validateAndGetDataSourceSameNs(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) (string, error) {
	volBackupName := lre.vmbo.GetVolBackupName(vb)
	if volBackupName == nil {
		return "", common.ErrVolumeBackupNameNil
	}

	// Get the VolumeSnapshot
	pvcNamespace := lre.vmbo.GetVolBackupPVCNameSpace(vb)
	vs, err := lre.vsCache.Get(pvcNamespace, *volBackupName)
	if err != nil {
		return "", err
	}

	// We don't have to check VolumeSnapshotContent if the VolumeSnapshot is from PVC
	if vs.Spec.Source.PersistentVolumeClaimName != nil {
		return *volBackupName, nil
	}

	// Validate VolumeSnapshotContent
	validationErr := lre.validateVolumeSnapshotContent(vb)
	if validationErr == nil {
		return *volBackupName, nil
	}

	// Validation failed, trigger revision
	if revErr := lre.reviseVolumeSnapshot(vmb, vb); revErr != nil {
		return "", revErr
	}
	return "", validationErr
}

// validateVolumeSnapshotContent validates that the VolumeSnapshotContent references a valid Longhorn backup
func (lre *LonghornRestoreEngine) validateVolumeSnapshotContent(vb *harvesterv1.VolumeBackup) error {
	volBackupName := lre.vmbo.GetVolBackupName(vb)
	if volBackupName == nil {
		return common.ErrVolumeBackupNameNil
	}

	vscName := lre.getVolumeSnapshotContentName(vb)
	vsc, err := lre.vscCache.Get(vscName)
	if err != nil {
		return err
	}

	if vsc.Spec.Source.SnapshotHandle == nil {
		return fmt.Errorf("vsc %s missing SnapshotHandle", vsc.Name)
	}

	// Decode and validate the snapshot ID
	_, vol, backup := decodeSnapshotID(*vsc.Spec.Source.SnapshotHandle)

	// Check if the Longhorn backup exists
	if err := lre.validateLonghornBackup(backup); err != nil {
		return err
	}

	// Check if the backup volume exists
	if err := lre.validateBackupVolume(vol, vsc); err != nil {
		return fmt.Errorf("volume backup %s needs to update VolumeSnapshot", *volBackupName)
	}

	return nil
}

// validateLonghornBackup checks if the Longhorn backup exists
func (lre *LonghornRestoreEngine) validateLonghornBackup(backupName string) error {
	_, err := lre.lhBackupCache.Get(util.LonghornSystemNamespaceName, backupName)
	return err
}

// validateBackupVolume checks if the backup volume exists and is unique
func (lre *LonghornRestoreEngine) validateBackupVolume(volumeName string, vsc *snapshotv1.VolumeSnapshotContent) error {
	sets := labels.Set{
		types.LonghornLabelBackupVolume: volumeName,
	}

	bvs, err := lre.lhBackupVolumeCache.List(util.LonghornSystemNamespaceName, sets.AsSelector())
	if err != nil {
		return err
	}

	if len(bvs) > 1 {
		return fmt.Errorf("found more than 1 backupvolume for volume %s", volumeName)
	}

	if len(bvs) == 1 {
		logrus.WithFields(logrus.Fields{
			"name":           vsc.Name,
			"snapshotHandle": *vsc.Spec.Source.SnapshotHandle,
		}).Info("VolumeSnapshotContent get correct snapshotHandle")
		return nil
	}

	// No backup volume found
	return fmt.Errorf("backup volume not found for volume %s", volumeName)
}

// reviseVolumeSnapshot updates VolumeBackup's name with snap-revise suffix
// This will trigger VMBackup controller to create new VolumeSnapshot/VolumeSnapshotContent
func (lre *LonghornRestoreEngine) reviseVolumeSnapshot(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) error {
	currentVMb, err := lre.vmBackupCache.Get(lre.vmbo.GetNamespace(vmb), lre.vmbo.GetName(vmb))
	if err != nil {
		return err
	}

	currentVMbCpy := currentVMb.DeepCopy()

	// Use vmbo to set ReadyToUse to false
	if err := lre.vmbo.SetReadyToUse(currentVMbCpy, false); err != nil {
		return err
	}

	if err := lre.updateVolumeBackup(currentVMbCpy, vb); err != nil {
		return err
	}

	if err := lre.vmbo.SetAnnotation(currentVMbCpy, util.AnnotationSnapshotRevise, strconv.FormatBool(true)); err != nil {
		return err
	}

	_, err = lre.vmbo.Update(currentVMb, currentVMbCpy)
	if err != nil {
		return err
	}

	// Make VMBackup CR be reconciled and re-created the VolumeSnapshotContent with correct content
	lre.vmBackupController.Enqueue(lre.vmbo.GetNamespace(currentVMb), lre.vmbo.GetName(currentVMb))
	return nil
}

// updateVolumeBackup updates the volume backup name with snap-revise suffix
func (lre *LonghornRestoreEngine) updateVolumeBackup(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) error {
	volName := lre.vmbo.GetVolBackupVolumeName(vb)
	volBackupName := lre.vmbo.GetVolBackupName(vb)

	if volBackupName == nil {
		return common.ErrVolumeBackupNameNil
	}

	newName := *volBackupName + snapRevise

	volBackups := lre.vmbo.GetVolBackups(vmb)
	for i := range volBackups {
		// Use vmbo to get the volume name for comparison
		currentVb := lre.vmbo.GetVolBackup(vmb, i)
		if currentVb == nil {
			continue
		}

		currentVolName := lre.vmbo.GetVolBackupVolumeName(currentVb)
		if currentVolName != volName {
			continue
		}

		if err := lre.vmbo.SetVolBackupName(currentVb, newName); err != nil {
			return err
		}

		logrus.WithFields(logrus.Fields{
			"VMBackupNamespace": lre.vmbo.GetNamespace(vmb),
			"VMBackupName":      lre.vmbo.GetName(vmb),
			"VolumeBackupName":  *volBackupName,
		}).Info("updating volume backup name")
		return nil
	}

	return fmt.Errorf("vmbackup %s/%s volumebackup %s has no matched entry %v",
		lre.vmbo.GetNamespace(vmb), lre.vmbo.GetName(vmb), *volBackupName, volName)
}

func (lre *LonghornRestoreEngine) createVolumeSnapshotForRestore(
	vmr *harvesterv1.VirtualMachineRestore,
	vb *harvesterv1.VolumeBackup,
) (string, error) {
	vsName := lre.constructVolumeSnapshotName(vmr, vb)

	// Check if VolumeSnapshot already exists
	vs, err := lre.vsCache.Get(lre.vmro.GetNamespace(vmr), vsName)
	if err == nil {
		return vs.Name, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", err
	}

	// Create VolumeSnapshotContent first
	vscName := lre.constructVolumeSnapshotContentName(vmr, vb)

	vsc, err := lre.createVolumeSnapshotContent(vmr, vb, vscName, vsName)
	if err != nil {
		return "", err
	}

	// Create VolumeSnapshot
	return lre.createVolumeSnapshot(vmr, vsName, vsc)
}

func (lre *LonghornRestoreEngine) createVolumeSnapshotContent(
	vmr *harvesterv1.VirtualMachineRestore,
	vb *harvesterv1.VolumeBackup,
	vscName, vsName string,
) (*snapshotv1.VolumeSnapshotContent, error) {
	// Get Longhorn backup
	lhBackupName := lre.vmbo.GetVolBackupLHBackupName(vb)
	if lhBackupName == nil {
		return nil, fmt.Errorf("longhorn backup name is nil for volume backup")
	}

	lhBackup, err := lre.lhBackupCache.Get(util.LonghornSystemNamespaceName, *lhBackupName)
	if err != nil {
		return nil, fmt.Errorf("failed to get Longhorn backup %s: %w", *lhBackupName, err)
	}

	if lhBackup.Status.VolumeName == "" {
		return nil, fmt.Errorf("the Longhorn backup %s has empty volume name", lhBackup.Name)
	}

	snapshotHandle := fmt.Sprintf("%s%s/%s", longhornBackupScheme, lhBackup.Status.VolumeName, lhBackup.Name)

	vsc := &snapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: vscName,
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver:         longhornDriver,
			DeletionPolicy: snapshotv1.VolumeSnapshotContentRetain,
			Source: snapshotv1.VolumeSnapshotContentSource{
				SnapshotHandle: ptr.To(snapshotHandle),
			},
			VolumeSnapshotClassName: ptr.To(settings.VolumeSnapshotClass.Get()),
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      vsName,
				Namespace: lre.vmro.GetNamespace(vmr),
			},
		},
	}

	return lre.vscClient.Create(vsc)
}

func (lre *LonghornRestoreEngine) createVolumeSnapshot(
	vmr *harvesterv1.VirtualMachineRestore,
	vsName string,
	vsc *snapshotv1.VolumeSnapshotContent,
) (string, error) {
	vs := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:            vsName,
			Namespace:       lre.vmro.GetNamespace(vmr),
			OwnerReferences: []metav1.OwnerReference{lre.vmro.BuildOwnerReference(vmr)},
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				VolumeSnapshotContentName: ptr.To(vsc.Name),
			},
			VolumeSnapshotClassName: ptr.To(settings.VolumeSnapshotClass.Get()),
		},
	}

	created, err := lre.vsClient.Create(vs)
	if err != nil {
		return "", err
	}

	return created.Name, nil
}

func (lre *LonghornRestoreEngine) createPVC(
	vmr *harvesterv1.VirtualMachineRestore,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
	vsName string,
) error {
	pvcName := lre.vmro.GetVolRestorePVCName(vr)
	namespace := lre.vmro.GetNamespace(vmr)
	pvcSpec := lre.vmbo.GetVolBackupPVCSpec(vb)
	labels := pvchelper.BuildRestoreLabels(lre.vmbo.GetVolBackupPVCLabels(vb))

	sourceAnnotations := lre.vmbo.GetVolBackupPVCAnnotations(vb)
	restoreName := lre.vmro.GetName(vmr)
	annotations := pvchelper.BuildRestoreAnnotations(sourceAnnotations, restoreName, restorecommon.RestoreNameAnnotation)

	pvc := pvchelper.BuildPVCFromSnapshot(namespace, pvcName, vsName, labels, annotations, pvcSpec)

	_, err := lre.pvcClient.Create(pvc)
	return err
}

func (lre *LonghornRestoreEngine) checkPVCStatus(pvc *corev1.PersistentVolumeClaim) error {
	if err := pvchelper.CheckPVCStatus(pvc); err != nil {
		return err
	}

	// Check Longhorn volume restore status
	provisioner := util.GetProvisionedPVCProvisioner(pvc, lre.scCache)
	if provisioner == "driver.longhorn.io" {
		return lre.checkLonghornVolumeRestore(pvc)
	}

	return nil
}

func (lre *LonghornRestoreEngine) checkLonghornVolumeRestore(pvc *corev1.PersistentVolumeClaim) error {
	volumeName, err := makeVolumeName(pvNamePrefix, string(pvc.ObjectMeta.UID), volumeNameUUIDNoTruncate)
	if err != nil {
		return err
	}

	volumeName = lhutil.AutoCorrectName(volumeName, lhdatastore.NameMaximumLength)

	volume, err := lre.volumeCache.Get(util.LonghornSystemNamespaceName, volumeName)
	if err != nil {
		return err
	}

	for _, condition := range volume.Status.Conditions {
		isRestoreCondition := condition.Type == lhv1beta2.VolumeConditionTypeRestore
		isRestoreFailure := condition.Reason == lhv1beta2.VolumeConditionReasonRestoreFailure

		if isRestoreCondition && isRestoreFailure {
			return fmt.Errorf("volume %s/%s restore failed: %s", volume.Namespace, volume.Name, condition.Message)
		}
	}

	return nil
}

func (lre *LonghornRestoreEngine) UpdateProgress(vr *harvesterv1.VolumeRestore) (int64, error) {
	lhEngineName := lre.vmro.GetVolRestoreLHEngineName(vr)
	if lhEngineName == nil {
		return 0, nil
	}

	lhEngine, err := lre.lhEngineCache.Get(util.LonghornSystemNamespaceName, *lhEngineName)
	if err != nil {
		return 0, err
	}

	if len(lhEngine.Status.RestoreStatus) == 0 {
		return 0, nil
	}

	var numReplica int
	var replicaRestoreProgressSum int

	for _, rs := range lhEngine.Status.RestoreStatus {
		if rs == nil {
			continue
		}

		numReplica++

		if !rs.IsRestoring {
			continue
		}

		replicaRestoreProgressSum += rs.Progress
	}

	if numReplica > 0 {
		return int64(replicaRestoreProgressSum / numReplica), nil
	}

	return 0, nil
}

// Delete removes the VolumeSnapshotContent created during restore for the given
// volume index. VSC is cluster-scoped and uses Retain deletion policy (so the
// underlying Longhorn backup survives VM deletion), which means it cannot be
// garbage-collected via owner references and must be deleted explicitly here.
func (lre *LonghornRestoreEngine) Delete(vmr *harvesterv1.VirtualMachineRestore, volIndex int) error {
	vr := lre.vmro.GetVolRestore(vmr, volIndex)
	if vr == nil {
		return nil
	}

	vbName := lre.vmro.GetVolRestoreVolumeBackupName(vr)
	if vbName == "" {
		return nil
	}

	vscName := name.SafeConcatName("restore", lre.vmro.GetNamespace(vmr), lre.vmro.GetName(vmr), vbName)
	vsc, err := lre.vscCache.Get(vscName)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get VolumeSnapshotContent %s: %w", vscName, err)
	}

	if err := lre.vscClient.Delete(vsc.Name, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete VolumeSnapshotContent %s: %w", vsc.Name, err)
	}
	return nil
}

// Helper functions
func (lre *LonghornRestoreEngine) constructVolumeSnapshotName(vmr *harvesterv1.VirtualMachineRestore, vb *harvesterv1.VolumeBackup) string {
	volBackupName := lre.vmbo.GetVolBackupName(vb)
	if volBackupName == nil {
		return ""
	}
	return name.SafeConcatName("restore", lre.vmro.GetName(vmr), *volBackupName)
}

func (lre *LonghornRestoreEngine) constructVolumeSnapshotContentName(vmr *harvesterv1.VirtualMachineRestore, vb *harvesterv1.VolumeBackup) string {
	volBackupName := lre.vmbo.GetVolBackupName(vb)
	if volBackupName == nil {
		return ""
	}
	return name.SafeConcatName("restore", lre.vmro.GetNamespace(vmr), lre.vmro.GetName(vmr), *volBackupName)
}

func (lre *LonghornRestoreEngine) getVolumeSnapshotContentName(vb *harvesterv1.VolumeBackup) string {
	volBackupName := lre.vmbo.GetVolBackupName(vb)
	if volBackupName == nil {
		return ""
	}
	return fmt.Sprintf("%s-vsc", *volBackupName)
}

func makeVolumeName(prefix, pvcUID string, volumeNameUUIDLength int) (string, error) {
	if len(prefix) == 0 {
		return "", fmt.Errorf("volume name prefix cannot be of length 0")
	}
	if len(pvcUID) == 0 {
		return "", fmt.Errorf("corrupted PVC object, it is missing UID")
	}
	if volumeNameUUIDLength == -1 {
		return fmt.Sprintf("%s-%s", prefix, pvcUID), nil
	}
	return fmt.Sprintf("%s-%s", prefix, pvcUID[:volumeNameUUIDLength]), nil
}

// decodeSnapshotID decodes Longhorn snapshot ID
func decodeSnapshotID(snapshotID string) (csiSnapshotType, sourceVolumeName, id string) {
	split := strings.Split(snapshotID, "://")
	if len(split) < 2 {
		return "", "", ""
	}
	csiSnapshotType = split[0]
	if normalizeCSISnapshotType(csiSnapshotType) == csiSnapshotTypeLonghornBackingImage {
		return csiSnapshotTypeLonghornBackingImage, "", ""
	}

	split = strings.Split(split[1], "/")
	if len(split) < 2 {
		return "", "", ""
	}
	sourceVolumeName = split[0]
	id = split[1]
	return normalizeCSISnapshotType(csiSnapshotType), sourceVolumeName, id
}

// normalizeCSISnapshotType normalizes CSI snapshot type
func normalizeCSISnapshotType(cSISnapshotType string) string {
	if cSISnapshotType == deprecatedCSISnapshotTypeLonghornBackup {
		return csiSnapshotTypeLonghornBackup
	}
	return cSISnapshotType
}

const (
	lhEngineWatcherName = "longhorn-engine-watcher"
)

// RegisterWatchers wires up a Longhorn-Engine → VMRestore event mapping so
// LH restore progress (captured in Engine.Status.RestoreStatus) feeds the
// VMRestore reconcile loop. Lives on the engine (not the controller) because
// the lookup chain Engine → Volume.annotations → VMRestore is LH-specific.
func (lre *LonghornRestoreEngine) RegisterWatchers(ctx context.Context, enqueueVMRestore func(namespace, name string)) {
	lre.lhEngineController.OnChange(ctx, lhEngineWatcherName, func(_ string, lhEngine *lhv1beta2.Engine) (*lhv1beta2.Engine, error) {
		return lre.onLHEngineChanged(lhEngine, enqueueVMRestore)
	})
}

func (lre *LonghornRestoreEngine) onLHEngineChanged(
	lhEngine *lhv1beta2.Engine,
	enqueueVMRestore func(namespace, name string),
) (*lhv1beta2.Engine, error) {
	if !shouldProcessEngine(lhEngine) {
		return nil, nil
	}

	pvcNamespace, pvcName, restoreName, volumeSize, err := lre.getVolumeRestoreInfo(lhEngine)
	if err != nil {
		return nil, err
	}
	if pvcNamespace == "" {
		return nil, nil
	}

	vmr, err := lre.vmrCache.Get(pvcNamespace, restoreName)
	if err != nil {
		return nil, nil
	}

	if err := lre.updateVolumeRestoreMetrics(vmr, pvcNamespace, pvcName, volumeSize, lhEngine); err != nil {
		return nil, err
	}

	enqueueVMRestore(pvcNamespace, restoreName)
	return nil, nil
}

func shouldProcessEngine(lhEngine *lhv1beta2.Engine) bool {
	return lhEngine != nil && lhEngine.DeletionTimestamp == nil && len(lhEngine.Status.RestoreStatus) > 0
}

func (lre *LonghornRestoreEngine) getVolumeRestoreInfo(lhEngine *lhv1beta2.Engine) (pvcNamespace, pvcName, restoreName string, volumeSize int64, err error) {
	volume, err := lre.volumeCache.Get(util.LonghornSystemNamespaceName, lhEngine.Spec.VolumeName)
	if err != nil {
		return "", "", "", 0, err
	}

	pvcNamespace, ok := volume.Annotations[restorecommon.PvcNameSpaceAnnotation]
	if !ok {
		return "", "", "", 0, nil
	}
	pvcName, ok = volume.Annotations[restorecommon.PvcNameAnnotation]
	if !ok {
		return "", "", "", 0, nil
	}
	restoreName, ok = volume.Annotations[restorecommon.RestoreNameAnnotation]
	if !ok {
		return "", "", "", 0, nil
	}
	return pvcNamespace, pvcName, restoreName, volume.Spec.Size, nil
}

func (lre *LonghornRestoreEngine) updateVolumeRestoreMetrics(
	vmr *harvesterv1.VirtualMachineRestore,
	pvcNamespace, pvcName string,
	volumeSize int64,
	lhEngine *lhv1beta2.Engine,
) error {
	vmrCpy := vmr.DeepCopy()

	vr := lre.findMatchingVolumeRestore(vmrCpy, pvcNamespace, pvcName)
	if vr == nil {
		return nil
	}

	if err := lre.vmro.SetVolRestoreLHEngineName(vr, lhEngine.Name); err != nil {
		return err
	}
	if err := lre.vmro.SetVolRestoreVolumeSize(vr, volumeSize); err != nil {
		return err
	}

	_, err := lre.vmro.UpdateByStatus(vmr, vmrCpy)
	return err
}

func (lre *LonghornRestoreEngine) findMatchingVolumeRestore(
	vmrCpy *harvesterv1.VirtualMachineRestore,
	pvcNamespace, pvcName string,
) *harvesterv1.VolumeRestore {
	vrs := lre.vmro.GetVolRestores(vmrCpy)
	for i := range vrs {
		vr := lre.vmro.GetVolRestore(vmrCpy, i)
		if lre.vmro.GetVolRestorePVCNamespace(vr) == pvcNamespace &&
			lre.vmro.GetVolRestorePVCName(vr) == pvcName {
			return vr
		}
	}
	return nil
}
