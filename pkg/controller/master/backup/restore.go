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
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	kv1 "kubevirt.io/client-go/api/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
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

	copyConfig := rest.CopyConfig(management.RestConfig)
	copyConfig.GroupVersion = &k8sschema.GroupVersion{Group: kv1.SubresourceGroupName, Version: kv1.ApiLatestVersion}
	copyConfig.APIPath = "/apis"
	copyConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	restClient, err := rest.RESTClientFor(copyConfig)
	if err != nil {
		return err
	}

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
		restClient:        restClient,
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

	if !isVMRestoreProgressing(restore) {
		return nil, nil
	}

	if restore.Status == nil {
		return nil, h.initStatus(restore)
	}

	backup, err := h.getVMBackup(restore)
	if err != nil {
		return nil, h.updateStatusError(restore, err, true)
	}

	if isVMRestoreMissingVolumes(restore) {
		return nil, h.initVolumesStatus(restore, backup)
	}

	vm, isVolumesReady, err := h.reconcileResources(restore, backup)
	if err != nil {
		return nil, h.updateStatusError(restore, err, true)
	}

	// set vmRestore owner reference to the target VM
	if len(restore.OwnerReferences) == 0 {
		return nil, h.updateOwnerRefAndTargetUID(restore, vm)
	}

	return nil, h.updateStatus(restore, backup, vm, isVolumesReady)
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

func (h *RestoreHandler) initStatus(restore *harvesterv1.VirtualMachineRestore) error {
	restoreCpy := restore.DeepCopy()

	restoreCpy.Status = &harvesterv1.VirtualMachineRestoreStatus{
		Complete: pointer.BoolPtr(false),
	}

	if _, err := h.restores.Update(restoreCpy); err != nil {
		return err
	}
	return nil
}

func (h *RestoreHandler) initVolumesStatus(vmRestore *harvesterv1.VirtualMachineRestore, backup *harvesterv1.VirtualMachineBackup) error {
	restoreCpy := vmRestore.DeepCopy()

	if restoreCpy.Status.VolumeRestores == nil {
		volumeRestores, err := getVolumeRestores(restoreCpy, backup)
		if err != nil {
			return err
		}
		restoreCpy.Status.VolumeRestores = volumeRestores
	}

	if !isNewVMOrHasRetainPolicy(vmRestore) && vmRestore.Status.DeletedVolumes == nil {
		var deletedVolumes []string
		for _, vol := range backup.Status.VolumeBackups {
			deletedVolumes = append(deletedVolumes, vol.PersistentVolumeClaim.ObjectMeta.Name)
		}
		restoreCpy.Status.DeletedVolumes = deletedVolumes
	}

	if _, err := h.restores.Update(restoreCpy); err != nil {
		return err
	}
	return nil
}

