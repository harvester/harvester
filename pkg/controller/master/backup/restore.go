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
	lhv1beta1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/name"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

var (
	restoreAnnotationsToDelete = []string{
		"pv.kubernetes.io",
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
	vmNameLabel    = "harvesterhci.io/vmName"

	restoreErrorEvent    = "VirtualMachineRestoreError"
	restoreCompleteEvent = "VirtualMachineRestoreComplete"
)

type RestoreHandler struct {
	context context.Context

	restores             ctlharvesterv1.VirtualMachineRestoreClient
	restoreController    ctlharvesterv1.VirtualMachineRestoreController
	backupCache          ctlharvesterv1.VirtualMachineBackupCache
	vms                  ctlkubevirtv1.VirtualMachineClient
	vmCache              ctlkubevirtv1.VirtualMachineCache
	vmis                 ctlkubevirtv1.VirtualMachineInstanceClient
	vmiCache             ctlkubevirtv1.VirtualMachineInstanceCache
	pvcClient            ctlcorev1.PersistentVolumeClaimClient
	pvcCache             ctlcorev1.PersistentVolumeClaimCache
	secretClient         ctlcorev1.SecretClient
	secretCache          ctlcorev1.SecretCache
	snapshots            ctlsnapshotv1.VolumeSnapshotClient
	snapshotCache        ctlsnapshotv1.VolumeSnapshotCache
	snapshotContents     ctlsnapshotv1.VolumeSnapshotContentClient
	snapshotContentCache ctlsnapshotv1.VolumeSnapshotContentCache
	lhbackupCache        ctllonghornv1.BackupCache
	volumeCache          ctllonghornv1.VolumeCache
	volumes              ctllonghornv1.VolumeClient

	recorder   record.EventRecorder
	restClient *rest.RESTClient
}

func RegisterRestore(ctx context.Context, management *config.Management, opts config.Options) error {
	restores := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore()
	backups := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	secrets := management.CoreFactory.Core().V1().Secret()
	snapshots := management.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshot()
	snapshotContents := management.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshotContent()
	lhbackups := management.LonghornFactory.Longhorn().V1beta1().Backup()
	volumes := management.LonghornFactory.Longhorn().V1beta1().Volume()

	copyConfig := rest.CopyConfig(management.RestConfig)
	copyConfig.GroupVersion = &k8sschema.GroupVersion{Group: kubevirtv1.SubresourceGroupName, Version: kubevirtv1.ApiLatestVersion}
	copyConfig.APIPath = "/apis"
	copyConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	restClient, err := rest.RESTClientFor(copyConfig)
	if err != nil {
		return err
	}

	handler := &RestoreHandler{
		context:              ctx,
		restores:             restores,
		restoreController:    restores,
		backupCache:          backups.Cache(),
		vms:                  vms,
		vmCache:              vms.Cache(),
		vmis:                 vmis,
		vmiCache:             vmis.Cache(),
		pvcClient:            pvcs,
		pvcCache:             pvcs.Cache(),
		secretClient:         secrets,
		secretCache:          secrets.Cache(),
		snapshots:            snapshots,
		snapshotCache:        snapshots.Cache(),
		snapshotContents:     snapshotContents,
		snapshotContentCache: snapshotContents.Cache(),
		lhbackupCache:        lhbackups.Cache(),
		volumes:              volumes,
		volumeCache:          volumes.Cache(),
		recorder:             management.NewRecorder(restoreControllerName, "", ""),
		restClient:           restClient,
	}

	restores.OnChange(ctx, restoreControllerName, handler.RestoreOnChanged)
	restores.OnRemove(ctx, restoreControllerName, handler.RestoreOnRemove)
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
		if err := h.mountLonghornVolumes(backup); err != nil {
			return nil, h.updateStatusError(restore, err, true)
		}
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

// RestoreOnRemove delete VolumeSnapshotContent which is created by restore controller
// Since we would like to prevent LH Backups from being removed when users delete the VM,
// we use Retain policy in VolumeSnapshotContent.
// We need to delete VolumeSnapshotContent by restore controller, or there will have remaining VolumeSnapshotContent in the system.
func (h *RestoreHandler) RestoreOnRemove(key string, restore *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	if restore == nil || restore.Status == nil {
		return nil, nil
	}

	backup, err := h.getVMBackup(restore)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, nil
	}

	for _, volumeBackup := range backup.Status.VolumeBackups {
		if volumeSnapshotContent, err := h.snapshotContentCache.Get(h.constructVolumeSnapshotContentName(restore.Namespace, restore.Name, *volumeBackup.Name)); err != nil {
			if err != nil && !apierrors.IsNotFound(err) {
				return nil, err
			}
		} else {
			if err = h.snapshotContents.Delete(volumeSnapshotContent.Name, &metav1.DeleteOptions{}); err != nil {
				return nil, err
			}
		}
	}

	return nil, nil
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
func (h *RestoreHandler) VMOnChange(key string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
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
		Conditions: []harvesterv1.Condition{
			newProgressingCondition(corev1.ConditionTrue, "", "Initializing VirtualMachineRestore"),
			newReadyCondition(corev1.ConditionFalse, "", "Initializing VirtualMachineRestore"),
		},
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
func (h *RestoreHandler) getVM(vmRestore *harvesterv1.VirtualMachineRestore) (*kubevirtv1.VirtualMachine, error) {
	switch vmRestore.Spec.Target.Kind {
	case kubevirtv1.VirtualMachineGroupVersionKind.Kind:
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
) (*kubevirtv1.VirtualMachine, bool, error) {
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
			volumeBackup := backup.Status.VolumeBackups[i]
			if err = h.createRestoredPVC(vmRestore, volumeBackup, volumeRestore); err != nil {
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
) (*kubevirtv1.VirtualMachine, error) {
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
	vm *kubevirtv1.VirtualMachine,
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
		newSecretName := getSecretRefName(vmRestore.Spec.Target.Name, secretBackup.Name)
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

	if !IsBackupReady(backup) {
		return nil, fmt.Errorf("VMBackup %s is not ready", backup.Name)
	}

	if backup.Status.SourceSpec == nil {
		return nil, fmt.Errorf("empty vm backup source spec of %s", backup.Name)
	}

	return backup, nil
}

// createNewVM helps to create new target VM and set the associated owner reference
func (h *RestoreHandler) createNewVM(restore *harvesterv1.VirtualMachineRestore, backup *harvesterv1.VirtualMachineBackup) (*kubevirtv1.VirtualMachine, error) {
	vmName := restore.Spec.Target.Name
	logrus.Infof("restore target does not exist, creating a new vm %s", vmName)

	restoreID := getRestoreID(restore)
	vmCpy := backup.Status.SourceSpec.DeepCopy()

	newVMAnnotations := map[string]string{
		lastRestoreAnnotation: restoreID,
		restoreNameAnnotation: restore.Name,
	}
	if reservedMem, ok := vmCpy.ObjectMeta.Annotations[util.AnnotationReservedMemory]; ok {
		newVMAnnotations[util.AnnotationReservedMemory] = reservedMem
	}

	newVMSpecAnnotations, err := sanitizeVirtualMachineAnnotationsForRestore(restore, vmCpy.Spec.Template.ObjectMeta.Annotations)
	if err != nil {
		return nil, err
	}

	defaultRunStrategy := kubevirtv1.RunStrategyRerunOnFailure
	if backup.Status.SourceSpec.Spec.RunStrategy != nil {
		defaultRunStrategy = *backup.Status.SourceSpec.Spec.RunStrategy
	}

	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        vmName,
			Namespace:   restore.Namespace,
			Annotations: newVMAnnotations,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: &defaultRunStrategy,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: newVMSpecAnnotations,
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

func (h *RestoreHandler) updateOwnerRefAndTargetUID(vmRestore *harvesterv1.VirtualMachineRestore, vm *kubevirtv1.VirtualMachine) error {
	restoreCpy := vmRestore.DeepCopy()
	if restoreCpy.Status.TargetUID == nil {
		restoreCpy.Status.TargetUID = &vm.UID
	}

	// set vmRestore owner reference to the target VM
	restoreCpy.SetOwnerReferences(configVMOwner(vm))

	if _, err := h.restores.Update(restoreCpy); err != nil {
		return err
	}

	return nil
}

// createRestoredPVC helps to create new PVC from CSI volumeSnapshot
func (h *RestoreHandler) createRestoredPVC(
	vmRestore *harvesterv1.VirtualMachineRestore,
	volumeBackup harvesterv1.VolumeBackup,
	volumeRestore harvesterv1.VolumeRestore,
) error {
	if volumeBackup.Name == nil {
		return fmt.Errorf("missing VolumeSnapshot name")
	}

	dataSourceName := *volumeBackup.Name
	if vmRestore.Namespace != volumeBackup.PersistentVolumeClaim.ObjectMeta.Namespace {
		// create volumesnapshot if namespace is different
		volumeSnapshot, err := h.getOrCreateVolumeSnapshot(vmRestore, volumeBackup)
		if err != nil {
			return err
		}
		dataSourceName = volumeSnapshot.Name
	}

	annotations := map[string]string{}
	for key, value := range volumeBackup.PersistentVolumeClaim.ObjectMeta.Annotations {
		needSkip := false
		for _, prefix := range restoreAnnotationsToDelete {
			if strings.HasPrefix(key, prefix) {
				needSkip = true
				break
			}
		}
		if !needSkip {
			annotations[key] = value
		}
	}
	annotations[restoreNameAnnotation] = vmRestore.Name

	_, err := h.pvcClient.Create(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        volumeRestore.PersistentVolumeClaim.ObjectMeta.Name,
			Namespace:   vmRestore.Namespace,
			Labels:      volumeBackup.PersistentVolumeClaim.ObjectMeta.Labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         harvesterv1.SchemeGroupVersion.String(),
					Kind:               vmRestoreKindName,
					Name:               vmRestore.Name,
					UID:                vmRestore.UID,
					Controller:         pointer.BoolPtr(true),
					BlockOwnerDeletion: pointer.BoolPtr(true),
				},
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: volumeBackup.PersistentVolumeClaim.Spec.AccessModes,
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: pointer.StringPtr(snapshotv1.SchemeGroupVersion.Group),
				Kind:     volumeSnapshotKindName,
				Name:     dataSourceName,
			},
			Resources:        volumeBackup.PersistentVolumeClaim.Spec.Resources,
			StorageClassName: volumeBackup.PersistentVolumeClaim.Spec.StorageClassName,
			VolumeMode:       volumeBackup.PersistentVolumeClaim.Spec.VolumeMode,
		},
	})
	return err
}

func (h *RestoreHandler) getOrCreateVolumeSnapshotContent(
	vmRestore *harvesterv1.VirtualMachineRestore,
	volumeBackup harvesterv1.VolumeBackup,
) (*snapshotv1.VolumeSnapshotContent, error) {
	volumeSnapshotContentName := h.constructVolumeSnapshotContentName(vmRestore.Namespace, vmRestore.Name, *volumeBackup.Name)
	if volumeSnapshotContent, err := h.snapshotContentCache.Get(volumeSnapshotContentName); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
	} else {
		return volumeSnapshotContent, nil
	}

	lhBackup, err := h.lhbackupCache.Get(util.LonghornSystemNamespaceName, *volumeBackup.LonghornBackupName)
	if err != nil {
		return nil, err
	}
	// Ref: https://longhorn.io/docs/1.2.3/snapshots-and-backups/csi-snapshot-support/restore-a-backup-via-csi/#restore-a-backup-that-has-no-associated-volumesnapshot
	snapshotHandle := fmt.Sprintf("bs://%s/%s", volumeBackup.PersistentVolumeClaim.ObjectMeta.Name, lhBackup.Name)

	logrus.Debugf("create VolumeSnapshotContent %s ...", volumeSnapshotContentName)
	return h.snapshotContents.Create(&snapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      volumeSnapshotContentName,
			Namespace: vmRestore.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: harvesterv1.SchemeGroupVersion.String(),
					Kind:       vmRestoreKindName,
					Name:       vmRestore.Name,
					UID:        vmRestore.UID,
				},
			},
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver: "driver.longhorn.io",
			// Use Retain policy to prevent LH Backup from being removed when users delete a VM.
			DeletionPolicy: snapshotv1.VolumeSnapshotContentRetain,
			Source: snapshotv1.VolumeSnapshotContentSource{
				SnapshotHandle: pointer.StringPtr(snapshotHandle),
			},
			VolumeSnapshotClassName: pointer.StringPtr(settings.VolumeSnapshotClass.Get()),
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      h.constructVolumeSnapshotName(vmRestore.Name, *volumeBackup.Name),
				Namespace: vmRestore.Namespace,
			},
		},
	})
}

