package virtualmachine

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/slice"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	kubevirt "kubevirt.io/api/core"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	rqutils "github.com/harvester/harvester/pkg/util/resourcequota"
)

// A list of labels that are always automatically synchronized with the
// VMI in addition to the instance labels.
var syncLabelsToVmi = []string{util.LabelMaintainModeStrategy}

type VMController struct {
	pvcClient      v1.PersistentVolumeClaimClient
	pvcCache       v1.PersistentVolumeClaimCache
	vmClient       ctlkubevirtv1.VirtualMachineClient
	vmController   ctlkubevirtv1.VirtualMachineController
	vmiCache       ctlkubevirtv1.VirtualMachineInstanceCache
	vmiClient      ctlkubevirtv1.VirtualMachineInstanceClient
	vmBackupClient ctlharvesterv1.VirtualMachineBackupClient
	vmBackupCache  ctlharvesterv1.VirtualMachineBackupCache
	snapshotClient ctlsnapshotv1.VolumeSnapshotClient
	snapshotCache  ctlsnapshotv1.VolumeSnapshotCache
	recorder       record.EventRecorder

	vmrCalculator *rqutils.Calculator
}

// createPVCsFromAnnotation creates PVCs defined in the volumeClaimTemplates annotation if they don't exist.
func (h *VMController) createPVCsFromAnnotation(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil {
		return nil, nil
	}
	volumeClaimTemplates, ok := vm.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok || volumeClaimTemplates == "" {
		return nil, nil
	}
	var pvcs []*corev1.PersistentVolumeClaim
	if err := json.Unmarshal([]byte(volumeClaimTemplates), &pvcs); err != nil {
		return nil, err
	}

	var (
		pvc *corev1.PersistentVolumeClaim
		err error
	)
	for _, pvcAnno := range pvcs {
		pvcAnno.Namespace = vm.Namespace
		if pvc, err = h.pvcCache.Get(vm.Namespace, pvcAnno.Name); apierrors.IsNotFound(err) {
			if _, err = h.pvcClient.Create(pvcAnno); err != nil {
				return nil, err
			}
			continue
		} else if err != nil {
			return nil, err
		}

		// Users also can resize volumes through Volumes page. In that case, we can't track the update in VM annotation.
		// If storage request in the VM annotation is smaller than or equal to the real PVC, we skip it.
		if pvcAnno.Spec.Resources.Requests.Storage().Cmp(*pvc.Spec.Resources.Requests.Storage()) <= 0 {
			continue
		}

		toUpdate := pvc.DeepCopy()
		toUpdate.Spec.Resources.Requests = pvcAnno.Spec.Resources.Requests
		if !reflect.DeepEqual(toUpdate, pvc) {
			if _, err = h.pvcClient.Update(toUpdate); err != nil {
				return nil, err
			}
		}
	}

	return nil, nil
}

// SetOwnerOfPVCs records the target VirtualMachine as the owner of the PVCs in annotation.
func (h *VMController) SetOwnerOfPVCs(vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	// add harvester finalizers
	if !util.ContainsFinalizer(vm, harvesterUnsetOwnerOfPVCsFinalizer) {
		vmCopy := vm.DeepCopy()
		util.AddFinalizer(vmCopy, harvesterUnsetOwnerOfPVCsFinalizer)
		return h.vmClient.Update(vmCopy)
	}

	pvcNames := getPVCNames(&vm.Spec.Template.Spec)

	vmReferenceKey := ref.Construct(vm.Namespace, vm.Name)
	// get all the volumes attached to this virtual machine
	attachedPVCs, err := h.pvcCache.GetByIndex(indexeres.PVCByVMIndex, vmReferenceKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get attached PVCs by VM index: %w", err)
	}

	vmGVK := kubevirtv1.VirtualMachineGroupVersionKind
	vmGK := vmGVK.GroupKind()

	for _, attachedPVC := range attachedPVCs {
		// check if it's still attached
		if pvcNames.Has(attachedPVC.Name) {
			continue
		}

		toUpdate := attachedPVC.DeepCopy()

		// if this volume is no longer attached
		// remove volume's annotation
		owners, err := ref.GetSchemaOwnersFromAnnotation(attachedPVC)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema owners from annotation: %w", err)
		}

		isAttached := owners.Remove(vmGK, vm)
		if isAttached {
			if err := owners.Bind(toUpdate); err != nil {
				return nil, fmt.Errorf("failed to apply schema owners to annotation: %w", err)
			}
		}

		// update volume
		if isAttached {
			if _, err = h.pvcClient.Update(toUpdate); err != nil {
				return nil, fmt.Errorf("failed to clean schema owners for PVC(%s/%s): %w",
					attachedPVC.Namespace, attachedPVC.Name, err)
			}
		}
	}

	var pvcNamespace = vm.Namespace
	for _, pvcName := range pvcNames.List() {
		var pvc, err = h.pvcCache.Get(pvcNamespace, pvcName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// ignores not-found error, A VM can reference a non-existent PVC.
				continue
			}
			return vm, fmt.Errorf("failed to get PVC(%s/%s): %w", pvcNamespace, pvcName, err)
		}

		err = setOwnerlessPVCReference(h.pvcClient, pvc, vm)
		if err != nil {
			return vm, fmt.Errorf("failed to grant VitrualMachine(%s/%s) as PVC(%s/%s)'s owner: %w",
				vm.Namespace, vm.Name, pvcNamespace, pvcName, err)
		}
	}

	return vm, nil
}

