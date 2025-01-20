package virtualmachine

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	vmwatch "kubevirt.io/kubevirt/pkg/virt-controller/watch"

	ctlappsv1 "github.com/harvester/harvester/pkg/generated/controllers/apps/v1"
	vmv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	util "github.com/harvester/harvester/pkg/util"
)

type VMNetworkController struct {
	vmCache       vmv1.VirtualMachineCache
	vmClient      vmv1.VirtualMachineClient
	crCache       ctlappsv1.ControllerRevisionCache
	crClient      ctlappsv1.ControllerRevisionClient
	vmiController vmv1.VirtualMachineInstanceController
}

func (h *VMNetworkController) SyncMacAddressAndControllerRevision(id string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	if id == "" || vmi == nil || vmi.DeletionTimestamp != nil || vmi.Status.Phase != kubevirtv1.Running || len(vmi.Status.Interfaces) == 0 {
		return vmi, nil
	}

	// ensure vmi mac is backfilled to vm
	retryvmi := false
	if vmi, err := h.syncNetworkDynamicMacAddress(vmi, &retryvmi); err != nil {
		return vmi, err
	}

	// ensure vmi mac is backfilled to rc
	retrycr := false
	if vmi, err := h.syncControllerRevision(vmi, &retrycr); err != nil {
		return vmi, err
	}

	// vmi may end onChange, but vm/rc is not synced yet
	if retryvmi || retrycr {
		h.vmiController.EnqueueAfter(vmi.Namespace, vmi.Name, time.Second*1)
		return vmi, nil
	}

	// last step: if vm still did not sync with rc, update it
	return h.updateVM(vmi)
}

// if user does not specify mac address when creating a VM
// guest OS will use the initial allocated mac address from its DHCP config
// when VM restarts that mac address may change, and related IP is not persistent
// syncNetworkDynamicMacAddress saves the dynamic mac address observed from the the VMI status to vm
func (h *VMNetworkController) syncNetworkDynamicMacAddress(vmi *kubevirtv1.VirtualMachineInstance, retry *bool) (*kubevirtv1.VirtualMachineInstance, error) {
	vm, err := h.vmCache.Get(vmi.Namespace, vmi.Name)
	if err != nil {
		return vmi, err
	}

	if vm == nil || vm.DeletionTimestamp != nil {
		return vmi, nil
	}

	vmCopy := vm.DeepCopy()
	vmiInterfaces := getVmiInterfaceMacs(vmi)
	updated := false
	for i, vmIface := range vmCopy.Spec.Template.Spec.Domain.Devices.Interfaces {
		if vmIface.Name == "" || vmIface.MacAddress != "" {
			continue
		}
		if macAddress := vmiInterfaces[vmIface.Name]; macAddress != "" {
			// only backfill the network mac address when it is empty
			logrus.Infof("set VM %s network %s macAddress to %s", vm.Name, vmIface.Name, macAddress)
			vmCopy.Spec.Template.Spec.Domain.Devices.Interfaces[i].MacAddress = macAddress
			updated = true
		}
	}
	if !updated {
		return vmi, nil
	}
	*retry = true
	if _, err := h.vmClient.Update(vmCopy); err != nil {
		return vmi, fmt.Errorf("failed to update vmi mac to vm: %w", err)
	}

	return vmi, nil
}

func getVMInterfaceMacs(vm *kubevirtv1.VirtualMachine) map[string]string {
	vmInterfaceMacs := make(map[string]string, len(vm.Spec.Template.Spec.Domain.Devices.Interfaces))
	for _, vmIface := range vm.Spec.Template.Spec.Domain.Devices.Interfaces {
		if vmIface.MacAddress != "" {
			vmInterfaceMacs[vmIface.Name] = vmIface.MacAddress
		}
	}
	return vmInterfaceMacs
}

func getVmiInterfaceMacs(vmi *kubevirtv1.VirtualMachineInstance) map[string]string {
	var vmiInterfaces = make(map[string]string, len(vmi.Status.Interfaces))
	for _, iface := range vmi.Status.Interfaces {
		if iface.Name != "" && iface.MAC != "" {
			vmiInterfaces[iface.Name] = iface.MAC
		}
	}
	return vmiInterfaces
}

func (h *VMNetworkController) syncControllerRevision(vmi *kubevirtv1.VirtualMachineInstance, retry *bool) (*kubevirtv1.VirtualMachineInstance, error) {
	vm, err := h.vmCache.Get(vmi.Namespace, vmi.Name)
	if err != nil {
		return vmi, err
	}

	if vm == nil || vm.DeletionTimestamp != nil {
		return vmi, nil
	}

	// already synced
	if vm.Generation == vm.Status.ObservedGeneration {
		return vmi, nil
	}

	if err := h.regenerateControllerRevision(vmi, vm, retry); err != nil {
		return vmi, err
	}

	return vmi, nil
}

