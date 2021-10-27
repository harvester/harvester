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
	"strings"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	kv1 "kubevirt.io/client-go/api/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
)

var (
	restoreAnnotationsToDelete = []string{
		"pv.kubernetes.io",
		"volume.beta.kubernetes.io",
		ref.AnnotationSchemaOwnerKeyName,
	}
)

const (
	restoreControllerName = "harvester-vm-restore-controller"

	volumeSnapshotKindName = "VolumeSnapshot"
	vmRestoreKindName      = "VirtualMachineRestore"

	restoreNameAnnotation = "restore.harvesterhci.io/name"
	lastRestoreAnnotation = "restore.harvesterhci.io/last-restore-uid"

	vmCreatorLabel = "harvesterhci.io/creator"
	vmNameLabel    = "harvesterhci.io/vm-name"

	restoreErrorEvent    = "VirtualMachineRestoreError"
	restoreCompleteEvent = "VirtualMachineRestoreComplete"
)

type vmRestoreTarget struct {
	handler   *RestoreHandler
	vmRestore *harvesterv1.VirtualMachineRestore
	vm        *kv1.VirtualMachine
	newVM     bool
}

type RestoreHandler struct {
	context context.Context

	restores          ctlharvesterv1.VirtualMachineRestoreClient
	restoreController ctlharvesterv1.VirtualMachineRestoreController
	backupCache       ctlharvesterv1.VirtualMachineBackupCache
	vms               ctlkubevirtv1.VirtualMachineClient
	vmCache           ctlkubevirtv1.VirtualMachineCache
	pvcClient         ctlcorev1.PersistentVolumeClaimClient
	pvcCache          ctlcorev1.PersistentVolumeClaimCache
	secretClient      ctlcorev1.SecretClient
	secretCache       ctlcorev1.SecretCache

	recorder   record.EventRecorder
	restClient *rest.RESTClient
}

func RegisterRestore(ctx context.Context, management *config.Management, opts config.Options) error {
	restores := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore()
	backups := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	secrets := management.CoreFactory.Core().V1().Secret()

	handler := &RestoreHandler{
		context:           ctx,
		restores:          restores,
		restoreController: restores,
		backupCache:       backups.Cache(),
		vms:               vms,
		vmCache:           vms.Cache(),
		pvcClient:         pvcs,
		pvcCache:          pvcs.Cache(),
		secretClient:      secrets,
		secretCache:       secrets.Cache(),
		recorder:          management.NewRecorder(restoreControllerName, "", ""),
	}

	restores.OnChange(ctx, restoreControllerName, handler.RestoreOnChanged)
	pvcs.OnChange(ctx, restoreControllerName, handler.PersistentVolumeClaimOnChange)
	vms.OnChange(ctx, restoreControllerName, handler.VMOnChange)
	return nil
}