// SyncLabelsToVmi synchronizes the labels in the VM spec to the existing VMI without re-deployment
func (h *VMController) SyncLabelsToVmi(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return vm, nil
	}

	vmi, err := h.vmiCache.Get(vm.Namespace, vm.Name)
	if err != nil && apierrors.IsNotFound(err) {
		return vm, nil
	} else if err != nil {
		return nil, fmt.Errorf("get vmi %s/%s failed, error: %w", vm.Namespace, vm.Name, err)
	}

	if err := h.syncLabels(vm, vmi); err != nil {
		return nil, err
	}

	return vm, nil
}

func (h *VMController) syncLabels(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) error {
	vmiCopy := vmi.DeepCopy()

	// Modification of the following reserved kubevirt.io/ labels on a VMI object is prohibited by the admission webhook
	// "virtualmachineinstances-update-validator.kubevirt.io", so ignore those labels during synchronization.
	// Add or update the labels of VMI
	for k, v := range vm.Spec.Template.ObjectMeta.Labels {
		if !strings.HasPrefix(k, kubevirt.GroupName) && vmi.Labels[k] != v {
			if len(vmiCopy.Labels) == 0 {
				vmiCopy.Labels = make(map[string]string)
			}
			vmiCopy.Labels[k] = v
		}
	}
	// delete the labels exist in the `vmi.Labels` but not in the `vm.spec.template.objectMeta.Labels`
	for k := range vmi.Labels {
		if _, ok := vm.Spec.Template.ObjectMeta.Labels[k]; !strings.HasPrefix(k, kubevirt.GroupName) && !ok {
			delete(vmiCopy.Labels, k)
		}
	}

	// Finally synchronize several extra labels.
	for _, k := range syncLabelsToVmi {
		value, ok := vm.Labels[k]
		if !ok {
			continue
		}
		if len(vmiCopy.Labels) == 0 {
			vmiCopy.Labels = make(map[string]string)
		}
		vmiCopy.Labels[k] = value
	}

	if !reflect.DeepEqual(vmi, vmiCopy) {
		if _, err := h.vmiClient.Update(vmiCopy); err != nil {
			return fmt.Errorf("sync labels of vm %s/%s to vmi failed, error: %w", vm.Namespace, vm.Name, err)
		}
	}

	return nil
}

// StoreRunStrategy stores the last running strategy into the annotation before the VM is stopped.
// As a workaround for the issue https://github.com/kubevirt/kubevirt/issues/7295
func (h *VMController) StoreRunStrategy(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return vm, nil
	}

	runStrategy, err := vm.RunStrategy()
	if err != nil {
		logrus.Warnf("Skip store run strategy to the annotation, error: %s", err)
		return vm, nil
	}

	if runStrategy != kubevirtv1.RunStrategyHalted && vm.Annotations[util.AnnotationRunStrategy] != string(runStrategy) {
		toUpdate := vm.DeepCopy()
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = make(map[string]string)
		}
		toUpdate.Annotations[util.AnnotationRunStrategy] = string(runStrategy)
		if _, err := h.vmClient.Update(toUpdate); err != nil {
			return vm, err
		}
	}

	return vm, nil
}