func (h *VMNetworkController) regenerateControllerRevision(vmi *kubevirtv1.VirtualMachineInstance, vm *kubevirtv1.VirtualMachine, retry *bool) error {
	crObj, err := h.crCache.Get(vmi.Namespace, vmi.Status.VirtualMachineRevisionName)
	if err != nil {
		// controllerRevision has a different name than vm name, essential to log it's name in error
		return fmt.Errorf("failed to fetch controllerRevision %s/%s: %w", vmi.Namespace, vmi.Status.VirtualMachineRevisionName, err)
	}

	revisionSpec := &vmwatch.VirtualMachineRevisionData{}
	if err = json.Unmarshal(crObj.Data.Raw, revisionSpec); err != nil {
		return fmt.Errorf("failed to unmarshal controllerRevision %s/%s: %w", crObj.Namespace, crObj.Name, err)
	}

	revisionSpecCopy := revisionSpec
	vmInterfaceMacs := getVMInterfaceMacs(vm)
	vmiInterfaceMacs := getVmiInterfaceMacs(vmi)
	updated := false
	for i, rcIface := range revisionSpec.Spec.Template.Spec.Domain.Devices.Interfaces {
		// only backfill when saved MAC is empty
		if rcIface.Name == "" || rcIface.MacAddress != "" {
			continue
		}
		vmMacAddress := vmInterfaceMacs[rcIface.Name]
		vmiMacAddress := vmiInterfaceMacs[rcIface.Name]
		// on the safe path to update revision
		if vmMacAddress != "" && vmMacAddress == vmiMacAddress {
			revisionSpecCopy.Spec.Template.Spec.Domain.Devices.Interfaces[i].MacAddress = vmMacAddress
			updated = true
		}
	}
	if !updated {
		return nil
	}

	crObjCopy := crObj.DeepCopy()
	patch, patchGenError := patchVMRevisionViaVMSpec(&revisionSpecCopy.Spec)
	if patchGenError != nil {
		return fmt.Errorf("failed to generate a patch for controllerRevision %s/%s: %w", crObj.Namespace, crObj.Name, patchGenError)
	}
	if deletionErr := h.crClient.Delete(vmi.Namespace, crObj.Name, &metav1.DeleteOptions{}); deletionErr != nil {
		return fmt.Errorf("failed to delete controllerRevision %s/%s: %w", crObj.Namespace, crObj.Name, deletionErr)
	}
	*retry = true
	logrus.Infof("VM %s/%s delete and create new controllerRevision %v to sync generation: vm.Generation %v, vm.Status.ObservedGeneration %v", vm.Namespace, vm.Name, crObj.Name, vm.Generation, vm.Status.ObservedGeneration)
	crObjCopy.Data.Raw = patch
	crObjCopy.ResourceVersion = ""
	if _, err = h.crClient.Create(crObjCopy); err != nil {
		return fmt.Errorf("failed to create controllerRevision %s/%s: %w", crObjCopy.Namespace, crObjCopy.Name, err)
	}

	return nil
}

// copied from kubevirt.io/kubevirt/pkg/virt-controller/watch as this is a private method
/*
func patchVMRevision(vm *kubevirtv1.VirtualMachine) ([]byte, error) {
	vmBytes, err := json.Marshal(vm)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	err = json.Unmarshal(vmBytes, &raw)
	if err != nil {
		return nil, err
	}
	objCopy := make(map[string]interface{})
	spec := raw["spec"].(map[string]interface{})
	objCopy["spec"] = spec
	patch, err := json.Marshal(objCopy)
	return patch, err
}
*/

// clone of patchVMRevision but works upon VirtualMachineSpec
func patchVMRevisionViaVMSpec(vmSpec *kubevirtv1.VirtualMachineSpec) ([]byte, error) {
	vmBytes, err := json.Marshal(vmSpec)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	err = json.Unmarshal(vmBytes, &raw)
	if err != nil {
		return nil, err
	}
	objCopy := make(map[string]interface{})
	objCopy["spec"] = raw
	patch, err := json.Marshal(objCopy)
	return patch, err
}

func (h *VMNetworkController) updateVM(vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	vm, err := h.vmCache.Get(vmi.Namespace, vmi.Name)
	if err != nil {
		return vmi, err
	}

	if vm == nil || vm.DeletionTimestamp != nil {
		return vmi, nil
	}

	// already synced
	if vm.Generation == vm.Status.ObservedGeneration {
		return vmi, nil
	}

	crObj, err := h.crCache.Get(vmi.Namespace, vmi.Status.VirtualMachineRevisionName)
	if err != nil {
		return vmi, fmt.Errorf("failed to fetch controllerRevision %s/%s: %w", vmi.Namespace, vmi.Status.VirtualMachineRevisionName, err)
	}

	revisionSpec := &vmwatch.VirtualMachineRevisionData{}
	if err = json.Unmarshal(crObj.Data.Raw, revisionSpec); err != nil {
		return vmi, fmt.Errorf("failed to unmarshal controllerRevision %s/%s: %w", crObj.Namespace, crObj.Name, err)
	}

	// if there are other changes than MAC, skip
	if !equality.Semantic.DeepEqual(revisionSpec.Spec, vm.Spec) {
		return vmi, nil
	}

	// forcely update vm, then upstream controller will sync the Generation and ObservedGeneration
	vmCopy := vm.DeepCopy()
	if vmCopy.Annotations == nil {
		vmCopy.Annotations = make(map[string]string, 1)
	}
	vmCopy.Annotations[util.AnnotationTimestamp] = time.Now().Format(time.RFC3339)
	logrus.Debugf("VM %s/%s is forcely updated to sync vm.Generation %v, vm.Status.ObservedGeneration %v", vm.Namespace, vm.Name, vm.Generation, vm.Status.ObservedGeneration)
	if _, err := h.vmClient.Update(vmCopy); err != nil {
		return vmi, fmt.Errorf("failed to update vm %s to sync generation: %w", util.AnnotationTimestamp, err)
	}

	return vmi, nil
}