// RestoreOnChanged handles vmRestore CRD object on change, it will help to create the new PVCs and either replace them
// with existing VM or used for the new VM.
func (h *RestoreHandler) RestoreOnChanged(key string, restore *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	if restore == nil || restore.DeletionTimestamp != nil {
		return nil, nil
	}

	if !vmRestoreProgressing(restore) {
		return nil, nil
	}

	restoreCpy := restore.DeepCopy()

	if restoreCpy.Status == nil {
		restoreCpy.Status = &harvesterv1.VirtualMachineRestoreStatus{
			Complete: pointer.BoolPtr(false),
		}
	}

	// construct the vmRestore target and check if the backup reference is existed
	target, err := h.getTarget(restoreCpy)
	if err != nil {
		return nil, h.doUpdateError(restore, restoreCpy, err, true)
	}

	if target == nil {
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, fmt.Sprintf("failed to find restore target %s", restoreCpy.Spec.Target.Name)))
		return nil, h.doUpdate(restore, restoreCpy)
	}

	backup, err := h.getVMBackup(restoreCpy, target.UID(), restoreCpy.Spec.NewVM)
	if err != nil {
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, err.Error()))
		return nil, h.doUpdate(restore, restoreCpy)
	}

	// set vmRestore owner reference to the target VM
	if len(restoreCpy.OwnerReferences) == 0 && !target.newVM {
		restoreCpy.SetOwnerReferences(configVMOwner(target.vm))
		updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "Initializing VirtualMachineRestore"))
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "Initializing VirtualMachineRestore"))
		return nil, h.doUpdate(restore, restoreCpy)
	}

	// reconcile restoring volumes and create new PVC from CSI volumeSnapshot if not exist
	var updated bool
	restoreCpy, updated, err = h.reconcileVolumeRestores(restoreCpy, backup)
	if err != nil {
		return nil, h.doUpdateError(restore, restoreCpy, fmt.Errorf("error reconciling VolumeRestores"), true)
	}

	// create target vm after restore PVC volumes
	if target.vm == nil && restoreCpy.Spec.NewVM {
		target.vm, err = h.createNewVM(restoreCpy, backup)
		if err != nil {
			return nil, h.doUpdateError(restore, restoreCpy, err, true)
		}
	}

	//restore referenced secrets
	if err := h.restoreCloudInitSecrets(backup, target); err != nil {
		return nil, h.doUpdateError(restore, restoreCpy, err, true)
	}

	if !updated {
		var ready bool
		ready, err = target.Ready()
		if err != nil {
			return nil, h.doUpdateError(restore, restoreCpy, fmt.Errorf("error checking target ready, err:%s", err.Error()), false)
		}

		// reconcile the vm if the restore is not ready
		if ready {
			updated, err = target.Reconcile()
			if err != nil {
				return nil, h.doUpdateError(restore, restoreCpy, fmt.Errorf("error reconciling target, err:%s", err.Error()), false)
			}

			if !updated {
				if err = target.Cleanup(); err != nil {
					return nil, h.doUpdateError(restore, restoreCpy, fmt.Errorf("error cleaning up, err:%s", err.Error()), false)
				}

				if err = target.RestartVM(); err != nil {
					return nil, fmt.Errorf("failed to restart vm, err:%s", err.Error())
				}

				h.recorder.Eventf(
					restoreCpy,
					corev1.EventTypeNormal,
					restoreCompleteEvent,
					"Successfully completed VirtualMachineRestore %s",
					restoreCpy.Name,
				)

				restoreCpy.Status.RestoreTime = currentTime()
				restoreCpy.Status.Complete = pointer.BoolPtr(true)
				updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionFalse, "Operation complete"))
				updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionTrue, "Operation complete"))
			} else {
				updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "Updating target spec"))
				updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "Waiting for target update"))
			}
		} else {
			reason := "Waiting for target vm to be ready"
			updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionFalse, reason))
			updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, reason))
			// try again in 5 secs
			h.enqueueAfter(restore, restoreCpy, 5*time.Second)
			return nil, nil
		}
	} else {
		vmCpy := target.vm
		if vmCpy.Annotations == nil {
			vmCpy.Annotations = make(map[string]string)
		}

		if vmCpy.Annotations[restoreNameAnnotation] != restoreCpy.Name {
			vmCpy.Annotations[restoreNameAnnotation] = restoreCpy.Name
			if _, err = h.vms.Update(vmCpy); err != nil {
				return nil, err
			}
		}
		updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "Creating new PVCs"))
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "Waiting for new PVCs"))
	}
	return nil, h.doUpdate(restore, restoreCpy)
}

func (h *RestoreHandler) restoreCloudInitSecrets(backup *harvesterv1.VirtualMachineBackup, target *vmRestoreTarget) error {
	if !target.newVM {
		//Update existing secrets
		for _, secretBackup := range backup.Status.SecretBackups {
			secret, err := h.secretCache.Get(backup.Namespace, secretBackup.Name)
			if err != nil {
				return err
			}
			toUpdate := secret.DeepCopy()
			toUpdate.Data = secretBackup.Data
			if _, err := h.secretClient.Update(toUpdate); err != nil {
				return err
			}
		}
	} else {
		for _, secretBackup := range backup.Status.SecretBackups {
			for _, volume := range backup.Status.SourceSpec.Spec.Template.Spec.Volumes {
				if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.UserDataSecretRef != nil &&
					secretBackup.Name == volume.CloudInitNoCloud.UserDataSecretRef.Name {
					newSecretName := getVMUserDataSecretName(target.vmRestore.Spec.Target.Name, volume.Name)
					newSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      newSecretName,
							Namespace: target.vmRestore.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: target.vm.APIVersion,
									Kind:       target.vm.Kind,
									Name:       target.vm.Name,
									UID:        target.vm.UID,
								},
							},
						},
						Data: secretBackup.Data,
					}
					if _, err := h.secretClient.Create(newSecret); err != nil && !apierrors.IsAlreadyExists(err) {
						return err
					}
				}
				if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.NetworkDataSecretRef != nil &&
					secretBackup.Name == volume.CloudInitNoCloud.NetworkDataSecretRef.Name {
					newSecretName := getVMNetworkDataSecretName(target.vmRestore.Spec.Target.Name, volume.Name)
					newSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      newSecretName,
							Namespace: target.vmRestore.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: target.vm.APIVersion,
									Kind:       target.vm.Kind,
									Name:       target.vm.Name,
									UID:        target.vm.UID,
								},
							},
						},
						Data: secretBackup.Data,
					}
					if _, err := h.secretClient.Create(newSecret); err != nil && !apierrors.IsAlreadyExists(err) {
						return err
					}
				}
			}
		}
	}
	return nil
}

