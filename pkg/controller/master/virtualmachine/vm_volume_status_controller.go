package virtualmachine

import (
	"encoding/json"
	"fmt"
	"strings"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	corectl "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtapis "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"

	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
)

const (
	// only specific LH volume status is checked and updated into virtual machine
	// all other states are categorized into  "OK"
	volumeStatusOK                  = volumeStatus("ok")
	volumeStatusInsufficientStorage = volumeStatus("insufficient storage")
)

type VMVolumeStatusController struct {
	vmClient  ctlkubevirtv1.VirtualMachineClient
	vmCache   ctlkubevirtv1.VirtualMachineCache
	pvcClient corectl.PersistentVolumeClaimClient
	pvcCache  corectl.PersistentVolumeClaimCache
}

type volumeStatus string

type volumeStatusAnnotation struct {
	// name of volume
	Name string `json:"name,omitempty"`
	// abstracted status based on longhorn (LH) volume status
	Status string `json:"status,omitempty"`
}

type volumeStatusAnnotationSlice []volumeStatusAnnotation

func (v volumeStatus) String() string {
	return string(v)
}

// try to update LH Volume status to PVC Status
func (h *VMVolumeStatusController) OnLHVolumeChange(key string, lhVolume *longhornv1.Volume) (*longhornv1.Volume, error) {
	if lhVolume == nil || lhVolume.DeletionTimestamp != nil {
		return nil, nil
	}

	// when volume is not connected to pvc, skip
	pvcName := lhVolume.Status.KubernetesStatus.PVCName
	namespace := lhVolume.Status.KubernetesStatus.Namespace
	if pvcName == "" || namespace == "" {
		return nil, nil
	}

	volStatus := volumeStatusOK
	// before LH has a better solution, we can only hard code those conditions to deduce one volume is in none-OK state
	// new status should also be generated here
	if lhVolume.Status.State == longhornv1.VolumeStateDetached && lhVolume.Status.Robustness == longhornv1.VolumeRobustnessUnknown {
		if c, ok := lhVolume.Status.Conditions[longhornv1.VolumeConditionTypeScheduled]; ok {
			if c.Status == longhornv1.ConditionStatusFalse && c.Reason == longhornv1.VolumeConditionReasonReplicaSchedulingFailure {
				// assume insufficient storage
				volStatus = volumeStatusInsufficientStorage
			}
		}
	}
	return nil, h.updateLHVolumeStatusToPVC(lhVolume.Name, namespace, pvcName, volStatus)
}

// try to remove volume status from PVC Status if it is there
func (h *VMVolumeStatusController) OnLHVolumeRemove(key string, lhVolume *longhornv1.Volume) (*longhornv1.Volume, error) {
	if lhVolume == nil {
		return nil, nil
	}

	pvcName := lhVolume.Status.KubernetesStatus.PVCName
	namespace := lhVolume.Status.KubernetesStatus.Namespace
	if pvcName == "" || namespace == "" {
		return nil, nil
	}

	return nil, h.removeLHVolumeStatusFromPVC(lhVolume.Name, namespace, pvcName)
}

// LH volume is changed, update the possible volume status to PVC
func (h *VMVolumeStatusController) updateLHVolumeStatusToPVC(volName string, pvcNamespace string, pvcName string, status volumeStatus) error {
	var err error
	var pvc *corev1.PersistentVolumeClaim
	// pvc may be deleted, but LH volume is still there, in such case, give up updating
	if pvc, err = h.getPVC(pvcNamespace, pvcName); err != nil {
		if !errors.IsNotFound(err) {
			logrus.Infof("fail to get pvc:(%s/%s), give up to update LH volume:%s status:%s, error:%s", pvcNamespace, pvcName, volName, status.String(), fmt.Errorf("%w", err))
		}
		return nil
	}

	if status == volumeStatusOK {
		if _, ok := pvc.Annotations[util.AnnotationVolumeStatus]; ok {
			// remove existing value
			if _, err := h.removePVCAnnotationsVolumeStatus(pvc); err != nil {
				if !errors.IsConflict(err) {
					logrus.Infof("pvc:(%s/%s), fail to remove volume:%s status, error:%s", pvcNamespace, pvcName, volName, fmt.Errorf("%w", err))
				}
				return err
			}
		}
	} else {
		if volStatus, ok := pvc.Annotations[util.AnnotationVolumeStatus]; !ok || volStatus != status.String() {
			if _, err := h.updatePVCAnnotationsVolumeStatus(pvc, status.String()); err != nil {
				if !errors.IsConflict(err) {
					logrus.Infof("pvc:(%s/%s), fail to update volume:%s status:%s, error:%s", pvcNamespace, pvcName, volName, status.String(), fmt.Errorf("%w", err))
				}
				return err
			}

		}
	}

	return nil
}

