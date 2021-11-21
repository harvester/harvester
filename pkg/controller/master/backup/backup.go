package backup

// Harvester VM backup & restore controllers helps to manage the VM backup & restore by leveraging
// the VolumeSnapshot functionality of Kubernetes CSI drivers with built-in storage driver longhorn.
// Currently, the following features are supported:
// 1. support VM live & offline backup to the supported backupTarget(i.e, nfs_v4 or s3 storage server).
// 2. restore a backup to a new VM or replacing it with the existing VM is supported.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"github.com/longhorn/backupstore"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	kv1 "kubevirt.io/client-go/api/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	backupControllerName = "harvester-vm-backup-controller"
	vmBackupKindName     = "VirtualMachineBackup"

	BackupTargetAnnotation       = "backup.harvesterhci.io/backup-target"
	BackupBucketNameAnnotation   = "backup.harvesterhci.io/bucket-name"
	BackupBucketRegionAnnotation = "backup.harvesterhci.io/bucket-region"

	volumeSnapshotCreateEvent = "VolumeSnapshotCreated"
)

var vmBackupKind = harvesterv1.SchemeGroupVersion.WithKind(vmBackupKindName)

// RegisterBackup register the vmBackup and volumeSnapshot controller
func RegisterBackup(ctx context.Context, management *config.Management, opts config.Options) error {
	vmBackups := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	pvc := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	secrets := management.CoreFactory.Core().V1().Secret()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	volumes := management.LonghornFactory.Longhorn().V1beta1().Volume()
	lhbackups := management.LonghornFactory.Longhorn().V1beta1().Backup()
	snapshots := management.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshot()
	snapshotContents := management.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshotContent()
	snapshotClass := management.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshotClass()

	vmBackupController := &Handler{
		vmBackups:            vmBackups,
		vmBackupController:   vmBackups,
		vmBackupCache:        vmBackups.Cache(),
		pvcCache:             pvc.Cache(),
		secretCache:          secrets.Cache(),
		vms:                  vms,
		vmsCache:             vms.Cache(),
		volumeCache:          volumes.Cache(),
		volumes:              volumes,
		lhbackupCache:        lhbackups.Cache(),
		snapshots:            snapshots,
		snapshotCache:        snapshots.Cache(),
		snapshotContents:     snapshotContents,
		snapshotContentCache: snapshotContents.Cache(),
		snapshotClassCache:   snapshotClass.Cache(),
		recorder:             management.NewRecorder(backupControllerName, "", ""),
	}

	vmBackups.OnChange(ctx, backupControllerName, vmBackupController.OnBackupChange)
	snapshots.OnChange(ctx, backupControllerName, vmBackupController.updateVolumeSnapshotChanged)
	lhbackups.OnChange(ctx, backupControllerName, vmBackupController.OnLHBackupChanged)
	return nil
}

type Handler struct {
	vmBackups            ctlharvesterv1.VirtualMachineBackupClient
	vmBackupCache        ctlharvesterv1.VirtualMachineBackupCache
	vmBackupController   ctlharvesterv1.VirtualMachineBackupController
	vms                  ctlkubevirtv1.VirtualMachineClient
	vmsCache             ctlkubevirtv1.VirtualMachineCache
	pvcCache             ctlcorev1.PersistentVolumeClaimCache
	secretCache          ctlcorev1.SecretCache
	volumeCache          ctllonghornv1.VolumeCache
	volumes              ctllonghornv1.VolumeClient
	lhbackupCache        ctllonghornv1.BackupCache
	snapshots            ctlsnapshotv1.VolumeSnapshotClient
	snapshotCache        ctlsnapshotv1.VolumeSnapshotCache
	snapshotContents     ctlsnapshotv1.VolumeSnapshotContentClient
	snapshotContentCache ctlsnapshotv1.VolumeSnapshotContentCache
	snapshotClassCache   ctlsnapshotv1.VolumeSnapshotClassCache
	recorder             record.EventRecorder
}