// PersistentVolumeClaimOnChange watching the PVCs on change and enqueue the vmRestore if it has the restore annotation
func (h *RestoreHandler) PersistentVolumeClaimOnChange(key string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc == nil || pvc.DeletionTimestamp != nil {
		return nil, nil
	}

	restoreName, ok := pvc.Annotations[restoreNameAnnotation]
	if !ok {
		return nil, nil
	}

	logrus.Debugf("handling PVC updating %s/%s", pvc.Namespace, pvc.Name)
	h.restoreController.EnqueueAfter(pvc.Namespace, restoreName, 5*time.Second)
	return nil, nil
}

// VMOnChange watching the VM on change and enqueue the vmRestore if it has the restore annotation
func (h *RestoreHandler) VMOnChange(key string, vm *kv1.VirtualMachine) (*kv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil {
		return nil, nil
	}

	restoreName, ok := vm.Annotations[restoreNameAnnotation]
	if !ok {
		return nil, nil
	}

	logrus.Debugf("handling VM updating %s/%s", vm.Namespace, vm.Name)
	h.restoreController.EnqueueAfter(vm.Namespace, restoreName, 5*time.Second)
	return nil, nil
}

// getTarget helps to construct the vmRestoreTarget object and config whether to restoring the new VM
// upon the vmRestore.spec.newVM option.
func (h *RestoreHandler) getTarget(vmRestore *harvesterv1.VirtualMachineRestore) (*vmRestoreTarget, error) {
	isNewVM := false
	switch vmRestore.Spec.Target.Kind {
	case kv1.VirtualMachineGroupVersionKind.Kind:
		vm, err := h.vmCache.Get(vmRestore.Namespace, vmRestore.Spec.Target.Name)
		if err != nil {
			if !apierrors.IsNotFound(err) && !vmRestore.Spec.NewVM {
				return nil, err
			}
			isNewVM = true
		}

		return &vmRestoreTarget{
			handler:   h,
			vmRestore: vmRestore,
			vm:        vm,
			newVM:     isNewVM,
		}, nil
	}

	return nil, fmt.Errorf("unknown source %+v", vmRestore.Spec.Target)
}

// getVolumeRestores helps to create an array of new restored volumes
func getVolumeRestores(vmRestore *harvesterv1.VirtualMachineRestore, backup *harvesterv1.VirtualMachineBackup) ([]harvesterv1.VolumeRestore, error) {
	restores := make([]harvesterv1.VolumeRestore, 0, len(backup.Status.VolumeBackups))
	for _, vb := range backup.Status.VolumeBackups {
		found := false
		for _, vr := range vmRestore.Status.VolumeRestores {
			if vb.VolumeName == vr.VolumeName {
				restores = append(restores, vr)
				found = true
				break
			}
		}

		if !found {
			if vb.Name == nil {
				return nil, fmt.Errorf("VolumeSnapshotName missing %+v", vb)
			}

			vr := harvesterv1.VolumeRestore{
				VolumeName: vb.VolumeName,
				PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:      restorePVCName(vmRestore, vb.VolumeName),
						Namespace: vmRestore.Namespace,
					},
					Spec: vb.PersistentVolumeClaim.Spec,
				},
				VolumeBackupName: *vb.Name,
			}
			restores = append(restores, vr)
		}
	}
	return restores, nil
}