// getVM returns restore target VM
func (h *RestoreHandler) getVM(vmRestore *harvesterv1.VirtualMachineRestore) (*kv1.VirtualMachine, error) {
	switch vmRestore.Spec.Target.Kind {
	case kv1.VirtualMachineGroupVersionKind.Kind:
		vm, err := h.vmCache.Get(vmRestore.Namespace, vmRestore.Spec.Target.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}

		return vm, nil
	}

	return nil, fmt.Errorf("unknown target %+v", vmRestore.Spec.Target)
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
						Name:      getRestorePVCName(vmRestore, vb.VolumeName),
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

func (h *RestoreHandler) reconcileResources(
	vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup,
) (*kv1.VirtualMachine, bool, error) {
	// reconcile restoring volumes and create new PVC from CSI volumeSnapshot if not exist
	isVolumesReady, err := h.reconcileVolumeRestores(vmRestore, backup)
	if err != nil {
		return nil, false, err
	}

	// reconcile VM
	vm, err := h.reconcileVM(vmRestore, backup)
	if err != nil {
		return nil, false, err
	}

	//restore referenced secrets
	if err := h.reconcileSecretBackups(vmRestore, backup, vm); err != nil {
		return nil, false, err
	}

	return vm, isVolumesReady, nil
}

func (h *RestoreHandler) reconcileVolumeRestores(
	vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup,
) (bool, error) {
	isVolumesReady := true
	for i, volumeRestore := range vmRestore.Status.VolumeRestores {
		pvc, err := h.pvcCache.Get(vmRestore.Namespace, volumeRestore.PersistentVolumeClaim.ObjectMeta.Name)
		if apierrors.IsNotFound(err) {
			backup := backup.Status.VolumeBackups[i]
			if err = h.createRestoredPVC(vmRestore, backup, volumeRestore); err != nil {
				return false, err
			}
			isVolumesReady = false
			continue
		}
		if err != nil {
			return false, err
		}

		if pvc.Status.Phase == corev1.ClaimPending {
			isVolumesReady = false
		} else if pvc.Status.Phase != corev1.ClaimBound {
			return false, fmt.Errorf("PVC %s/%s in status %q", pvc.Namespace, pvc.Name, pvc.Status.Phase)
		}
	}
	return isVolumesReady, nil
}

func (h *RestoreHandler) reconcileVM(
	vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup,
) (*kv1.VirtualMachine, error) {
	// create new VM if it's not exist
	vm, err := h.getVM(vmRestore)
	if err != nil {
		return nil, err
	} else if vm == nil && err == nil {
		vm, err = h.createNewVM(vmRestore, backup)
		if err != nil {
			return nil, err
		}
	}

	// make sure target VM has correct annotations
	restoreID := getRestoreID(vmRestore)
	if lastRestoreID, ok := vm.Annotations[lastRestoreAnnotation]; ok && lastRestoreID == restoreID {
		return vm, nil
	}

	// VM doesn't have correct annotations like restore to existing VM.
	// We update its volumes to new reconsile volumes
	newVolumes, err := getNewVolumes(&backup.Status.SourceSpec.Spec, vmRestore)
	if err != nil {
		return nil, err
	}

	vmCpy := vm.DeepCopy()
	vmCpy.Spec = backup.Status.SourceSpec.Spec
	vmCpy.Spec.Template.Spec.Volumes = newVolumes
	if vmCpy.Annotations == nil {
		vmCpy.Annotations = make(map[string]string)
	}
	vmCpy.Annotations[lastRestoreAnnotation] = restoreID
	vmCpy.Annotations[restoreNameAnnotation] = vmRestore.Name
	delete(vmCpy.Annotations, util.AnnotationVolumeClaimTemplates)

	if vm, err = h.vms.Update(vmCpy); err != nil {
		return nil, err
	}

	return vm, nil
}

func (h *RestoreHandler) reconcileSecretBackups(
	vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup,
	vm *kv1.VirtualMachine,
) error {
	ownerRefs := configVMOwner(vm)
	if !vmRestore.Spec.NewVM {
		for _, secretBackup := range backup.Status.SecretBackups {
			if err := h.createOrUpdateSecret(vmRestore.Namespace, secretBackup.Name, secretBackup.Data, ownerRefs); err != nil {
				return err
			}
		}
		return nil
	}

	// Create new secret for new VM
	for _, secretBackup := range backup.Status.SecretBackups {
		newSecretName := getCloudInitSecretRefVolumeName(vmRestore.Spec.Target.Name, secretBackup.Name)
		if err := h.createOrUpdateSecret(vmRestore.Namespace, newSecretName, secretBackup.Data, ownerRefs); err != nil {
			return err
		}
	}
	return nil
}

func (h *RestoreHandler) createOrUpdateSecret(namespace, name string, data map[string][]byte, ownerRefs []metav1.OwnerReference) error {
	secret, err := h.secretCache.Get(namespace, name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		logrus.Infof("create secret %s/%s", namespace, name)
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       namespace,
				OwnerReferences: ownerRefs,
			},
			Data: data,
		}
		if _, err := h.secretClient.Create(secret); err != nil {
			return err
		}
		return nil
	}

	secretCpy := secret.DeepCopy()
	secretCpy.Data = data

	if !reflect.DeepEqual(secret, secretCpy) {
		logrus.Infof("update secret %s/%s", namespace, name)
		if _, err := h.secretClient.Update(secretCpy); err != nil {
			return err
		}
	}
	return nil
}

