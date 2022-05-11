package virtualmachine

import (
	"encoding/json"
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewMutator(
	setting ctlharvesterv1.SettingCache,
) types.Mutator {
	return &vmMutator{
		setting: setting,
	}
}

type vmMutator struct {
	types.DefaultMutator
	setting ctlharvesterv1.SettingCache
}

func (m *vmMutator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"virtualmachines"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   kubevirtv1.SchemeGroupVersion.Group,
		APIVersion: kubevirtv1.SchemeGroupVersion.Version,
		ObjectType: &kubevirtv1.VirtualMachine{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (m *vmMutator) Create(request *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	vm := newObj.(*kubevirtv1.VirtualMachine)
	patchOps, err := m.patchResourceOvercommit(vm)
	if err != nil {
		return patchOps, err
	}
	return patchOps, nil
}

func (m *vmMutator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	newVm := newObj.(*kubevirtv1.VirtualMachine)
	oldVm := oldObj.(*kubevirtv1.VirtualMachine)

	var patchOps types.PatchOps
	var err error

	if needUpdateResourceOvercommit(oldVm, newVm) {
		patchOps, err = m.patchResourceOvercommit(newVm)
	}
	if err != nil {
		return patchOps, err
	}

	needUpdateRunStrategy, err := needUpdateRunStrategy(oldVm, newVm)
	if err != nil {
		return patchOps, err
	}

	if needUpdateRunStrategy {
		patchOps = patchRunStrategy(newVm, patchOps)
	}
	return patchOps, nil
}

func needUpdateRunStrategy(oldVm, newVm *kubevirtv1.VirtualMachine) (bool, error) {
	// no need to patch the run strategy if user uses the spec.running filed.
	if newVm.Spec.Running != nil {
		return false, nil
	}

	newRunStrategy, err := newVm.RunStrategy()
	if err != nil {
		return false, err
	}

	oldRunStrategy, err := oldVm.RunStrategy()
	if err != nil {
		return false, err
	}

	if oldRunStrategy == kubevirtv1.RunStrategyHalted && newRunStrategy != kubevirtv1.RunStrategyHalted {
		return true, nil
	}
	return false, nil
}

// add workaround for the issue https://github.com/kubevirt/kubevirt/issues/7295
func patchRunStrategy(newVm *kubevirtv1.VirtualMachine, patchOps types.PatchOps) types.PatchOps {
	runStrategy := newVm.Annotations[util.AnnotationRunStrategy]
	if string(runStrategy) == "" {
		runStrategy = string(kubevirtv1.RunStrategyRerunOnFailure)
	}
	patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/runStrategy", "value": "%s"}`, runStrategy))
	return patchOps
}

func needUpdateResourceOvercommit(oldVm, newVm *kubevirtv1.VirtualMachine) bool {
	var newReservedMemory, oldReservedMemory string
	newLimits := newVm.Spec.Template.Spec.Domain.Resources.Limits
	newCpu := newLimits.Cpu()
	newMem := newLimits.Memory()
	if newVm.Annotations != nil {
		newReservedMemory = newVm.Annotations[util.AnnotationReservedMemory]
	}

	oldLimits := oldVm.Spec.Template.Spec.Domain.Resources.Limits
	oldCpu := oldLimits.Cpu()
	oldMem := oldLimits.Memory()
	if oldVm.Annotations != nil {
		oldReservedMemory = oldVm.Annotations[util.AnnotationReservedMemory]
	}

	if !newCpu.IsZero() && (oldCpu.IsZero() || !newCpu.Equal(*oldCpu)) {
		return true
	}
	if !newMem.IsZero() && (oldMem.IsZero() || !newMem.Equal(*oldMem)) {
		return true
	}
	if newReservedMemory != oldReservedMemory {
		return true
	}
	return false
}

func (m *vmMutator) patchResourceOvercommit(vm *kubevirtv1.VirtualMachine) ([]string, error) {
	var patchOps types.PatchOps
	limits := vm.Spec.Template.Spec.Domain.Resources.Limits
	cpu := limits.Cpu()
	mem := limits.Memory()
	requestsMissing := len(vm.Spec.Template.Spec.Domain.Resources.Requests) == 0
	requestsToMutate := v1.ResourceList{}
	overcommit, err := m.getOvercommit()
	if err != nil || overcommit == nil {
		return patchOps, err
	}

	if !cpu.IsZero() {
		newRequest := cpu.MilliValue() * int64(100) / int64(overcommit.Cpu)
		quantity := resource.NewMilliQuantity(newRequest, cpu.Format)
		if requestsMissing {
			requestsToMutate[v1.ResourceCPU] = *quantity
		} else {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/cpu", "value": "%s"}`, quantity))
		}
	}
	if !mem.IsZero() {
		// Truncate to MiB
		newRequest := mem.Value() * int64(100) / int64(overcommit.Memory) / 1048576 * 1048576
		quantity := resource.NewQuantity(newRequest, mem.Format)
		if requestsMissing {
			requestsToMutate[v1.ResourceMemory] = *quantity
		} else {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "%s"}`, quantity))
		}
		// Reserve 100MiB (104857600 Bytes) for QEMU on guest memory
		// Ref: https://github.com/harvester/harvester/issues/1234
		// TODO: handle hugepage memory
		reservedMemory := *resource.NewQuantity(104857600, resource.BinarySI)
		if vm.Annotations != nil && vm.Annotations[util.AnnotationReservedMemory] != "" {
			reservedMemory, err = resource.ParseQuantity(vm.Annotations[util.AnnotationReservedMemory])
			if err != nil {
				return patchOps, err
			}
		}
		guestMemory := *mem
		guestMemory.Sub(reservedMemory)
		if vm.Spec.Template.Spec.Domain.Memory == nil {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"%s"}}`, &guestMemory))
		} else {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/memory/guest", "value": "%s"}`, &guestMemory))
		}
	}
	if len(requestsToMutate) > 0 {
		bytes, err := json.Marshal(requestsToMutate)
		if err != nil {
			return patchOps, err
		}
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests", "value": %s}`, string(bytes)))
	}
	return patchOps, nil
}

func (m *vmMutator) getOvercommit() (*settings.Overcommit, error) {
	s, err := m.setting.Get("overcommit-config")
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	value := s.Value
	if value == "" {
		value = s.Default
	}
	if value == "" {
		return nil, nil
	}
	overcommit := &settings.Overcommit{}
	if err := json.Unmarshal([]byte(value), overcommit); err != nil {
		return overcommit, err
	}
	return overcommit, nil
}