func (h *RestoreHandler) reconcileVolumeRestores(vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineRestore, bool, error) {

	if vmRestore.Status.VolumeRestores == nil {
		volumeRestores, err := getVolumeRestores(vmRestore, backup)
		if err != nil {
			return vmRestore, false, err
		}
		vmRestore.Status.VolumeRestores = volumeRestores
	}

	createdPVC := false
	waitingPVC := false
	for i, restore := range vmRestore.Status.VolumeRestores {
		pvc, err := h.pvcCache.Get(restore.PersistentVolumeClaim.ObjectMeta.Namespace, restore.PersistentVolumeClaim.ObjectMeta.Name)
		if apierrors.IsNotFound(err) {
			backup := backup.Status.VolumeBackups[i]
			if err = h.createRestoredPVC(vmRestore, backup, restore); err != nil {
				return vmRestore, false, err
			}
			createdPVC = true
			continue
		}
		if err != nil {
			return vmRestore, false, err
		}

		if pvc.Status.Phase == corev1.ClaimPending {
			waitingPVC = true
		} else if pvc.Status.Phase != corev1.ClaimBound {
			return vmRestore, false, fmt.Errorf("PVC %s/%s in status %q", pvc.Namespace, pvc.Name, pvc.Status.Phase)
		}
	}

	if vmRestore.Status.DeletedVolumes == nil {
		var deletedVolumes []string
		for _, vol := range backup.Status.VolumeBackups {
			deletedVolumes = append(deletedVolumes, vol.PersistentVolumeClaim.ObjectMeta.Name)
		}
		vmRestore.Status.DeletedVolumes = deletedVolumes
	}

	return vmRestore, createdPVC || waitingPVC, nil
}

func (h *RestoreHandler) getVMBackup(vmRestore *harvesterv1.VirtualMachineRestore, targetUID types.UID, newVM bool) (*harvesterv1.VirtualMachineBackup, error) {
	backup, err := h.backupCache.Get(vmRestore.Spec.VirtualMachineBackupNamespace, vmRestore.Spec.VirtualMachineBackupName)
	if err != nil {
		return nil, err
	}

	if !isBackupReady(backup) {
		return nil, fmt.Errorf("VMBackup %s is not ready", backup.Name)
	}

	if !newVM && (vmRestore.Status.TargetUID == nil || *vmRestore.Status.TargetUID != targetUID) &&
		(backup.Status.SourceUID == nil || *backup.Status.SourceUID != targetUID) {
		return nil, fmt.Errorf("neither a new VM or VMBackup source and restore target differ")
	}

	if backup.Status.SourceSpec == nil {
		return nil, fmt.Errorf("empty vm backup source spec of %s", backup.Name)
	}

	return backup, nil
}

