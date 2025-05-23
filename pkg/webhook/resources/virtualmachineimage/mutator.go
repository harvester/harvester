package virtualmachineimage

import (
	"fmt"

	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/backingimage"
	"github.com/harvester/harvester/pkg/image/cdi"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewMutator(scCache ctlstoragev1.StorageClassCache) types.Mutator {
	vmim := common.GetVMIMutator(scCache)
	mutators := map[harvesterv1.VMIBackend]backend.Mutator{
		harvesterv1.VMIBackendBackingImage: backingimage.GetMutator(vmim),
		harvesterv1.VMIBackendCDI:          cdi.GetMutator(vmim),
	}

	return &virtualMachineImageMutator{
		scCache:  scCache,
		mutators: mutators,
	}
}

type virtualMachineImageMutator struct {
	types.DefaultMutator
	scCache  ctlstoragev1.StorageClassCache
	mutators map[harvesterv1.VMIBackend]backend.Mutator
}

func (m *virtualMachineImageMutator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{harvesterv1.VirtualMachineImageResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   harvesterv1.SchemeGroupVersion.Group,
		APIVersion: harvesterv1.SchemeGroupVersion.Version,
		ObjectType: &harvesterv1.VirtualMachineImage{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (m *virtualMachineImageMutator) Create(_ *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	var patchOps types.PatchOps
	vmi := newObj.(*harvesterv1.VirtualMachineImage)

	if vmi.Spec.Backend == "" {
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/backend", "value": "%s"}`, harvesterv1.VMIBackendBackingImage))
		vmi.Spec.Backend = harvesterv1.VMIBackendBackingImage
	}

	mutatePatches, err := m.mutators[util.GetVMIBackend(vmi)].Create(vmi)
	if err != nil {
		return patchOps, err
	}

	patchOps = append(patchOps, mutatePatches...)
	return patchOps, nil
}

func (m *virtualMachineImageMutator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	var patchOps types.PatchOps
	newVMI := newObj.(*harvesterv1.VirtualMachineImage)
	oldVMI := oldObj.(*harvesterv1.VirtualMachineImage)

	mutatePatches, err := m.mutators[util.GetVMIBackend(newVMI)].Update(oldVMI, newVMI)
	if err != nil {
		return patchOps, err
	}

	patchOps = append(patchOps, mutatePatches...)
	return patchOps, nil
}
