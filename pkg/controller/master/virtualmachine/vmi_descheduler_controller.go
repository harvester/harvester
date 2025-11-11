package virtualmachine

import (
	"reflect"

	"github.com/sirupsen/logrus"
	kubevirtv1 "kubevirt.io/api/core/v1"

	vmv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

const (
	deschedulerPreferNoEvictionAnnotationKey = "descheduler.alpha.kubernetes.io/prefer-no-eviction"
)

type VMIDeschedulerController struct {
	vmCache  vmv1.VirtualMachineCache
	vmClient vmv1.VirtualMachineClient
}

func (h *VMIDeschedulerController) IgnoreNonMigratableVM(id string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	if id == "" || vmi == nil || vmi.DeletionTimestamp != nil {
		return vmi, nil
	}

	if vmi.Status.Phase != kubevirtv1.Running {
		return vmi, nil
	}

	vm, err := h.vmCache.Get(vmi.Namespace, vmi.Name)
	if err != nil {
		return vmi, err
	}

	vmCopy := vm.DeepCopy()
	if vmCopy.Annotations == nil {
		vmCopy.Annotations = make(map[string]string)
	}

	if !vmi.IsMigratable() {
		logrus.Infof("VM %s/%s is non-migratable, skipping descheduling", vm.Namespace, vm.Name)
		vmCopy.Annotations[deschedulerPreferNoEvictionAnnotationKey] = "true"
	} else {
		logrus.Infof("VM %s/%s is migratable, removing skipping descheduling annotation", vm.Namespace, vm.Name)
		delete(vmCopy.Annotations, deschedulerPreferNoEvictionAnnotationKey)
	}

	if !reflect.DeepEqual(vm.Annotations, vmCopy.Annotations) {
		_, err = h.vmClient.Update(vmCopy)
		if err != nil {
			return vmi, err
		}
	}
	return vmi, nil
}