// OnVMRemove handle related PVC and delete related VMBackup snapshot
func (h *VMController) OnVMRemove(vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {

	if err := h.unsetOwnerOfPVCs(vm); err != nil {
		return vm, err
	}

	if err := h.removeVMBackupSnapshot(vm); err != nil {
		return vm, err
	}

	// remove harvester finalizers
	vmObj := vm.DeepCopy()
	util.RemoveFinalizer(vmObj, harvesterUnsetOwnerOfPVCsFinalizer)
	util.RemoveFinalizer(vmObj, oldWranglerFinalizer) // post upgrade, these are not being cleaned by wrangler so need to be managed by us
	return h.vmClient.Update(vmObj)
}

// unsetOwnerOfPVCs erases the target VirtualMachine from the owner of the PVCs in annotation.
func (h *VMController) unsetOwnerOfPVCs(vm *kubevirtv1.VirtualMachine) error {
	var (
		pvcNamespace = vm.Namespace
		pvcNames     = getPVCNames(&vm.Spec.Template.Spec)
		removedPVCs  = getRemovedPVCs(vm)
	)
	for _, pvcName := range pvcNames.List() {
		var pvc, err = h.pvcCache.Get(pvcNamespace, pvcName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// NB(thxCode): ignores error, since this can't be fixed by an immediate requeue,
				// and also doesn't block the whole logic if the PVC has already been deleted.
				continue
			}
			return fmt.Errorf("failed to get PVC(%s/%s): %w", pvcNamespace, pvcName, err)
		}

		if err := unsetBoundedPVCReference(h.pvcClient, pvc, vm); err != nil {
			return fmt.Errorf("failed to revoke VitrualMachine(%s/%s) as PVC(%s/%s)'s owner: %w",
				vm.Namespace, vm.Name, pvcNamespace, pvcName, err)
		}

		if slice.ContainsString(removedPVCs, pvcName) {
			numberOfOwner, err := numberOfBoundedPVCReference(pvc)
			if err != nil {
				return fmt.Errorf("failed to count number of owners for PVC(%s/%s): %w", pvcNamespace, pvcName, err)
			}
			// We are the solely owner here, so try to delete the PVC as requested.
			if numberOfOwner == 1 {
				if err := h.pvcClient.Delete(pvcNamespace, pvcName, &metav1.DeleteOptions{}); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// removeVMBackupSnapshot only works on VMBackup snapshot.
// This function will set PVC as VolumeSnapshot's owner reference and remove related VMBackup snapshot.
func (h *VMController) removeVMBackupSnapshot(vm *kubevirtv1.VirtualMachine) error {
	vmBackups, err := h.vmBackupCache.GetByIndex(indexeres.VMBackupBySourceVMNameIndex, vm.Name)
	if err != nil {
		return fmt.Errorf("can't get VMBackups from index %s, err: %w", indexeres.VMBackupBySourceVMNameIndex, err)
	}

	for _, vmBackup := range vmBackups {
		if vmBackup.Spec.Type == harvesterv1.Backup || vmBackup.Status == nil {
			continue
		}

		for _, vb := range vmBackup.Status.VolumeBackups {
			volumeSnapshot, err := h.snapshotCache.Get(vmBackup.Namespace, *vb.Name)
			if err != nil {
				return fmt.Errorf("can't get VolumeSnapshot %s/%s, err: %w", vmBackup.Namespace, *vb.Name, err)
			}
			pvc, err := h.pvcCache.Get(vb.PersistentVolumeClaim.ObjectMeta.Namespace, vb.PersistentVolumeClaim.ObjectMeta.Name)
			if err != nil {
				return fmt.Errorf("can't get PVC %s/%s, err: %w", vb.PersistentVolumeClaim.ObjectMeta.Namespace, vb.PersistentVolumeClaim.ObjectMeta.Name, err)
			}

			volumeSnapshotCpy := volumeSnapshot.DeepCopy()
			volumeSnapshotCpy.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: pvc.APIVersion,
					Kind:       pvc.Kind,
					Name:       pvc.Name,
					UID:        pvc.UID,
				},
			}
			if !reflect.DeepEqual(volumeSnapshot, volumeSnapshotCpy) {
				if _, err := h.snapshotClient.Update(volumeSnapshotCpy); err != nil {
					return fmt.Errorf("can't update VolumeSnapshot %+v, err: %w", volumeSnapshotCpy, err)
				}
			}
		}

		if err := h.vmBackupClient.Delete(vmBackup.Namespace, vmBackup.Name, metav1.NewDeleteOptions(0)); err != nil {
			return fmt.Errorf("can't delete VMBackup %s/%s, err: %w", vmBackup.Namespace, vmBackup.Name, err)
		}
	}
	return nil
}

// getRemovedPVCs returns removed PVCs.
func getRemovedPVCs(vm *kubevirtv1.VirtualMachine) []string {
	return strings.Split(vm.Annotations[util.RemovedPVCsAnnotationKey], ",")
}

// getPVCNames returns a name set of the PVCs used by the VMI.
func getPVCNames(vmiSpecPtr *kubevirtv1.VirtualMachineInstanceSpec) sets.String {
	var pvcNames = sets.String{}

	for _, volume := range vmiSpecPtr.Volumes {
		if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName != "" {
			pvcNames.Insert(volume.PersistentVolumeClaim.ClaimName)
		}
	}

	return pvcNames
}

func (h *VMController) ManageOwnerOfPVCs(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.Spec.Template == nil {
		return vm, nil
	}

	if vm.DeletionTimestamp == nil {
		return h.SetOwnerOfPVCs(vm)
	}

	return h.OnVMRemove(vm)
}

func (h *VMController) SetHaltIfInsufficientResourceQuota(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return vm, nil
	}

	err := h.vmrCalculator.CheckIfVMCanStartByResourceQuota(vm)
	if err == nil {
		if vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusStarting {
			logrus.Debugf("VM %s in namespace %s is starting", vm.Name, vm.Namespace)

			// Since VMI would wait for Pod to run status,
			// cause the VM is still in Starting,
			// that recalculates the VM resource quota each time until the VM is non-Starting.
			h.vmController.EnqueueAfter(vm.Namespace, vm.Name, 5*time.Second)
			return vm, nil
		}
		return vm, h.cleanUpInsufficientResourceAnnotation(vm)
	} else if !rqutils.IsInsufficientResourceError(err) {
		return nil, err
	}

	return vm, h.stopVM(vm, err.Error())
}

func (h *VMController) stopVM(vm *kubevirtv1.VirtualMachine, errMsg string) error {
	return stopVM(h.vmClient, h.recorder, vm, errMsg)
}

func (h *VMController) cleanUpInsufficientResourceAnnotation(vm *kubevirtv1.VirtualMachine) error {
	if vm.Annotations == nil {
		return nil
	}
	if _, ok := vm.Annotations[util.AnnotationInsufficientResourceQuota]; !ok {
		return nil
	}

	runStrategy, err := vm.RunStrategy()
	if err != nil {
		return err
	}
	if runStrategy == kubevirtv1.RunStrategyHalted {
		logrus.Debugf("skip cleaning up insufficient resource annotation, VM %s in namespace %s is halted.", vm.Name, vm.Namespace)
		return nil
	}

	vmi, err := h.vmiCache.Get(vm.Namespace, vm.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if vmi == nil || !vmi.IsRunning() {
		logrus.Debugf("skip cleaning up insufficient resource annotation, VM %s in namespace %s is not running.", vm.Name, vm.Namespace)
		return nil
	}

	vmCpy := vm.DeepCopy()
	delete(vmCpy.Annotations, util.AnnotationInsufficientResourceQuota)
	_, err = h.vmClient.Update(vmCpy)

	return err
}

func stopVM(vms ctlkubevirtv1.VirtualMachineClient, recorder record.EventRecorder, vm *kubevirtv1.VirtualMachine, errMsg string) error {
	logrus.Debugf("stop the VM %s in namespace %s due to insufficient resource quota: %s", vm.Name, vm.Namespace, errMsg)

	vmCpy := vm.DeepCopy()

	if vmCpy.Annotations == nil {
		vmCpy.Annotations = make(map[string]string)
	}
	vmCpy.ObjectMeta.Annotations[util.AnnotationInsufficientResourceQuota] = errMsg

	runStrategy := kubevirtv1.RunStrategyHalted
	vmCpy.Spec.RunStrategy = &runStrategy
	_, err := vms.Update(vmCpy)
	if err != nil {
		return fmt.Errorf("error updating run strategy for vm %s in namespace %s: %v", vmCpy.Name, vmCpy.Namespace, err)
	}

	recorder.Eventf(vm, corev1.EventTypeWarning, "InsufficientResourceQuota", "Set runStrategy to %s: %s", kubevirtv1.RunStrategyHalted, errMsg)
	return nil
}