// OnBackupChange handles vm backup object on change and reconcile vm backup status
func (h *Handler) OnBackupChange(key string, vmBackup *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineBackup, error) {
	if vmBackup == nil || vmBackup.DeletionTimestamp != nil || isBackupReady(vmBackup) {
		return nil, nil
	}

	// set vmBackup init status
	if isBackupMissingStatus(vmBackup) {
		// get vmBackup source
		sourceVM, err := h.getBackupSource(vmBackup)
		if err != nil {
			return nil, err
		}

		// check if the VM is running, if not make sure the volumes are mounted to the host
		if !sourceVM.Status.Ready || !sourceVM.Status.Created {
			if err := h.mountLonghornVolumes(sourceVM); err != nil {
				return nil, err
			}
		}

		return nil, h.initBackup(vmBackup, sourceVM)
	}

	target, err := settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return nil, err
	}

	if isBackupAnnotationMissing(vmBackup) {
		return nil, h.initAnnotations(vmBackup, target)
	}

	// TODO, make sure status is initialized, and "Lock" the source VM by adding a finalizer and setting snapshotInProgress in status

	// create volume snapshots if not exist
	if err := h.reconcileVolumeSnapshots(vmBackup); err != nil {
		return nil, h.setStatusError(vmBackup, err)
	}

	// reconcile backup status of volume backups, validate if those volumeSnapshots are ready to use
	if err := h.updateConditions(vmBackup); err != nil {
		return nil, err
	}

	// generate vm backup metadata and upload to backup target
	if err := h.uploadVMBackupMetadata(vmBackup, target); err != nil {
		return nil, err
	}

	return nil, nil
}

func (h *Handler) getBackupSource(vmBackup *harvesterv1.VirtualMachineBackup) (*kv1.VirtualMachine, error) {
	switch vmBackup.Spec.Source.Kind {
	case kv1.VirtualMachineGroupVersionKind.Kind:
		return h.vmsCache.Get(vmBackup.Namespace, vmBackup.Spec.Source.Name)
	}

	return nil, fmt.Errorf("unsupported source: %+v", vmBackup.Spec.Source)
}

// getVolumeBackups helps to build a list of VolumeBackup upon the volume list of backup VM
func (h *Handler) getVolumeBackups(backup *harvesterv1.VirtualMachineBackup, vm *kv1.VirtualMachine) ([]harvesterv1.VolumeBackup, error) {
	sourceVolumes := vm.Spec.Template.Spec.Volumes
	var volumeBackups = make([]harvesterv1.VolumeBackup, 0, len(sourceVolumes))

	for volumeName, pvcName := range volumeToPVCMappings(sourceVolumes) {
		pvc, err := h.getBackupPVC(vm.Namespace, pvcName)
		if err != nil {
			return nil, err
		}

		volumeBackupName := fmt.Sprintf("%s-volume-%s", backup.Name, pvcName)

		vb := harvesterv1.VolumeBackup{
			Name:       &volumeBackupName,
			VolumeName: volumeName,
			PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:        pvc.ObjectMeta.Name,
					Namespace:   pvc.ObjectMeta.Namespace,
					Labels:      pvc.Labels,
					Annotations: pvc.Annotations,
				},
				Spec: pvc.Spec,
			},
			ReadyToUse: pointer.BoolPtr(false),
		}

		volumeBackups = append(volumeBackups, vb)
	}

	return volumeBackups, nil
}

// getSecretBackups helps to build a list of SecretBackup upon the cloud init secrets used by the backup VM
func (h *Handler) getSecretBackups(vm *kv1.VirtualMachine) ([]harvesterv1.SecretBackup, error) {
	secretRefs := []*corev1.LocalObjectReference{}

	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.UserDataSecretRef != nil {
			secretRefs = append(secretRefs, volume.CloudInitNoCloud.UserDataSecretRef)
		}
		if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
			secretRefs = append(secretRefs, volume.CloudInitNoCloud.NetworkDataSecretRef)
		}
	}

	secretBackups := []harvesterv1.SecretBackup{}
	secretBackupMap := map[string]bool{}
	for _, secretRef := range secretRefs {
		// users may put UserDataSecretRef and NetworkDataSecretRef in a same secret, so we only keep one
		secretFullName := fmt.Sprintf("%s/%s", vm.Namespace, secretRef.Name)
		if v, ok := secretBackupMap[secretFullName]; ok && v {
			continue
		}

		secretBackup, err := h.getSecretBackupFromSecret(vm.Namespace, secretRef.Name)
		if err != nil {
			return nil, err
		}
		secretBackupMap[secretFullName] = true
		secretBackups = append(secretBackups, *secretBackup)
	}

	return secretBackups, nil
}