func (h *RestoreHandler) getVMBackup(vmRestore *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineBackup, error) {
	backup, err := h.backupCache.Get(vmRestore.Spec.VirtualMachineBackupNamespace, vmRestore.Spec.VirtualMachineBackupName)
	if err != nil {
		return nil, err
	}

	if !isBackupReady(backup) {
		return nil, fmt.Errorf("VMBackup %s is not ready", backup.Name)
	}

	if backup.Status.SourceSpec == nil {
		return nil, fmt.Errorf("empty vm backup source spec of %s", backup.Name)
	}

	return backup, nil
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

	newVolumes, err := getNewVolumes(&vm.Spec, restore)
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

	return newVM, nil
}

func (h *RestoreHandler) updateOwnerRefAndTargetUID(vmRestore *harvesterv1.VirtualMachineRestore, vm *kv1.VirtualMachine) error {
	restoreCpy := vmRestore.DeepCopy()
	if restoreCpy.Status.TargetUID == nil {
		restoreCpy.Status.TargetUID = &vm.UID
	}

	// set vmRestore owner reference to the target VM
	restoreCpy.SetOwnerReferences(configVMOwner(vm))
	updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "", "Initializing VirtualMachineRestore"))
	updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "", "Initializing VirtualMachineRestore"))

	if _, err := h.restores.Update(restoreCpy); err != nil {
		return err
	}

	return nil
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

func (h *RestoreHandler) deleteOldPVC(vmRestore *harvesterv1.VirtualMachineRestore, vm *kv1.VirtualMachine) error {
	if isNewVMOrHasRetainPolicy(vmRestore) {
		logrus.Infof("skip deleting old PVC of vm %s/%s", vm.Name, vm.Namespace)
		return nil
	}

	// clean up existing pvc
	for _, volName := range vmRestore.Status.DeletedVolumes {
		vol, err := h.pvcCache.Get(vmRestore.Namespace, volName)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		if vol != nil {
			err = h.pvcClient.Delete(vol.Namespace, vol.Name, &metav1.DeleteOptions{})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (h *RestoreHandler) startVM(vm *kv1.VirtualMachine) error {
	logrus.Infof("starting the vm %s, current state running:%v", vm.Name, *vm.Spec.Running)
	if vm.Spec.Running == nil || !*vm.Spec.Running {
		return h.restClient.Put().Namespace(vm.Namespace).Resource("virtualmachines").SubResource("start").Name(vm.Name).Do(h.context).Error()
	}
	return nil
}

func (h *RestoreHandler) updateStatus(
	vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup,
	vm *kv1.VirtualMachine,
	isVolumesReady bool,
) error {
	restoreCpy := vmRestore.DeepCopy()
	if !isVolumesReady {
		updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "", "Creating new PVCs"))
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "", "Waiting for new PVCs"))
		if !reflect.DeepEqual(vmRestore, restoreCpy) {
			if _, err := h.restores.Update(restoreCpy); err != nil {
				return err
			}
		}
		return nil
	}

	if !vm.Status.Ready {
		message := "Waiting for target vm to be ready"
		updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionFalse, "", message))
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "", message))
		if !reflect.DeepEqual(vmRestore, restoreCpy) {
			if _, err := h.restores.Update(restoreCpy); err != nil {
				return err
			}
		}
		return nil
	}

	if err := h.deleteOldPVC(restoreCpy, vm); err != nil {
		return h.updateStatusError(vmRestore, fmt.Errorf("error cleaning up, err:%s", err.Error()), false)
	}

	if err := h.startVM(vm); err != nil {
		return h.updateStatusError(vmRestore, fmt.Errorf("failed to start vm, err:%s", err.Error()), false)
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
	updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionFalse, "", "Operation complete"))
	updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionTrue, "", "Operation complete"))
	if _, err := h.restores.Update(restoreCpy); err != nil {
		return err
	}
	return nil
}

func (h *RestoreHandler) updateStatusError(restore *harvesterv1.VirtualMachineRestore, err error, createEvent bool) error {
	restoreCpy := restore.DeepCopy()
	updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionFalse, "Error", err.Error()))
	updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "Error", err.Error()))

	if !reflect.DeepEqual(restore, restoreCpy) {
		if createEvent {
			h.recorder.Eventf(
				restoreCpy,
				corev1.EventTypeWarning,
				restoreErrorEvent,
				"VirtualMachineRestore encountered error %s",
				err.Error(),
			)
		}

		if _, err2 := h.restores.Update(restoreCpy); err2 != nil {
			return err2
		}
	}

	return err
}