func (t *vmRestoreTarget) Cleanup() error {
	if t.vmRestore.Spec.NewVM || t.vmRestore.Spec.DeletionPolicy == harvesterv1.VirtualMachineRestoreRetain {
		logrus.Infof("skip clean up carryover resources of vm %s/%s", t.vm.Name, t.vm.Namespace)
		return nil
	}

	// clean up existing pvc
	for _, volName := range t.vmRestore.Status.DeletedVolumes {
		vol, err := t.handler.pvcCache.Get(t.vmRestore.Namespace, volName)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		if vol != nil {
			err = t.handler.pvcClient.Delete(vol.Namespace, vol.Name, &metav1.DeleteOptions{})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (t *vmRestoreTarget) UID() types.UID {
	if t.newVM {
		return ""
	}
	return t.vm.UID
}

func (t *vmRestoreTarget) Ready() (bool, error) {
	_, err := t.vm.RunStrategy()
	if err != nil {
		return false, err
	}

	_, err = t.handler.vmCache.Get(t.vm.Namespace, t.vm.Name)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (t *vmRestoreTarget) RestartVM() error {
	logrus.Infof("restarting the vm %s, current state running:%v", t.vm.Name, *t.vm.Spec.Running)
	var running = true
	if t.vm.Spec.Running == nil || t.vm.Spec.Running == &running {
		return t.handler.restClient.Put().Namespace(t.vm.Namespace).Resource("virtualmachines").SubResource("restart").Name(t.vm.Name).Do(t.handler.context).Error()
	}
	return nil
}

func (h *RestoreHandler) GetNewVolumes(vm *kv1.VirtualMachineSpec, vmRestore *harvesterv1.VirtualMachineRestore) ([]kv1.Volume, error) {
	var newVolumes = make([]kv1.Volume, len(vm.Template.Spec.Volumes))
	copy(newVolumes, vm.Template.Spec.Volumes)

	for j, vol := range vm.Template.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			for _, vr := range vmRestore.Status.VolumeRestores {
				if vr.VolumeName != vol.Name {
					continue
				}

				nv := vol.DeepCopy()
				nv.PersistentVolumeClaim.ClaimName = vr.PersistentVolumeClaim.ObjectMeta.Name
				newVolumes[j] = *nv
			}
		}
	}
	return newVolumes, nil
}

func (t *vmRestoreTarget) Reconcile() (bool, error) {
	logrus.Debugf("VM created, reconciling target VM %s", t.vmRestore.Name)

	restoreID := getRestoreID(t.vmRestore)
	if lastRestoreID, ok := t.vm.Annotations[lastRestoreAnnotation]; ok && lastRestoreID == restoreID {
		return false, nil
	}

	backup, err := t.handler.getVMBackup(t.vmRestore, t.UID(), false)
	if err != nil {
		return false, err
	}

	newVolumes, err := t.handler.GetNewVolumes(&backup.Status.SourceSpec.Spec, t.vmRestore)
	if err != nil {
		return false, err
	}

	vmCpy := t.vm.DeepCopy()
	vmCpy.Spec = backup.Status.SourceSpec.Spec
	vmCpy.Spec.Template.Spec.Volumes = newVolumes
	if vmCpy.Annotations == nil {
		vmCpy.Annotations = make(map[string]string)
	}
	vmCpy.Annotations[lastRestoreAnnotation] = restoreID
	vmCpy.Annotations[restoreNameAnnotation] = t.vmRestore.Name
	_, ok := vmCpy.Annotations[util.AnnotationVolumeClaimTemplates]
	if ok {
		delete(vmCpy.Annotations, util.AnnotationVolumeClaimTemplates)
	}

	if _, err = t.handler.vms.Update(vmCpy); err != nil {
		return false, err
	}

	return true, nil
}

// createNewVM helps to create new target VM and set the associated owner reference
func (h *RestoreHandler) createNewVM(restore *harvesterv1.VirtualMachineRestore, backup *harvesterv1.VirtualMachineBackup) (*kv1.VirtualMachine, error) {
	vmName := restore.Spec.Target.Name
	logrus.Infof("restore target does not exist, creating a new vm %s", vmName)

	restoreID := getRestoreID(restore)
	vmCpy := backup.Status.SourceSpec.DeepCopy()
	vm := &kv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmName,
			Namespace: restore.Namespace,
			Annotations: map[string]string{
				lastRestoreAnnotation: restoreID,
				restoreNameAnnotation: restore.Name,
			},
		},
		Spec: kv1.VirtualMachineSpec{
			Running: pointer.BoolPtr(true),
			Template: &kv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: vmCpy.Spec.Template.ObjectMeta.Annotations,
					Labels: map[string]string{
						vmCreatorLabel: "harvester",
						vmNameLabel:    vmName,
					},
				},
				Spec: sanitizeVirtualMachineForRestore(restore, vmCpy.Spec.Template.Spec),
			},
		},
	}

	newVolumes, err := h.GetNewVolumes(&vm.Spec, restore)
	if err != nil {
		return nil, err
	}
	vm.Spec.Template.Spec.Volumes = newVolumes

	for i := range vm.Spec.Template.Spec.Domain.Devices.Interfaces {
		// remove the copied mac address of the new VM
		vm.Spec.Template.Spec.Domain.Devices.Interfaces[i].MacAddress = ""
	}

	newVM, err := h.vms.Create(vm)
	if err != nil {
		return nil, err
	}

	if restore.Status.TargetUID == nil {
		restore.Status.TargetUID = &newVM.UID
	}
	vm.SetOwnerReferences(configVMOwner(vm))
	if _, err = h.restores.Update(restore); err != nil {
		return nil, err
	}

	return vm, nil
}

func sanitizeVirtualMachineForRestore(restore *harvesterv1.VirtualMachineRestore, spec kv1.VirtualMachineInstanceSpec) kv1.VirtualMachineInstanceSpec {
	for index, volume := range spec.Volumes {
		if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.UserDataSecretRef != nil {
			spec.Volumes[index].CloudInitNoCloud.UserDataSecretRef.Name = getVMUserDataSecretName(restore.Spec.Target.Name, volume.Name)
		}
		if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
			spec.Volumes[index].CloudInitNoCloud.NetworkDataSecretRef.Name = getVMNetworkDataSecretName(restore.Spec.Target.Name, volume.Name)
		}
	}
	return spec
}