func (h *Handler) getSecretBackupFromSecret(namespace, name string) (*harvesterv1.SecretBackup, error) {
	secret, err := h.secretCache.Get(namespace, name)
	if err != nil {
		return nil, err
	}

	// Remove empty string. If there is empty string in secret, we will encounter error.
	// ref: https://github.com/harvester/harvester/issues/1536
	data := secret.DeepCopy().Data
	for k, v := range secret.Data {
		if len(v) == 0 {
			delete(data, k)
		}
	}

	return &harvesterv1.SecretBackup{Name: secret.Name, Data: data}, nil
}

// initBackup initialize VM backup status and annotation
func (h *Handler) initBackup(backup *harvesterv1.VirtualMachineBackup, vm *kv1.VirtualMachine) error {
	var err error
	backupCpy := backup.DeepCopy()
	backupCpy.Status = &harvesterv1.VirtualMachineBackupStatus{
		ReadyToUse: pointer.BoolPtr(false),
		SourceUID:  &vm.UID,
		SourceSpec: &harvesterv1.VirtualMachineSourceSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name:        vm.ObjectMeta.Name,
				Namespace:   vm.ObjectMeta.Namespace,
				Annotations: vm.ObjectMeta.Annotations,
				Labels:      vm.ObjectMeta.Labels,
			},
			Spec: vm.Spec,
		},
	}

	if backupCpy.Status.VolumeBackups, err = h.getVolumeBackups(backup, vm); err != nil {
		return err
	}

	if backupCpy.Status.SecretBackups, err = h.getSecretBackups(vm); err != nil {
		return err
	}

	if _, err := h.vmBackups.Update(backupCpy); err != nil {
		return err
	}
	return nil
}

func (h *Handler) initAnnotations(backup *harvesterv1.VirtualMachineBackup, target *settings.BackupTarget) error {
	var backupCpy = backup.DeepCopy()
	if backupCpy.Annotations == nil {
		backupCpy.Annotations = map[string]string{}
	}

	if backupCpy.Annotations[BackupTargetAnnotation] == "" {
		backupCpy.Annotations[BackupTargetAnnotation] = target.Endpoint
		if target.Type == settings.S3BackupType {
			backupCpy.Annotations[BackupBucketNameAnnotation] = target.BucketName
			backupCpy.Annotations[BackupBucketRegionAnnotation] = target.BucketRegion
		}
	}

	if _, err := h.vmBackups.Update(backupCpy); err != nil {
		return err
	}
	return nil
}

func (h *Handler) getBackupPVC(namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := h.pvcCache.Get(namespace, name)
	if err != nil {
		return nil, err
	}

	if pvc.Spec.VolumeName == "" {
		return nil, fmt.Errorf("unbound PVC %s/%s", pvc.Namespace, pvc.Name)
	}

	if pvc.Spec.StorageClassName == nil {
		return nil, fmt.Errorf("no storage class for PVC %s/%s", pvc.Namespace, pvc.Name)
	}

	return pvc, nil
}