// LH volume is deleted, remove the possible volue status from PVC
func (h *VMVolumeStatusController) removeLHVolumeStatusFromPVC(volName string, pvcNamespace string, pvcName string) error {
	var err error
	var pvc *corev1.PersistentVolumeClaim
	if pvc, err = h.getPVC(pvcNamespace, pvcName); err != nil {
		if !errors.IsNotFound(err) {
			logrus.Infof("fail to get owner pvc:(%s/%s), give up to remove status of LH volume:%s, error:%s", pvcNamespace, pvcName, volName, fmt.Errorf("%w", err))
		}
		return nil
	}

	if _, ok := pvc.Annotations[util.AnnotationVolumeStatus]; ok {
		if _, err := h.removePVCAnnotationsVolumeStatus(pvc); err != nil {
			if !errors.IsConflict(err) {
				logrus.Infof("pvc:(%s/%s), fail to remove LH volume:%s status, error:%s", pvcNamespace, pvcName, volName, fmt.Errorf("%w", err))
			}
			return err
		}
	}

	return nil
}

// as per: func (o AnnotationSchemaOwners) List(ownerGK schema.GroupKind) []string
func (h *VMVolumeStatusController) getPVCRefOwnerVMs(pvc *corev1.PersistentVolumeClaim) ([]string, error) {
	if pvcOwners, err := ref.GetSchemaOwnersFromAnnotation(pvc); err != nil {
		return nil, err
	} else if len(pvcOwners) == 0 {
		// pvc is not attched to any owner yet
		return nil, nil
	} else {
		vmGVK := kubevirtapis.VirtualMachineGroupVersionKind
		vmGK := vmGVK.GroupKind()
		refOwnerVms := pvcOwners.List(vmGK)
		return refOwnerVms, nil
	}
}

// pvc annotation is further updated to VM annotation
func (h *VMVolumeStatusController) OnPVCChange(key string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc == nil || pvc.DeletionTimestamp != nil {
		return nil, nil
	}

	var err error
	// process "pvc is detached vm" first, pvc may be updated after this call, following needs to return pvc
	if pvc, err = h.removeVolumeStatusFromDetachedVM(pvc); err != nil {
		return nil, err
	}

	var refOwnerVms []string
	if refOwnerVms, err = h.getPVCRefOwnerVMs(pvc); err != nil {
		return pvc, err
	} else if len(refOwnerVms) == 0 {
		// pvc is not attched to any owner yet
		return pvc, nil
	}

	update := false
	var volStatus string
	var ok bool
	// when pvc has !empty volume annotations, then update(add) to vm, otherwise remove from vm
	if volStatus, ok = pvc.Annotations[util.AnnotationVolumeStatus]; ok && volStatus != "" {
		update = true
	}

	for _, owner := range refOwnerVms {
		namespace, vmName := ref.Parse(owner)
		var err error
		var vm *kubevirtapis.VirtualMachine
		if vm, err = h.getVM(namespace, vmName); err != nil {
			if !errors.IsNotFound(err) {
				logrus.Infof("fail to get owner vm:(%s/%s), give up to update pvc status:%s, error:%s", namespace, vmName, volStatus, fmt.Errorf("%w", err))
			}
			continue
		}

		if update {
			_, err = h.updateVolumeStatusToVM(vm, volumeStatusAnnotation{Name: pvc.Name, Status: volStatus})
		} else {
			_, err = h.removeVolumeStatusFromVM(vm, volumeStatusAnnotation{Name: pvc.Name})
		}
		if err != nil {
			if !errors.IsConflict(err) {
				logrus.Infof("pvc:(%s/%s) update status to owner vm:%s fail, error:%s", pvc.Namespace, pvc.Name, vm.Name, fmt.Errorf("%w", err))
			}
			return pvc, err
		}
	}

	return pvc, nil
}

// when pvc is used by vm, it is not allowed to delte from webUI
func (h *VMVolumeStatusController) OnPVCRemove(key string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc == nil {
		return nil, nil
	}

	var refOwnerVms []string
	var err error
	if refOwnerVms, err = h.getPVCRefOwnerVMs(pvc); err != nil {
		return nil, err
	} else if len(refOwnerVms) == 0 {
		// pvc is not attched to any owner yet
		return nil, nil
	}

	// remove pvc status from vm
	for _, owner := range refOwnerVms {
		namespace, vmName := ref.Parse(owner)
		var err error
		var vm *kubevirtapis.VirtualMachine
		if vm, err = h.getVM(namespace, vmName); err != nil {
			if !errors.IsNotFound(err) {
				logrus.Infof("fail to get owner vm:(%s/%s), give up to remove pvc:%s status, error:%s", namespace, vmName, pvc.Name, fmt.Errorf("%w", err))
			}
			continue
		}

		_, err = h.removeVolumeStatusFromVM(vm, volumeStatusAnnotation{Name: pvc.Name})
		if err != nil {
			if !errors.IsConflict(err) {
				logrus.Infof("pvc:(%s/%s) remove status from owner vm:%s fail, error:%s", pvc.Namespace, pvc.Name, vmName, fmt.Errorf("%w", err))
			}
			return nil, err
		}
	}

	return nil, nil
}

