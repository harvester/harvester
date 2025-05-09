package virtualmachineimage

import (
	"fmt"

	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
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

func NewMutator(scCache ctlstoragev1.StorageClassCache) types.Mutator {
	vmim := common.GetVMIMutator(scCache)
	mutators := map[harvesterv1.VMIBackend]backend.Mutator{
		harvesterv1.VMIBackendBackingImage: backingimage.GetMutator(vmim),
		harvesterv1.VMIBackendCDI:          cdi.GetMutator(),
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

	targetSCName := ""
	if vmi.Annotations[util.AnnotationStorageClassName] != "" {
		targetSCName = vmi.Annotations[util.AnnotationStorageClassName]
	} else if vmi.Spec.TargetStorageClassName != "" {
		targetSCName = vmi.Spec.TargetStorageClassName
	}

	// find the default storage class
	if targetSCName == "" {
		scList, err := m.scCache.List(labels.Everything())
		if err != nil {
			return patchOps, err
		}
		defaultSC := util.GetDefaultSC(scList)
		if defaultSC == nil {
			err := fmt.Errorf("missing default storage class")
			return patchOps, err
		}
		targetSCName = defaultSC.Name
	}

	// ensure the annotation `harvesterhci.io/storageClassName` is set
	if vmi.Annotations[util.AnnotationStorageClassName] == "" && targetSCName != "" {
		if vmi.Annotations == nil {
			patchOps = append(patchOps, `{"op": "add", "path": "/metadata/annotations", "value": {}}`)
			vmi.Annotations = map[string]string{}
		}
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/metadata/annotations/%s", "value": "%s"}`, patch.EscapeJSONPointer(util.AnnotationStorageClassName), targetSCName))
		// set the annotation
		vmi.Annotations[util.AnnotationStorageClassName] = targetSCName
	}

	// ensure the spec.targetStorageClassName is set and consist with the above annotation
	if vmi.Spec.TargetStorageClassName == "" && targetSCName != "" {
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/targetStorageClassName", "value": "%s"}`, targetSCName))
		vmi.Spec.TargetStorageClassName = targetSCName
	}

	mutatePatches, err := m.mutators[util.GetVMIBackend(vmi)].Create(vmi)
	if err != nil {
		return patchOps, err
	}

	patchOps = append(patchOps, mutatePatches...)
	return patchOps, nil
}