// reconcileVolumeSnapshots create volume snapshot if not exist.
// For vm backup from a existent VM, we create volume snapshot from pvc.
// For vm backup from syncing vm backup metadata, we create volume snapshot from volume snapshot content.
func (h *Handler) reconcileVolumeSnapshots(vmBackup *harvesterv1.VirtualMachineBackup) error {
	vmBackupCpy := vmBackup.DeepCopy()
	for i, volumeBackup := range vmBackupCpy.Status.VolumeBackups {
		if volumeBackup.Name == nil {
			continue
		}

		snapshotName := *volumeBackup.Name
		volumeSnapshot, err := h.getVolumeSnapshot(vmBackupCpy.Namespace, snapshotName)
		if err != nil {
			return err
		}

		if volumeSnapshot == nil {
			volumeSnapshot, err = h.createVolumeSnapshot(vmBackupCpy, volumeBackup)
			if err != nil {
				logrus.Errorf("create volume snapshot error: %v", err)
				return err
			}
		}

		if volumeSnapshot.Status != nil {
			vmBackupCpy.Status.VolumeBackups[i].ReadyToUse = volumeSnapshot.Status.ReadyToUse
			vmBackupCpy.Status.VolumeBackups[i].CreationTime = volumeSnapshot.Status.CreationTime
			vmBackupCpy.Status.VolumeBackups[i].Error = translateError(volumeSnapshot.Status.Error)
		}

	}

	if !reflect.DeepEqual(vmBackup.Status, vmBackupCpy.Status) {
		if _, err := h.vmBackups.Update(vmBackupCpy); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) getVolumeSnapshot(namespace, name string) (*snapshotv1.VolumeSnapshot, error) {
	snapshot, err := h.snapshotCache.Get(namespace, name)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return snapshot, nil
}

func (h *Handler) createVolumeSnapshot(vmBackup *harvesterv1.VirtualMachineBackup, volumeBackup harvesterv1.VolumeBackup) (*snapshotv1.VolumeSnapshot, error) {
	logrus.Debugf("attempting to create VolumeSnapshot %s", *volumeBackup.Name)

	sc, err := h.snapshotClassCache.Get(settings.VolumeSnapshotClass.Get())
	if err != nil {
		return nil, fmt.Errorf("%s/%s VolumeSnapshot requested but no storage class, err: %s",
			vmBackup.Namespace, volumeBackup.PersistentVolumeClaim.ObjectMeta.Name, err.Error())
	}

	pvc, err := h.pvcCache.Get(volumeBackup.PersistentVolumeClaim.ObjectMeta.Namespace, volumeBackup.PersistentVolumeClaim.ObjectMeta.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	volumeSnapshotSource := snapshotv1.VolumeSnapshotSource{}
	if pvc != nil {
		volumeSnapshotSource.PersistentVolumeClaimName = &volumeBackup.PersistentVolumeClaim.ObjectMeta.Name
	} else {
		volumeSnapshotContent, err := h.createVolumeSnapshotContent(vmBackup, volumeBackup, sc)
		if err != nil {
			return nil, err
		}
		volumeSnapshotSource.VolumeSnapshotContentName = &volumeSnapshotContent.Name
	}

	snapshot := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *volumeBackup.Name,
			Namespace: vmBackup.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         harvesterv1.SchemeGroupVersion.String(),
					Kind:               vmBackupKind.Kind,
					Name:               vmBackup.Name,
					UID:                vmBackup.UID,
					Controller:         pointer.BoolPtr(true),
					BlockOwnerDeletion: pointer.BoolPtr(true),
				},
			},
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source:                  volumeSnapshotSource,
			VolumeSnapshotClassName: pointer.StringPtr(sc.Name),
		},
	}

	volumeSnapshot, err := h.snapshots.Create(snapshot)
	if err != nil {
		return nil, err
	}

	h.recorder.Eventf(
		vmBackup,
		corev1.EventTypeNormal,
		volumeSnapshotCreateEvent,
		"Successfully created VolumeSnapshot %s",
		snapshot.Name,
	)

	return volumeSnapshot, nil
}

