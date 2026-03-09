package virtualmachineinstance

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	patchOps, err := m.patchMacAddress(vm, vmi)
	if err != nil {
		return nil, fmt.Errorf("error patching mac address for vmi %s/%s: %w", vmi.Namespace, vmi.Name, err)
	}

	if len(vmi.Spec.Domain.Devices.HostDevices) > 0 || len(vmi.Spec.Domain.Devices.GPUs) > 0 {
		devicesPatch, err := patchDeviceName(vmi)
		if err != nil {
			return nil, fmt.Errorf("error patching device name for vmi %s/%s: %w", vmi.Namespace, vmi.Name, err)
		}
		patchOps = append(patchOps, devicesPatch...)
	}

	return patchOps, nil
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

func generateEncodedAlias(aliasName string) string {
	matched := regexp.MustCompile("^[a-zA-Z0-9_-]+$").MatchString(aliasName)
	if matched {
		return aliasName
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(aliasName))
}

func patchDeviceName(vmi *kubevirtv1.VirtualMachineInstance) (types.PatchOps, error) {
	patchOps := types.PatchOps{}

	hostDevicePath := "/spec/domain/devices/hostDevices/%d/name"
	gpuDevicePath := "/spec/domain/devices/gpus/%d/name"
	for i, hostDevice := range vmi.Spec.Domain.Devices.HostDevices {
		encodedAlias := generateEncodedAlias(hostDevice.Name)
		if encodedAlias != hostDevice.Name {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "%s", "value": "%s"}`, fmt.Sprintf(hostDevicePath, i), encodedAlias))
		}
	}
	for i, gpu := range vmi.Spec.Domain.Devices.GPUs {
		encodedAlias := generateEncodedAlias(gpu.Name)
		if encodedAlias != gpu.Name {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "%s", "value": "%s"}`, fmt.Sprintf(gpuDevicePath, i), encodedAlias))
		}
	}
	return patchOps, nil
}
