package backup

// Harvester VM backup & restore controllers helps to manage the VM backup & restore by leveraging
// the VolumeSnapshot functionality of Kubernetes CSI drivers with built-in storage driver longhorn.
// Currently, the following features are supported:
// 1. support VM live & offline backup to the supported backupTarget(i.e, nfs_v4 or s3 storage server).
// 2. restore a backup to a new VM or replacing it with the existing VM is supported.
import (
	"context"
	"fmt"
	"reflect"

	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
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
)

const (
	backupControllerName = "harvester-vm-backup-controller"
	vmBackupKindName     = "VirtualMachineBackup"

	BackupTargetAnnotation       = "backup.harvesterhci.io/backup-target"
	BackupBucketNameAnnotation   = "backup.harvesterhci.io/bucket-name"
	BackupBucketRegionAnnotation = "backup.harvesterhci.io/bucket-region"

	volumeSnapshotMissingEvent = "VolumeSnapshotMissing"
	volumeSnapshotCreateEvent  = "VolumeSnapshotCreated"
)

var vmBackupKind = harvesterv1.SchemeGroupVersion.WithKind(vmBackupKindName)

// RegisterBackup register the vmBackup and volumeSnapshot controller
func RegisterBackup(ctx context.Context, management *config.Management, opts config.Options) error {
	vmBackups := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	pvc := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	secrets := management.CoreFactory.Core().V1().Secret()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	volumes := management.LonghornFactory.Longhorn().V1beta1().Volume()
	snapshots := management.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshot()
	snapshotClass := management.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshotClass()

	vmBackupController := &Handler{
		vmBackups:          vmBackups,
		vmBackupController: vmBackups,
		vmBackupCache:      vmBackups.Cache(),
		pvcCache:           pvc.Cache(),
		secretCache:        secrets.Cache(),
		vms:                vms,
		vmsCache:           vms.Cache(),
		volumeCache:        volumes.Cache(),
		volumes:            volumes,
		snapshots:          snapshots,
		snapshotCache:      snapshots.Cache(),
		snapshotClassCache: snapshotClass.Cache(),
		recorder:           management.NewRecorder(backupControllerName, "", ""),
	}

	vmBackups.OnChange(ctx, backupControllerName, vmBackupController.OnBackupChange)
	snapshots.OnChange(ctx, backupControllerName, vmBackupController.updateVolumeSnapshotChanged)
	return nil
}

type Handler struct {
	vmBackups          ctlharvesterv1.VirtualMachineBackupClient
	vmBackupCache      ctlharvesterv1.VirtualMachineBackupCache
	vmBackupController ctlharvesterv1.VirtualMachineBackupController
	vms                ctlkubevirtv1.VirtualMachineClient
	vmsCache           ctlkubevirtv1.VirtualMachineCache
	pvcCache           ctlcorev1.PersistentVolumeClaimCache
	secretCache        ctlcorev1.SecretCache
	volumeCache        ctllonghornv1.VolumeCache
	volumes            ctllonghornv1.VolumeClient
	snapshots          ctlsnapshotv1.VolumeSnapshotClient
	snapshotCache      ctlsnapshotv1.VolumeSnapshotCache
	snapshotClassCache ctlsnapshotv1.VolumeSnapshotClassCache
	recorder           record.EventRecorder
}

// OnBackupChange handles vm backup object on change and reconcile vm backup status
func (h *Handler) OnBackupChange(key string, vmBackup *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineBackup, error) {
	if vmBackup == nil || vmBackup.DeletionTimestamp != nil {
		return nil, nil
	}

	if isBackupReady(vmBackup) {
		return nil, nil
	}

	// set vmBackup init status
	if vmBackup.Status == nil {
		return nil, h.updateStatus(vmBackup, nil)
	}

	// get vmBackup source
	sourceVM, err := h.getBackupSource(vmBackup)
	if err != nil {
		return nil, err
	}

	// check if the VM is running, if not make sure the volumes are mounted to the host
	if !sourceVM.Status.Ready || !sourceVM.Status.Created {
		if err := h.reconcileLonghornVolumes(sourceVM); err != nil {
			return vmBackup, err
		}
	}

	// TODO, make sure status is initialized, and "Lock" the source VM by adding a finalizer and setting snapshotInProgress in status
	if sourceVM != nil && isBackupProgressing(vmBackup) {
		// create source spec and volume backups if it is not exist
		if vmBackup.Status.SourceSpec == nil || vmBackup.Status.VolumeBackups == nil {
			vmBackup.Status, err = h.updateBackupStatusContent(vmBackup, sourceVM.DeepCopy())
			if err != nil {
				return nil, err
			}
		}

		// reconcile backup status of volume backups, validate if those volumeSnapshots are ready to use
		if err = h.reconcileBackupStatus(vmBackup); err != nil {
			return nil, err
		}

		return vmBackup, h.updateStatus(vmBackup, sourceVM)
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
	var secretBackups []harvesterv1.SecretBackup

	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.UserDataSecretRef != nil {
			secret, err := h.secretCache.Get(vm.Namespace, volume.CloudInitNoCloud.UserDataSecretRef.Name)
			if err != nil {
				return nil, err
			}
			secretBackups = append(secretBackups, harvesterv1.SecretBackup{
				Name: secret.Name,
				Data: secret.Data,
			})
		}
		if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
			secret, err := h.secretCache.Get(vm.Namespace, volume.CloudInitNoCloud.NetworkDataSecretRef.Name)
			if err != nil {
				return nil, err
			}
			secretBackups = append(secretBackups, harvesterv1.SecretBackup{
				Name: secret.Name,
				Data: secret.Data,
			})
		}
	}

	return secretBackups, nil
}