func (h *RestoreHandler) getOrCreateVolumeSnapshot(
	vmRestore *harvesterv1.VirtualMachineRestore,
	volumeBackup harvesterv1.VolumeBackup,
) (*snapshotv1.VolumeSnapshot, error) {
	volumeSnapshotName := h.constructVolumeSnapshotName(vmRestore.Name, *volumeBackup.Name)
	if volumeSnapshot, err := h.snapshotCache.Get(vmRestore.Namespace, volumeSnapshotName); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
	} else {
		return volumeSnapshot, nil
	}

	volumeSnapshotContent, err := h.getOrCreateVolumeSnapshotContent(vmRestore, volumeBackup)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("create VolumeSnapshot %s/%s", vmRestore.Namespace, volumeSnapshotName)
	return h.snapshots.Create(&snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      volumeSnapshotName,
			Namespace: vmRestore.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         harvesterv1.SchemeGroupVersion.String(),
					Kind:               vmRestoreKindName,
					Name:               vmRestore.Name,
					UID:                vmRestore.UID,
					BlockOwnerDeletion: pointer.BoolPtr(true),
				},
			},
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				VolumeSnapshotContentName: pointer.StringPtr(volumeSnapshotContent.Name),
			},
			VolumeSnapshotClassName: pointer.StringPtr(settings.VolumeSnapshotClass.Get()),
		},
	})
}

