package backup

// NOTE: Harvester VM backup & restore is referenced to the Kubevirt's VM snapshot & restore,
// currently, we have decided to use custom VM backup and restore controller because of the following issues:
// 1. live VM snapshot/backup is supported using Harvester's build-in block storage with longhorn.
// 2. restore a VM backup to a new VM should be supported.
import (
	"context"
	"fmt"
	"reflect"

	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	kutil "k8s.io/utils/pointer"
	kv1 "kubevirt.io/client-go/api/v1"

	harvesterapiv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	ctlharvesterv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	ctlkubevirtv1 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/rancher/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
)

const (
	backupControllerName = "harvester-vm-backup-controller"
	vmBackupKindName     = "VirtualMachineBackup"
	vmKindName           = "VirtualMachine"

	backupTargetAnnotation = "backup.harvester.io/backupTarget"
)

var vmBackupKind = harvesterapiv1.SchemeGroupVersion.WithKind(vmBackupKindName)

// RegisterBackup register the vmbackup and vmbackupcontent controller
func RegisterBackup(ctx context.Context, management *config.Management, opts config.Options) error {
	vmBackups := management.HarvesterFactory.Harvester().V1alpha1().VirtualMachineBackup()
	vmBackupContents := management.HarvesterFactory.Harvester().V1alpha1().VirtualMachineBackupContent()
	pvc := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	longhornSettings := management.LonghornFactory.Longhorn().V1beta1().Setting()

	vmBackupController := &Handler{
		vmBackups:            vmBackups,
		vmBackupController:   vmBackups,
		vmBackupCache:        vmBackups.Cache(),
		vmBackupContents:     vmBackupContents,
		vmBackupContentCache: vmBackupContents.Cache(),
		pvcCache:             pvc.Cache(),
		longhornSettingCache: longhornSettings.Cache(),
		vmCache:              vms.Cache(),
	}

	vmBackups.OnChange(ctx, backupControllerName, vmBackupController.OnBackupChange)
	vmBackupContents.OnChange(ctx, backupControllerName, vmBackupController.updateContentChanged)
	return nil
}

type Handler struct {
	vmBackups            ctlharvesterv1.VirtualMachineBackupClient
	vmBackupCache        ctlharvesterv1.VirtualMachineBackupCache
	vmBackupController   ctlharvesterv1.VirtualMachineBackupController
	vmBackupContentCache ctlharvesterv1.VirtualMachineBackupContentCache
	vmBackupContents     ctlharvesterv1.VirtualMachineBackupContentClient
	pvcCache             ctlcorev1.PersistentVolumeClaimCache
	longhornSettingCache ctllonghornv1.SettingCache
	vmCache              ctlkubevirtv1.VirtualMachineCache
}

// OnBackupChange handles vm backup object on change and create vmbackup content if not exist
func (h *Handler) OnBackupChange(key string, vmBackup *harvesterapiv1.VirtualMachineBackup) (*harvesterapiv1.VirtualMachineBackup, error) {
	if vmBackup == nil || vmBackup.DeletionTimestamp != nil {
		return nil, nil
	}

	if vmBackupReady(vmBackup) {
		return nil, nil
	}

	if vmBackup.Status == nil {
		return nil, h.updateStatus(vmBackup, nil)
	}

	// get vmBackup source
	source, err := h.getBackupSource(vmBackup)
	if err != nil {
		return nil, err
	}

	// make sure status is initialized, and "Lock" the source VM by adding a finalizer and setting snapshotInProgress in status
	if source != nil && vmBackupProgressing(vmBackup) {

		// Create VirtualMachineSnapshotContent containing VM spec, specs of any PVCs that are snapshot-able, and VolumeSnapshot names.
		content, err := h.getContent(vmBackup)
		if err != nil {
			return nil, err
		}

		// create content if does not exist
		if content == nil {
			if err = h.createContent(vmBackup, source.DeepCopy()); err != nil {
				return nil, err
			}
		}

		// update status
		if err := h.updateStatus(vmBackup, source); err != nil {
			return vmBackup, err
		}
	}
	return nil, nil
}

func (h *Handler) getBackupSource(vmBackup *harvesterapiv1.VirtualMachineBackup) (*kv1.VirtualMachine, error) {
	switch vmBackup.Spec.Source.Kind {
	case vmKindName:
		vm, err := h.vmCache.Get(vmBackup.Namespace, vmBackup.Spec.Source.Name)
		if err != nil {
			return nil, err
		}
		return vm, nil
	}

	return nil, fmt.Errorf("unsupported source: %+v", vmBackup.Spec.Source)
}

