package virtualmachine

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	vmwatch "kubevirt.io/kubevirt/pkg/virt-controller/watch/vm"

	ctlappsv1 "github.com/harvester/harvester/pkg/generated/controllers/apps/v1"
	vmv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

type VMNetworkController struct {
	vmCache   vmv1.VirtualMachineCache
	vmClient  vmv1.VirtualMachineClient
	vmiClient vmv1.VirtualMachineInstanceClient
	crCache   ctlappsv1.ControllerRevisionCache
	crClient  ctlappsv1.ControllerRevisionClient
}

// SetDefaultNetworkMacAddress set the default mac address of networks using the initial allocated mac address from the VMI status,
// since the most guest OS will use the initial allocated mac address of its DHCP config, and on Kubevirt the VM restart it will re-allocate
// a new mac address which will lead the original network unreachable.
func (h *VMNetworkController) SetDefaultNetworkMacAddress(id string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	if id == "" || vmi == nil || vmi.DeletionTimestamp != nil {
		return vmi, nil
	}

	if vmi.Status.Phase != kubevirtv1.Running {
		return vmi, nil
	}

	if len(vmi.Status.Interfaces) == 0 {
		return vmi, nil
	}

	err := h.updateVMDefaultNetworkMacAddress(vmi)
	if err != nil {
		return vmi, err
	}

	return vmi, nil
}

func (h *VMNetworkController) updateVMDefaultNetworkMacAddress(vmi *kubevirtv1.VirtualMachineInstance) error {
	vm, err := h.vmCache.Get(vmi.Namespace, vmi.Name)
	if err != nil {
		return err
	}

	vmCopy := vm.DeepCopy()
	var length = len(vmi.Status.Interfaces)
	var vmiInterfaces = make(map[string]string, length)
	for _, iface := range vmi.Status.Interfaces {
		if iface.MAC != "" && iface.Name != "" {
			vmiInterfaces[iface.Name] = iface.MAC
		}
	}

	for i, vmIface := range vmCopy.Spec.Template.Spec.Domain.Devices.Interfaces {
		macAddress, ok := vmiInterfaces[vmIface.Name]
		// only set the network mac address when it has no existing value
		if ok && vmIface.MacAddress == "" {
			logrus.Debugf("set VM %s management network %s macAddress to %s", vm.Name, vmIface.Name, macAddress)
			vmCopy.Spec.Template.Spec.Domain.Devices.Interfaces[i].MacAddress = macAddress
		}
	}

	// note: this function is related to vm/vmi, should always run on vmi change
	if err := h.regenerateControllerRevision(vmi, vm); err != nil {
		return err
	}

	if equality.Semantic.DeepEqual(vmCopy.Spec.Template.Spec.Domain.Devices, vm.Spec.Template.Spec.Domain.Devices) {
		return nil
	}
	if _, err := h.vmClient.Update(vmCopy); err != nil {
		return err
	}

	return nil
}

func (h *VMNetworkController) regenerateControllerRevision(vmi *kubevirtv1.VirtualMachineInstance, vm *kubevirtv1.VirtualMachine) error {
	crObj, err := h.crCache.Get(vmi.Namespace, vmi.Status.VirtualMachineRevisionName)
	if err != nil {
		return fmt.Errorf("error fetch controller revision object for vmi %s-%s: %v", vmi.Name, vmi.Namespace, err)
	}

	revisionSpec := &vmwatch.VirtualMachineRevisionData{}
	if err = json.Unmarshal(crObj.Data.Raw, revisionSpec); err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(revisionSpec.Spec.Template, vm.Spec.Template) && vm.Generation != vm.Status.ObservedGeneration {
		patch, patchGenError := patchVMRevision(vm)
		if patchGenError != nil {
			return fmt.Errorf("error during patch generation for vmi %s-%s: %v", vmi.Name, vmi.Name, patchGenError)
		}
		if deletionErr := h.crClient.Delete(vmi.Namespace, crObj.Name, &metav1.DeleteOptions{}); deletionErr != nil {
			return fmt.Errorf("error during deletion of controllerRevision for vmi %s-%s: %v", vmi.Name, vmi.Name, deletionErr)
		}
		logrus.Debugf("VM %s/%s delete and create new cr object to patch generation: %s", vm.Namespace, vm.Name, patch)
		crObj.Data.Raw = patch
		crObj.ResourceVersion = ""
		_, err = h.crClient.Create(crObj)
	}

	return err
}

// copied from kubevirt.io/kubevirt/pkg/virt-controller/watch as this is a private method
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
