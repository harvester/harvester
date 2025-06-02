package virtualmachineinstance

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewMutator(
	vm ctlkubevirtv1.VirtualMachineCache,
) types.Mutator {
	return &vmiMutator{
		vm: vm,
	}
}

type vmiMutator struct {
	types.DefaultMutator
	vm ctlkubevirtv1.VirtualMachineCache
}

func (m *vmiMutator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"virtualmachineinstances"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   kubevirtv1.SchemeGroupVersion.Group,
		APIVersion: kubevirtv1.SchemeGroupVersion.Version,
		ObjectType: &kubevirtv1.VirtualMachineInstance{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (m *vmiMutator) Create(_ *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	vmi := newObj.(*kubevirtv1.VirtualMachineInstance)

	logrus.Debugf("create VMI %s/%s", vmi.Namespace, vmi.Name)

	vm, err := m.vm.Get(vmi.Namespace, vmi.Name)
	if err != nil {
		return nil, err
	}

	return m.patchMacAddress(vm, vmi)
}

func (m *vmiMutator) patchMacAddress(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) (types.PatchOps, error) {
	if vm.Annotations == nil || vm.Annotations[util.AnnotationMacAddressName] == "" {
		return nil, nil
	}

	vmiInterfaces := map[string]string{}
	if err := json.Unmarshal([]byte(vm.Annotations[util.AnnotationMacAddressName]), &vmiInterfaces); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"name":      vm.Name,
			"namespace": vm.Namespace,
			"macs":      vm.Annotations[util.AnnotationMacAddressName],
		}).Error("failed to unmarshal mac-address from vm annotation")
		return nil, nil
	}

	patchOps := types.PatchOps{}
	for i, iface := range vmi.Spec.Domain.Devices.Interfaces {
		if vm.Spec.Template.Spec.Domain.Devices.Interfaces[i].MacAddress != "" {
			continue
		}
		ifaceMac, ok := vmiInterfaces[iface.Name]
		if !ok || ifaceMac == "" {
			continue
		}
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "add", "path": "/spec/domain/devices/interfaces/%d/macAddress", "value": "%s"}`, i, ifaceMac))
	}
	return patchOps, nil
}