func (h *Handler) getContent(vmBackup *harvesterapiv1.VirtualMachineBackup) (*harvesterapiv1.VirtualMachineBackupContent, error) {
	contentName := getVMBackupContentName(vmBackup)
	obj, err := h.vmBackupContentCache.Get(vmBackup.Namespace, contentName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return obj, nil
}

func (h *Handler) createContent(vmBackup *harvesterapiv1.VirtualMachineBackup, vm *kv1.VirtualMachine) error {
	contentName := getVMBackupContentName(vmBackup)
	logrus.Debugf("create vm backup content %s/%s", vmBackup.Namespace, contentName)

	sourceVolumes := vm.Spec.Template.Spec.Volumes
	var volumeBackups = make([]harvesterapiv1.VolumeBackup, 0, len(sourceVolumes))

	for volumeName, pvcName := range volumeToPvcMappings(sourceVolumes) {
		pvc, err := h.getBackupPVC(vmBackup.Namespace, pvcName)
		if err != nil {
			return err
		}

		volumeBackupName := fmt.Sprintf("%s-volume-%s", vmBackup.Name, pvcName)

		vb := harvesterapiv1.VolumeBackup{
			Name:       &volumeBackupName,
			VolumeName: volumeName,
			PersistentVolumeClaim: harvesterapiv1.PersistentVolumeClaimSpec{
				Name:      pvc.ObjectMeta.Name,
				Namespace: pvc.ObjectMeta.Namespace,
				Spec:      pvc.Spec,
			},
		}

		volumeBackups = append(volumeBackups, vb)
	}

	content := &harvesterapiv1.VirtualMachineBackupContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      contentName,
			Namespace: vmBackup.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:               vmBackup.Name,
					Kind:               vmBackup.Kind,
					UID:                vmBackup.UID,
					APIVersion:         vmBackup.APIVersion,
					Controller:         kutil.BoolPtr(true),
					BlockOwnerDeletion: kutil.BoolPtr(true),
				},
			},
		},
		Spec: harvesterapiv1.VirtualMachineBackupContentSpec{
			VirtualMachineBackupName: &vmBackup.Name,
			Source: harvesterapiv1.SourceSpec{
				Name:               vm.ObjectMeta.Name,
				Namespace:          vm.ObjectMeta.Namespace,
				VirtualMachineSpec: &vm.Spec,
			},
			VolumeBackups: volumeBackups,
		},
	}

	if _, err := h.vmBackupContents.Create(content); err != nil {
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

func (h *Handler) updateStatus(vmBackup *harvesterapiv1.VirtualMachineBackup, source *kv1.VirtualMachine) error {
	var vmBackupCpy = vmBackup.DeepCopy()
	if vmBackupCpy.Status == nil {
		vmBackupCpy.Status = &harvesterapiv1.VirtualMachineBackupStatus{
			ReadyToUse: pointer.BoolPtr(false),
		}
	}

	if source != nil {
		vmBackupCpy.Status.SourceUID = &source.UID
	}

	content, err := h.getContent(vmBackupCpy)
	if err != nil {
		return err
	}
	if content != nil && content.Status != nil {
		// content exists and is initialized
		vmBackupCpy.Status.VirtualMachineBackupContentName = &content.Name
		vmBackupCpy.Status.ReadyToUse = content.Status.ReadyToUse
		vmBackupCpy.Status.Error = content.Status.Error
	}

	if vmBackupProgressing(vmBackupCpy) {
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
	} else if vmBackupReady(vmBackupCpy) {
		updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionFalse, "Operation complete"))
		updateBackupCondition(vmBackupCpy, newReadyCondition(corev1.ConditionTrue, "Operation complete"))
	} else {
		updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionUnknown, "Unknown state"))
		updateBackupCondition(vmBackupCpy, newReadyCondition(corev1.ConditionUnknown, "Unknown state"))
	}

	if vmBackup.Annotations == nil {
		vmBackup.Annotations = make(map[string]string)
	}

	if vmBackup.Annotations[backupTargetAnnotation] == "" {
		target, err := h.longhornSettingCache.Get(LonghornSystemNameSpace, longhornBackupTargetSettingName)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if target != nil && target.Value != "" {
			vmBackup.Annotations[backupTargetAnnotation] = target.Value
		}
	}

	if !reflect.DeepEqual(vmBackupCpy, vmBackup) {
		if vmBackupCpy.Status != nil {
			if _, err := h.vmBackups.Update(vmBackupCpy); err != nil {
				return err
			}
		}
	}
	return nil
}

func volumeToPvcMappings(volumes []kv1.Volume) map[string]string {
	pvcs := map[string]string{}

	for _, volume := range volumes {
		var pvcName string

		if volume.PersistentVolumeClaim != nil {
			pvcName = volume.PersistentVolumeClaim.ClaimName
		} else if volume.DataVolume != nil {
			pvcName = volume.DataVolume.Name
		} else {
			continue
		}

		pvcs[volume.Name] = pvcName
	}

	return pvcs
}

func (h *Handler) updateContentChanged(key string, content *harvesterapiv1.VirtualMachineBackupContent) (*harvesterapiv1.VirtualMachineBackupContent, error) {
	if content == nil || content.DeletionTimestamp != nil {
		return nil, nil
	}
	controllerRef := metav1.GetControllerOf(content)

	// if it has a ControllerRef, that's all that matters.
	if controllerRef != nil {
		backup := h.resolveContentRef(content.Namespace, controllerRef)
		if backup == nil {
			return nil, nil
		}
		if isContentReady(content) {
			h.vmBackupController.Enqueue(backup.Namespace, backup.Name)
		}
	}
	return nil, nil
}

// resolveContentRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (h *Handler) resolveContentRef(namespace string, controllerRef *metav1.OwnerReference) *harvesterapiv1.VirtualMachineBackup {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != vmBackupKind.Kind {
		return nil
	}
	backup, err := h.vmBackupCache.Get(namespace, controllerRef.Name)
	if err != nil {
		return nil
	}
	if backup.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return backup
}