// after pvc is detached/unmounted from VM, remove its volume status from VM
func (h *VMVolumeStatusController) removeVolumeStatusFromDetachedVM(pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	namespace, vmName, ok := h.decodeAnnotationDetachedVMString(pvc)
	if !ok {
		return pvc, nil
	}

	logrus.Debugf("remove pvc:%s status from previous owner vm:(%s/%s)", pvc.Name, namespace, vmName)

	vm, err := h.getVM(namespace, vmName)
	if err != nil {
		if !errors.IsNotFound(err) {
			logrus.Infof("fail to get vm:(%s/%s), give up to remove pvc:%s status, error:%s", namespace, vmName, pvc.Name, fmt.Errorf("%w", err))
		}
		return pvc, nil
	}

	if _, err = h.removeVolumeStatusFromVM(vm, volumeStatusAnnotation{Name: pvc.Name}); err != nil {
		if !errors.IsConflict(err) {
			logrus.Infof("pvc:(%s/%s) remove status from owner vm:%s fail, error:%s", pvc.Namespace, pvc.Name, vmName, fmt.Errorf("%w", err))
		}
		return pvc, err
	}

	// delete this temp annotation
	return h.removePVCAnnotationsDetachedVM(pvc)
}

func (h *VMVolumeStatusController) getPVC(namespace string, name string) (*corev1.PersistentVolumeClaim, error) {
	if pvc, err := h.pvcCache.Get(namespace, name); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		} else {
			return h.pvcClient.Get(namespace, name, metav1.GetOptions{})
		}
	} else {
		return pvc, nil
	}
}

func (h *VMVolumeStatusController) getVM(namespace string, name string) (*kubevirtapis.VirtualMachine, error) {
	if vm, err := h.vmCache.Get(namespace, name); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		} else {
			return h.vmClient.Get(namespace, name, metav1.GetOptions{})
		}
	} else {
		return vm, nil
	}
}

// update/add volume status to vm
func (h *VMVolumeStatusController) updateVolumeStatusToVM(vm *kubevirtapis.VirtualMachine, status volumeStatusAnnotation) (*kubevirtapis.VirtualMachine, error) {
	volumeStatusAnno, err := getVMVolumeStatusAnnotationSlice(vm)
	if err != nil {
		return nil, err
	}

	add := true
	if volumeStatusAnno != nil {
		if index, ok := volumeStatusAnno.findByKey(status.Name); ok {
			if volumeStatusAnno[index].Status == status.Status {
				return nil, nil
			} else {
				// update
				volumeStatusAnno[index].Status = status.Status
				add = false
			}
		}
	}

	if add {
		volumeStatusAnno = append(volumeStatusAnno, status)
	}

	logrus.Debugf("udpate vm(%s/%s) volume status annotations, volume(%s/%s), isAdded:%t", vm.Namespace, vm.Name, status.Name, status.Status, add)

	return h.updateVmAnnotationsVolumeStatus(vm, volumeStatusAnno)
}

func (v volumeStatusAnnotationSlice) findByKey(key string) (int, bool) {
	for i, value := range v {
		if value.Name == key {
			return i, true
		}
	}
	return -1, false
}

func (v volumeStatusAnnotationSlice) deleteByIndex(index int) volumeStatusAnnotationSlice {
	if len(v) == 0 || index < 0 || index >= len(v) {
		return v
	}

	var vNew = make(volumeStatusAnnotationSlice, len(v)-1)
	k := 0
	for i, value := range v {
		if i != index {
			vNew[k] = value
			k++
		}
	}
	return vNew
}

// convert vm volume status annotations string into a slice
func getVMVolumeStatusAnnotationSlice(vm *kubevirtapis.VirtualMachine) (volumeStatusAnnotationSlice, error) {
	if vm == nil {
		return nil, nil
	}
	var volumeStatusStr string
	var ok bool
	if volumeStatusStr, ok = vm.Annotations[util.AnnotationVolumeStatus]; !ok || volumeStatusStr == "" {
		return nil, nil
	}

	var volumeStatusAnno volumeStatusAnnotationSlice
	if err := json.Unmarshal([]byte(volumeStatusStr), &volumeStatusAnno); err != nil {
		return nil, err
	}
	return volumeStatusAnno, nil
}