// updateBackupStatusContent helps to store the backup source and volume contents within the VM backup status
func (h *Handler) updateBackupStatusContent(backup *harvesterv1.VirtualMachineBackup, vm *kv1.VirtualMachine) (*harvesterv1.VirtualMachineBackupStatus, error) {
	var err error
	status := backup.Status.DeepCopy()

	if status.VolumeBackups == nil {
		status.VolumeBackups, err = h.getVolumeBackups(backup, vm)
		if err != nil {
			return nil, err
		}
	}

	if status.SecretBackups == nil {
		status.SecretBackups, err = h.getSecretBackups(vm)
		if err != nil {
			return nil, err
		}
	}

	if status.SourceSpec == nil {
		status.SourceSpec = &harvesterv1.VirtualMachineSourceSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name:        vm.ObjectMeta.Name,
				Namespace:   vm.ObjectMeta.Namespace,
				Annotations: vm.ObjectMeta.Annotations,
				Labels:      vm.ObjectMeta.Labels,
			},
			Spec: vm.Spec,
		}
	}

	return status, nil
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

func (h *Handler) updateStatus(vmBackup *harvesterv1.VirtualMachineBackup, source *kv1.VirtualMachine) error {
	var vmBackupCpy = vmBackup.DeepCopy()
	if vmBackupCpy.Status == nil {
		vmBackupCpy.Status = &harvesterv1.VirtualMachineBackupStatus{
			ReadyToUse: pointer.BoolPtr(false),
		}
	}

	if source != nil {
		vmBackupCpy.Status.SourceUID = &source.UID
	}

	if isBackupProgressing(vmBackupCpy) {
		source, err := h.getBackupSource(vmBackupCpy)
		if err != nil {
			return err
		}

		if source != nil {
			updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionTrue, "Operation in progress"))
		} else {
			updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionFalse, "Source does not exist"))
		}
		updateBackupCondition(vmBackupCpy, newReadyCondition(corev1.ConditionFalse, "Not ready"))
	} else if vmBackupError(vmBackupCpy) != nil {
		updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionFalse, "In error state"))
		updateBackupCondition(vmBackupCpy, newReadyCondition(corev1.ConditionFalse, "Error"))
	} else if isBackupReady(vmBackupCpy) {
		updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionFalse, "Operation complete"))
		updateBackupCondition(vmBackupCpy, newReadyCondition(corev1.ConditionTrue, "Operation complete"))
	} else {
		updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionUnknown, "Unknown state"))
		updateBackupCondition(vmBackupCpy, newReadyCondition(corev1.ConditionUnknown, "Unknown state"))
	}

	if vmBackupCpy.Annotations == nil {
		vmBackupCpy.Annotations = make(map[string]string)
	}

	if vmBackupCpy.Annotations[BackupTargetAnnotation] == "" {
		target, err := decodeTarget(settings.BackupTargetSet.Get())
		if err != nil {
			return err
		}
		vmBackupCpy.Annotations[BackupTargetAnnotation] = target.Endpoint
		if target.Type == settings.S3BackupType {
			vmBackupCpy.Annotations[BackupBucketNameAnnotation] = target.BucketName
			vmBackupCpy.Annotations[BackupBucketRegionAnnotation] = target.BucketRegion
		}
	}

	if !reflect.DeepEqual(vmBackup.Status, vmBackupCpy.Status) && vmBackupCpy.Status != nil {
		if _, err := h.vmBackups.Update(vmBackupCpy); err != nil {
			return err
		}
	}
	return nil
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