func (h *RestoreHandler) deleteOldPVC(vmRestore *harvesterv1.VirtualMachineRestore, vm *kubevirtv1.VirtualMachine) error {
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

func (h *RestoreHandler) startVM(vm *kubevirtv1.VirtualMachine) error {
	runStrategy, err := vm.RunStrategy()
	if err != nil {
		return err
	}

	logrus.Infof("starting the vm %s, current state running:%v", vm.Name, runStrategy)
	switch runStrategy {
	case kubevirtv1.RunStrategyAlways, kubevirtv1.RunStrategyRerunOnFailure:
		return nil
	case kubevirtv1.RunStrategyManual:
		if vmi, err := h.vmiCache.Get(vm.Namespace, vm.Name); err == nil {
			if vmi != nil && !vmi.IsFinal() && vmi.Status.Phase != kubevirtv1.Unknown && vmi.Status.Phase != kubevirtv1.VmPhaseUnset {
				// vm is already running
				return nil
			}
		}
		return h.restClient.Put().Namespace(vm.Namespace).Resource("virtualmachines").SubResource("start").Name(vm.Name).Do(h.context).Error()
	case kubevirtv1.RunStrategyHalted:
		return h.restClient.Put().Namespace(vm.Namespace).Resource("virtualmachines").SubResource("start").Name(vm.Name).Do(h.context).Error()
	default:
		// skip
	}
	return nil
}

func (h *RestoreHandler) updateStatus(
	vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup,
	vm *kubevirtv1.VirtualMachine,
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

	// start VM before checking status
	if err := h.startVM(vm); err != nil {
		return h.updateStatusError(vmRestore, fmt.Errorf("failed to start vm, err:%s", err.Error()), false)
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

func (h *RestoreHandler) constructVolumeSnapshotName(restoreName, volumeBackupName string) string {
	return name.SafeConcatName("restore", restoreName, volumeBackupName)
}

func (h *RestoreHandler) constructVolumeSnapshotContentName(restoreNamespace, restoreName, volumeBackupName string) string {
	// VolumeSnapshotContent is cluster-scoped resource,
	// so adding restoreNamespace to its name to prevent conflict in different namespace with same restore name and backup
	return name.SafeConcatName("restore", restoreNamespace, restoreName, volumeBackupName)
}

// mountLonghornVolumes helps to mount the volumes to host if it is detached
func (h *RestoreHandler) mountLonghornVolumes(backup *harvesterv1.VirtualMachineBackup) error {
	// we only need to mount LH Volumes for snapshot type.
	if backup.Spec.Type == harvesterv1.Backup {
		return nil
	}

	for _, vb := range backup.Status.VolumeBackups {
		pvcNamespace := vb.PersistentVolumeClaim.ObjectMeta.Namespace
		pvcName := vb.PersistentVolumeClaim.ObjectMeta.Name

		pvc, err := h.pvcCache.Get(pvcNamespace, pvcName)
		if err != nil {
			return fmt.Errorf("failed to get pvc %s/%s, error: %s", pvcNamespace, pvcName, err.Error())
		}

		volume, err := h.volumeCache.Get(util.LonghornSystemNamespaceName, pvc.Spec.VolumeName)
		if err != nil {
			return fmt.Errorf("failed to get volume %s/%s, error: %s", util.LonghornSystemNamespaceName, pvc.Spec.VolumeName, err.Error())
		}

		volCpy := volume.DeepCopy()
		if volume.Status.State == lhv1beta1.VolumeStateDetached || volume.Status.State == lhv1beta1.VolumeStateDetaching {
			volCpy.Spec.NodeID = volume.Status.OwnerID
		}

		if !reflect.DeepEqual(volCpy, volume) {
			logrus.Infof("mount detached volume %s to the node %s", volCpy.Name, volCpy.Spec.NodeID)
			if _, err = h.volumes.Update(volCpy); err != nil {
				return err
			}
		}
	}
	return nil
}
