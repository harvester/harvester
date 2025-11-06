package virtualmachine

import (
	"encoding/json"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/validation"
	kubevirtv1 "kubevirt.io/api/core/v1"

	vmv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

type VMNetworkController struct {
	vmCache   vmv1.VirtualMachineCache
	vmClient  vmv1.VirtualMachineClient
	vmiClient vmv1.VirtualMachineInstanceClient
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

	err := h.updateVMMacAddressAnnotation(vmi)
	if err != nil {
		return vmi, err
	}

	return vmi, nil
}

func (h *VMNetworkController) BackfillObservedNetworkMacAddress(id string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	if vmi == nil {
		return vmi, nil
	}

	err := h.updateVMMacAddressFromAnnotation(vmi)
	if err != nil {
		return vmi, err
	}

	return vmi, nil
}

func (h *VMNetworkController) updateVMMacAddressAnnotation(vmi *kubevirtv1.VirtualMachineInstance) error {
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

	vmiInterfaceBytes, err := json.Marshal(vmiInterfaces)
	if err != nil {
		return err
	}

	if vmCopy.Annotations == nil {
		vmCopy.Annotations = make(map[string]string)
	}

	previousMacAddress := vmCopy.Annotations[util.AnnotationMacAddressName]

	if previousMacAddress != string(vmiInterfaceBytes) {
		vmCopy.Annotations[util.AnnotationMacAddressName] = string(vmiInterfaceBytes)
		if err = validation.ValidateAnnotationsSize(vmCopy.Annotations); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"name":      vmCopy.Name,
				"namespace": vmCopy.Namespace,
			}).Error("cannot set vmi interfaces to vm annotations")
			// skip error to avoid the controller keeps reconciling the VM
			return nil
		}

		if _, err := h.vmClient.Update(vmCopy); err != nil {
			return err
		}
	}

	return nil
}

func (h *VMNetworkController) updateVMMacAddressFromAnnotation(vmi *kubevirtv1.VirtualMachineInstance) error {
	vm, err := h.vmCache.Get(vmi.Namespace, vmi.Name)
	if err != nil {
		return err
	}

	// no need to patch VM spec when both VM and VMI are registered for deletion
	if vm.DeletionTimestamp != nil {
		return nil
	}

	macAddressFromAnnotation, exists := vm.Annotations[util.AnnotationMacAddressName]
	if !exists {
		return nil
	}

	vmiInterfaces := make(map[string]string)
	if err := json.Unmarshal([]byte(macAddressFromAnnotation), &vmiInterfaces); err != nil {
		return err
	}

	vmCopy := vm.DeepCopy()
	vmInterfaces := vmCopy.Spec.Template.Spec.Domain.Devices.Interfaces
	for i := range(vmInterfaces) {
		iface := vmInterfaces[i]
		macAddress, observed := vmiInterfaces[iface.Name]
		if iface.MacAddress != "" {
			if iface.MacAddress != macAddress {
				logrus.WithFields(logrus.Fields{
					"name":      vmCopy.Name,
					"namespace": vmCopy.Namespace,
				}).Warnf("Detected unmatched MAC address of interface '%s' between annotation '%s' and spec '%s'", iface.Name, macAddress, iface.MacAddress)
			}
			continue
		}

		if !observed {
			logrus.WithFields(logrus.Fields{
				"name":      vmCopy.Name,
				"namespace": vmCopy.Namespace,
			}).Warnf("MAC address of interface '%s' hasn't been observed yet", iface.Name)
			continue
		}

		vmInterfaces[i].MacAddress = macAddress
	}

	if _, err := h.vmClient.Update(vmCopy); err != nil {
		return err
	}

	return nil
}