func (h *Handler) createVolumeSnapshotContent(
	vmBackup *harvesterv1.VirtualMachineBackup,
	volumeBackup harvesterv1.VolumeBackup,
	snapshotClass *snapshotv1.VolumeSnapshotClass,
) (*snapshotv1.VolumeSnapshotContent, error) {
	logrus.Debugf("attempting to create VolumeSnapshotContent %s", *volumeBackup.Name)
	snapshotContent, err := h.snapshotContents.Get(getVolumeSnapshotContentName(volumeBackup), metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	} else if err == nil {
		return snapshotContent, nil
	}

	lhBackup, err := h.lhbackupCache.Get(util.LonghornSystemNamespaceName, *volumeBackup.LonghornBackupName)
	if err != nil {
		return nil, err
	}
	snapshotHandle := fmt.Sprintf("bs://%s/%s", volumeBackup.PersistentVolumeClaim.ObjectMeta.Name, lhBackup.Name)

	return h.snapshotContents.Create(&snapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getVolumeSnapshotContentName(volumeBackup),
			Namespace: vmBackup.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         harvesterv1.SchemeGroupVersion.String(),
					Kind:               vmBackupKind.Kind,
					Name:               vmBackup.Name,
					UID:                vmBackup.UID,
					Controller:         pointer.BoolPtr(true),
					BlockOwnerDeletion: pointer.BoolPtr(true),
				},
			},
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver:         "driver.longhorn.io",
			DeletionPolicy: snapshotv1.VolumeSnapshotContentDelete,
			Source: snapshotv1.VolumeSnapshotContentSource{
				SnapshotHandle: pointer.StringPtr(snapshotHandle),
			},
			VolumeSnapshotClassName: pointer.StringPtr(snapshotClass.Name),
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      *volumeBackup.Name,
				Namespace: vmBackup.Namespace,
			},
		},
	})
}

func (h *Handler) setStatusError(vmBackup *harvesterv1.VirtualMachineBackup, err error) error {
	vmBackupCpy := vmBackup.DeepCopy()
	vmBackupCpy.Status.Error = &harvesterv1.Error{
		Time:    currentTime(),
		Message: pointer.StringPtr(err.Error()),
	}
	updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionFalse, "In error state"))
	updateBackupCondition(vmBackupCpy, newReadyCondition(corev1.ConditionFalse, "Error"))

	if _, err = h.vmBackups.Update(vmBackupCpy); err != nil {
		return err
	}
	return err
}

func (h *Handler) uploadVMBackupMetadata(vmBackup *harvesterv1.VirtualMachineBackup, target *settings.BackupTarget) error {
	if target.Type == settings.S3BackupType {
		secret, err := h.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
		if err != nil {
			return err
		}
		os.Setenv(AWSAccessKey, string(secret.Data[AWSAccessKey]))
		os.Setenv(AWSSecretKey, string(secret.Data[AWSSecretKey]))
		os.Setenv(AWSEndpoints, string(secret.Data[AWSEndpoints]))
		os.Setenv(AWSCERT, string(secret.Data[AWSCERT]))
	}

	bsDriver, err := backupstore.GetBackupStoreDriver(ConstructEndpoint(target))
	if err != nil {
		return err
	}

	vmBackupMetadata := &VirtualMachineBackupMetadata{
		Name:          vmBackup.Name,
		Namespace:     vmBackup.Namespace,
		BackupSpec:    vmBackup.Spec,
		VMSourceSpec:  vmBackup.Status.SourceSpec,
		VolumeBackups: sanitizeVolumeBackups(vmBackup.Status.VolumeBackups),
		SecretBackups: vmBackup.Status.SecretBackups,
	}
	if vmBackup.Namespace == "" {
		vmBackupMetadata.Namespace = metav1.NamespaceDefault
	}

	j, err := json.Marshal(vmBackupMetadata)
	if err != nil {
		return err
	}

	destURL := filepath.Join(metadataFolderPath, fmt.Sprintf("%s.cfg", vmBackup.Name))
	if err := bsDriver.Write(destURL, bytes.NewReader(j)); err != nil {
		return err
	}

	return nil
}

func sanitizeVolumeBackups(volumeBackups []harvesterv1.VolumeBackup) []harvesterv1.VolumeBackup {
	for i := 0; i < len(volumeBackups); i++ {
		volumeBackups[i].ReadyToUse = nil
	}
	return volumeBackups
}

func volumeToPVCMappings(volumes []kv1.Volume) map[string]string {
	pvcs := map[string]string{}

	for _, volume := range volumes {
		var pvcName string

		if volume.PersistentVolumeClaim != nil {
			pvcName = volume.PersistentVolumeClaim.ClaimName
		} else {
			continue
		}

		pvcs[volume.Name] = pvcName
	}

	return pvcs
}
