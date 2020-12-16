package virtualmachine

import (
	"github.com/sirupsen/logrus"
	"kubevirt.io/client-go/api/v1alpha3"

	vmv1alpha3 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1alpha3"
)

type VMNetworkController struct {
	vmCache   vmv1alpha3.VirtualMachineCache
	vmClient  vmv1alpha3.VirtualMachineClient
	vmiClient vmv1alpha3.VirtualMachineInstanceClient
}

// SetDefaultNetworkMacAddress set the default mac address of networks using the initial allocated mac address from the VMI status,
// since the most guest OS will use the initial allocated mac address of its DHCP config, and on Kubevirt the VM restart it will re-allocate
// a new mac address which will lead the original network unreachable.
func (h *VMNetworkController) SetDefaultNetworkMacAddress(id string, vmi *v1alpha3.VirtualMachineInstance) (*v1alpha3.VirtualMachineInstance, error) {
	if id == "" || vmi == nil || vmi.DeletionTimestamp != nil {
		return vmi, nil
	}

	if vmi.Status.Phase != v1alpha3.Running {
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

func (h *VMNetworkController) updateVMDefaultNetworkMacAddress(vmi *v1alpha3.VirtualMachineInstance) error {
	logrus.Debugf("update default network mac address of the vm: %s\n", vmi.Name)
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

	if _, err := h.vmClient.Update(vmCopy); err != nil {
		return err
	}

	return nil
}