// remove volume status from vm
func (h *VMVolumeStatusController) removeVolumeStatusFromVM(vm *kubevirtapis.VirtualMachine, status volumeStatusAnnotation) (*kubevirtapis.VirtualMachine, error) {
	var volumeStatusAnno volumeStatusAnnotationSlice
	var err error

	if volumeStatusAnno, err = getVMVolumeStatusAnnotationSlice(vm); err != nil {
		return nil, err
	}

	if volumeStatusAnno == nil {
		return nil, nil
	}

	var index int
	var ok bool
	index, ok = volumeStatusAnno.findByKey(status.Name)

	// this pvc/volume is not in vm annotations, no action is needed
	if !ok {
		return nil, nil
	}

	if len(volumeStatusAnno) == 1 {
		// the last one, directly delete the annotation
		logrus.Debugf("delete vm(%s/%s) volume status annotations, last one:%s", vm.Namespace, vm.Name, status.Name)
		return h.removeVmAnnotationsVolumeStatus(vm)
	} else {
		logrus.Debugf("udpate vm(%s/%s) volume annotations, remove:%s", vm.Namespace, vm.Name, status.Name)
		volumeStatusAnnoNew := volumeStatusAnno.deleteByIndex(index)
		return h.updateVmAnnotationsVolumeStatus(vm, volumeStatusAnnoNew)
	}
}

func (h *VMVolumeStatusController) updatePVCAnnotationsVolumeStatus(pvc *corev1.PersistentVolumeClaim, status string) (*corev1.PersistentVolumeClaim, error) {
	pvcCpy := pvc.DeepCopy()
	pvcCpy.Annotations[util.AnnotationVolumeStatus] = status
	return h.pvcClient.Update(pvcCpy)
}

func (h *VMVolumeStatusController) removePVCAnnotationsVolumeStatus(pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	pvcCpy := pvc.DeepCopy()
	delete(pvcCpy.Annotations, util.AnnotationVolumeStatus)
	return h.pvcClient.Update(pvcCpy)
}

// remove PVC temp annotation
func (h *VMVolumeStatusController) removePVCAnnotationsDetachedVM(pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	pvcCpy := pvc.DeepCopy()
	delete(pvcCpy.Annotations, util.AnnotationDetachedVM)
	return h.pvcClient.Update(pvcCpy)
}

func encodeAnnotationDetachedVMString(namespace string, name string) string {
	return strings.Join([]string{namespace, name}, "/")
}

func (h *VMVolumeStatusController) decodeAnnotationDetachedVMString(pvc *corev1.PersistentVolumeClaim) (string, string, bool) {
	if detachedVM, ok := pvc.Annotations[util.AnnotationDetachedVM]; !ok || detachedVM == "" {
		return "", "", false
	} else {
		vminfo := strings.Split(detachedVM, "/")
		if len(vminfo) != 2 {
			return "", "", false
		}
		// namespace, name
		return vminfo[0], vminfo[1], true
	}
}

// PVC is detached from VM, update VM to PVC annotations (temp)
// controller will use it to remove related VM annotations, and then delete it from PVC annotations
func updatePVCAnnotationsDetachedFromVM(pvc *corev1.PersistentVolumeClaim, vm *kubevirtapis.VirtualMachine) error {
	if vm == nil || pvc == nil {
		return nil
	}

	// only when vm has volume status
	if vmVolumeStatusAnno, err := getVMVolumeStatusAnnotationSlice(vm); vmVolumeStatusAnno != nil && err == nil {
		// pvc status is also in vm volume status
		if _, ok := vmVolumeStatusAnno.findByKey(pvc.Name); ok {
			logrus.Debugf("vm:(%s/%s) is added to pvc:%s annotation detached vm", vm.Namespace, vm.Name, pvc.Name)
			pvc.Annotations[util.AnnotationDetachedVM] = encodeAnnotationDetachedVMString(vm.Namespace, vm.Name)
		}
	}

	return nil
}

// update vm volume status annotations with new value
func (h *VMVolumeStatusController) updateVmAnnotationsVolumeStatus(vm *kubevirtapis.VirtualMachine, status volumeStatusAnnotationSlice) (*kubevirtapis.VirtualMachine, error) {
	if toUpdateVolueStatusBytes, err := json.Marshal(status); err != nil {
		return nil, err
	} else {
		vmCpy := vm.DeepCopy()
		vmCpy.Annotations[util.AnnotationVolumeStatus] = string(toUpdateVolueStatusBytes)
		return h.vmClient.Update(vmCpy)
	}
}

// remove vm volume status annotations
func (h *VMVolumeStatusController) removeVmAnnotationsVolumeStatus(vm *kubevirtapis.VirtualMachine) (*kubevirtapis.VirtualMachine, error) {
	vmCpy := vm.DeepCopy()
	delete(vmCpy.Annotations, util.AnnotationVolumeStatus)
	return h.vmClient.Update(vmCpy)
}
