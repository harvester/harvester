package virtualmachineimage

import (
	"fmt"

	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"kubevirt.io/kubevirt/pkg/apimachinery/patch"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/backingimage"
	"github.com/harvester/harvester/pkg/image/cdi"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewMutator(scCache ctlstoragev1.StorageClassCache, vmio common.VMIOperator) types.Mutator {
	vmim := common.GetVMIMutator(scCache)
	mutators := map[harvesterv1.VMIBackend]backend.Mutator{
		harvesterv1.VMIBackendBackingImage: backingimage.GetMutator(vmim),
		harvesterv1.VMIBackendCDI:          cdi.GetMutator(vmim),
	}

	return &virtualMachineImageMutator{
		scCache:  scCache,
		vmio:     vmio,
		mutators: mutators,
	}
}

type virtualMachineImageMutator struct {
	types.DefaultMutator
	scCache  ctlstoragev1.StorageClassCache
	vmio     common.VMIOperator
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

	imageBackend, backendPatches := m.patchBackend(vmi, "")
	patchOps = append(patchOps, backendPatches...)

	// Inject the imageDisplayName label at creation time so that the validating
	// webhook's CheckDisplayName can always detect duplicate display names via
	// label selector, regardless of whether the image upload succeeds or fails.
	if len(vmi.Labels) == 0 {
		patchOps = append(patchOps, `{"op": "add", "path": "/metadata/labels", "value": {}}`)
	}
	escapedLabel := patch.EscapeJSONPointer(util.LabelImageDisplayName)
	patchOps = append(patchOps, fmt.Sprintf(`{"op": "add", "path": "/metadata/labels/%s", "value": "%s"}`, escapedLabel, vmi.Spec.DisplayName))

	mutatePatches, err := m.mutators[imageBackend].Create(vmi)
	if err != nil {
		return patchOps, err
	}

	patchOps = append(patchOps, mutatePatches...)
	return patchOps, nil
}

func (m *virtualMachineImageMutator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	newVMI := newObj.(*harvesterv1.VirtualMachineImage)
	oldVMI := oldObj.(*harvesterv1.VirtualMachineImage)

	imageBackend, patchOps := m.patchBackend(newVMI, m.vmio.GetBackend(oldVMI))
	mutatePatches, err := m.mutators[imageBackend].Update(oldVMI, newVMI)
	if err != nil {
		return nil, err
	}

	patchOps = append(patchOps, mutatePatches...)
	return patchOps, nil
}

func (m *virtualMachineImageMutator) patchBackend(vmi *harvesterv1.VirtualMachineImage, fallback harvesterv1.VMIBackend) (harvesterv1.VMIBackend, types.PatchOps) {
	if imageBackend := m.vmio.GetBackend(vmi); imageBackend != "" {
		return imageBackend, nil
	}
	if fallback == "" {
		fallback = m.defaultBackend()
	}
	op := fmt.Sprintf(`{"op": "add", "path": "/spec/backend", "value": "%s"}`, fallback)
	return fallback, types.PatchOps{op}
}

func (m *virtualMachineImageMutator) defaultBackend() harvesterv1.VMIBackend {
	sc := util.GetDefaultSC(m.scCache)
	if sc != nil && util.IsLonghornV1StorageClass(sc) {
		return harvesterv1.VMIBackendBackingImage
	}
	return harvesterv1.VMIBackendCDI
}
