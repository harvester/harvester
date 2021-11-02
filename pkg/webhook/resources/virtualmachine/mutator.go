package virtualmachine

import (
	"encoding/json"
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/client-go/api/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
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
		Name:       "virtualmachines",
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
	vm := newObj.(*kubevirtv1.VirtualMachine)
	patchOps, err := m.patchResourceOvercommit(vm)
	if err != nil {
		return patchOps, err
	}
	return patchOps, nil
}

func (m *vmMutator) patchResourceOvercommit(vm *kubevirtv1.VirtualMachine) ([]string, error) {
	// We only handle cases that missing limits but having requests on resources.
	// That's where KubeVirt won't apply the overcommit feature.
	var patchOps types.PatchOps
	limits := vm.Spec.Template.Spec.Domain.Resources.Limits
	requests := vm.Spec.Template.Spec.Domain.Resources.Requests
	newLimits := v1.ResourceList{}
	overcommit, err := m.getOvercommit()
	if err != nil || overcommit == nil {
		return patchOps, err
	}

	if limits.Cpu().IsZero() && !requests.Cpu().IsZero() {
		newRequest := requests.Cpu().MilliValue() * int64(100) / int64(overcommit.Cpu)
		quantity := resource.NewMilliQuantity(newRequest, requests.Cpu().Format)
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/cpu", "value": "%s"}`, quantity))
		if len(limits) == 0 {
			newLimits[v1.ResourceCPU] = *requests.Cpu()
		} else {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/limits/cpu", "value": "%s"}`, requests.Cpu().String()))
		}
	}
	if limits.Memory().IsZero() && !requests.Memory().IsZero() {
		newRequest := requests.Memory().Value() * int64(100) / int64(overcommit.Memory)
		quantity := resource.NewQuantity(newRequest, requests.Memory().Format)
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "%s"}`, quantity))
		if len(limits) == 0 {
			newLimits[v1.ResourceMemory] = *requests.Memory()
		} else {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/limits/memory", "value": "%s"}`, requests.Memory().String()))
		}
	}
	if len(newLimits) > 0 {
		bytes, err := json.Marshal(newLimits)
		if err != nil {
			return patchOps, err
		}
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/limits", "value": %s}`, string(bytes)))
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
	if s.Value == "" {
		return nil, nil
	}
	overcommit := &settings.Overcommit{}
	if err := json.Unmarshal([]byte(s.Value), overcommit); err != nil {
		return overcommit, err
	}
	return overcommit, nil
}
