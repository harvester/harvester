package addon

import (
	"fmt"

	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	pcidevicesControllerAddonName = "pcidevices-controller"
	// pcidevicesVMIFinalizer is added by wrangler OnRemove to every VMI the pcidevices-controller sees.
	// When the add-on is disabled, the controller pod is gone but the finalizer stays, blocking VM deletion.
	pcidevicesVMIFinalizer = "wrangler.cattle.io/virtual-machine-deletion"
)

// cleanupOrphanedPCIDevicesFinalizer removes the pcidevices wrangler finalizer from VMIs that have
// no host devices or GPUs whenever the pcidevices-controller addon reaches AddonDisabled. The
// finalizer is harmless on VMIs with actual devices (the add-on must be re-enabled to delete those),
// but it permanently blocks deletion of plain VMs once the add-on is gone.
func (h *Handler) cleanupOrphanedPCIDevicesFinalizer(_ string, addon *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if addon == nil || addon.Name != pcidevicesControllerAddonName {
		return addon, nil
	}
	if addon.Status.Status != harvesterv1.AddonDisabled {
		return addon, nil
	}

	vmis, err := h.vmiCache.List("", labels.Everything())
	if err != nil {
		return addon, fmt.Errorf("listing VMIs to clean up pcidevices finalizer: %w", err)
	}

	for _, vmi := range vmis {
		if !util.ContainsFinalizer(vmi, pcidevicesVMIFinalizer) {
			continue
		}
		if len(vmi.Spec.Domain.Devices.HostDevices) > 0 || len(vmi.Spec.Domain.Devices.GPUs) > 0 {
			continue
		}
		vmiCopy := vmi.DeepCopy()
		util.RemoveFinalizer(vmiCopy, pcidevicesVMIFinalizer)
		if _, err := h.vmiClient.Update(vmiCopy); err != nil {
			return addon, fmt.Errorf("removing pcidevices finalizer from VMI %s/%s: %w", vmi.Namespace, vmi.Name, err)
		}
	}

	return addon, nil
}