// createRestoredPVC helps to create new PVC from CSI volumeSnapshot
func (h *RestoreHandler) createRestoredPVC(vmRestore *harvesterv1.VirtualMachineRestore,
	volumeBackup harvesterv1.VolumeBackup, volumeRestore harvesterv1.VolumeRestore) error {
	sourcePVC := volumeBackup.PersistentVolumeClaim
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        volumeRestore.PersistentVolumeClaim.ObjectMeta.Name,
			Namespace:   vmRestore.Namespace,
			Labels:      sourcePVC.ObjectMeta.Labels,
			Annotations: sourcePVC.ObjectMeta.Annotations,
		},
		Spec: *sourcePVC.Spec.DeepCopy(),
	}

	pvc.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion:         harvesterv1.SchemeGroupVersion.String(),
			Kind:               vmRestoreKindName,
			Name:               vmRestore.Name,
			UID:                vmRestore.UID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	})
	if volumeBackup.Name == nil {
		return fmt.Errorf("missing VolumeSnapshot name")
	}

	if pvc.Annotations == nil {
		pvc.Annotations = make(map[string]string)
	}

	for _, prefix := range restoreAnnotationsToDelete {
		for anno := range pvc.Annotations {
			if strings.HasPrefix(anno, prefix) {
				delete(pvc.Annotations, anno)
			}
		}
	}
	pvc.Annotations[restoreNameAnnotation] = vmRestore.Name
	pvc.Spec.DataSource = &corev1.TypedLocalObjectReference{
		APIGroup: pointer.StringPtr(snapshotv1.SchemeGroupVersion.Group),
		Kind:     volumeSnapshotKindName,
		Name:     *volumeBackup.Name,
	}
	pvc.Spec.VolumeName = ""

	_, err := h.pvcClient.Create(pvc)
	return err
}

func getRestoreID(vmRestore *harvesterv1.VirtualMachineRestore) string {
	return fmt.Sprintf("%s-%s", vmRestore.Name, vmRestore.UID)
}

func (h *RestoreHandler) doUpdateError(original, updated *harvesterv1.VirtualMachineRestore, err error, createEvent bool) error {
	if createEvent {
		h.recorder.Eventf(
			updated,
			corev1.EventTypeWarning,
			restoreErrorEvent,
			"VirtualMachineRestore encountered error %s",
			err.Error(),
		)
	}
	updateRestoreCondition(updated, newProgressingCondition(corev1.ConditionFalse, err.Error()))
	updateRestoreCondition(updated, newReadyCondition(corev1.ConditionFalse, err.Error()))
	if err2 := h.doUpdate(original, updated); err2 != nil {
		return err2
	}

	return err
}

func updateRestoreCondition(r *harvesterv1.VirtualMachineRestore, c harvesterv1.Condition) {
	r.Status.Conditions = updateCondition(r.Status.Conditions, c, true)
}

func (h *RestoreHandler) doUpdate(original, updated *harvesterv1.VirtualMachineRestore) error {
	if !reflect.DeepEqual(original, updated) {
		if _, err := h.restores.Update(updated); err != nil {
			return err
		}
	}

	return nil
}

func (h *RestoreHandler) enqueueAfter(original, updated *harvesterv1.VirtualMachineRestore, t time.Duration) {
	if !reflect.DeepEqual(original, updated) {
		h.restoreController.EnqueueAfter(updated.Namespace, updated.Name, t)
	}
}

func restorePVCName(vmRestore *harvesterv1.VirtualMachineRestore, name string) string {
	s := fmt.Sprintf("restore-%s-%s-%s", vmRestore.Spec.VirtualMachineBackupName, vmRestore.UID, name)
	return s
}

func vmRestoreProgressing(vmRestore *harvesterv1.VirtualMachineRestore) bool {
	return vmRestore.Status == nil || vmRestore.Status.Complete == nil || !*vmRestore.Status.Complete
}

func configVMOwner(vm *kv1.VirtualMachine) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         kv1.SchemeGroupVersion.String(),
			Kind:               kv1.VirtualMachineGroupVersionKind.Kind,
			Name:               vm.Name,
			UID:                vm.UID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	}
}

func getVMUserDataSecretName(vmName, volumeName string) string {
	return fmt.Sprintf("vm-%s-%s-userdata", vmName, volumeName)
}

func getVMNetworkDataSecretName(vmName, volumeName string) string {
	return fmt.Sprintf("vm-%s-%s-networkdata", vmName, volumeName)
}
