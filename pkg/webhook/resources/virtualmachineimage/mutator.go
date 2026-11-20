package virtualmachineimage

import (
	"fmt"

	longhorntypes "github.com/longhorn/longhorn-manager/types"
	"github.com/sirupsen/logrus"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
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

func NewMutator(scCache ctlstoragev1.StorageClassCache, nodeCache ctlcorev1.NodeCache) types.Mutator {
	vmim := common.GetVMIMutator(scCache)
	mutators := map[harvesterv1.VMIBackend]backend.Mutator{
		harvesterv1.VMIBackendBackingImage: backingimage.GetMutator(vmim),
		harvesterv1.VMIBackendCDI:          cdi.GetMutator(vmim),
	}

	return &virtualMachineImageMutator{
		scCache:   scCache,
		nodeCache: nodeCache,
		mutators:  mutators,
	}
}

type virtualMachineImageMutator struct {
	types.DefaultMutator
	scCache   ctlstoragev1.StorageClassCache
	nodeCache ctlcorev1.NodeCache
	mutators  map[harvesterv1.VMIBackend]backend.Mutator
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
		backend, err := m.defaultBackend()
		if err != nil {
			logrus.WithError(err).Errorf("failed to determine default backend for VirtualMachineImage %s/%s, VMI creation will be rejected", vmi.Namespace, vmi.Name)
			return patchOps, err
		}
		logrus.Infof("VirtualMachineImage %s/%s has no backend specified, defaulting to %q", vmi.Namespace, vmi.Name, backend)
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "add", "path": "/spec/backend", "value": "%s"}`, backend))
		vmi.Spec.Backend = backend
	}

	// Inject the imageDisplayName label at creation time so that the validating
	// webhook's CheckDisplayName can always detect duplicate display names via
	// label selector, regardless of whether the image upload succeeds or fails.
	if len(vmi.Labels) == 0 {
		patchOps = append(patchOps, `{"op": "add", "path": "/metadata/labels", "value": {}}`)
	}
	escapedLabel := patch.EscapeJSONPointer(util.LabelImageDisplayName)
	patchOps = append(patchOps, fmt.Sprintf(`{"op": "add", "path": "/metadata/labels/%s", "value": "%s"}`, escapedLabel, vmi.Spec.DisplayName))

	mutatePatches, err := m.mutators[util.GetVMIBackend(vmi)].Create(vmi)
	if err != nil {
		return patchOps, err
	}

	patchOps = append(patchOps, mutatePatches...)
	return patchOps, nil
}

// defaultBackend assumes management nodes share the installer's Longhorn default disk choice;
// worker nodes are ignored.
func (m *virtualMachineImageMutator) defaultBackend() (harvesterv1.VMIBackend, error) {
	nodes, err := m.nodeCache.List(labels.Everything())
	if err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}
	for _, node := range nodes {
		if util.IsManagementRole(node) && node.Labels[longhorntypes.NodeCreateDefaultDiskLabelKey] == "false" {
			return harvesterv1.VMIBackendCDI, nil
		}
	}
	return harvesterv1.VMIBackendBackingImage, nil
}

func (m *virtualMachineImageMutator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	newVMI := newObj.(*harvesterv1.VirtualMachineImage)
	oldVMI := oldObj.(*harvesterv1.VirtualMachineImage)

	var patchOps types.PatchOps
	if newVMI.Spec.Backend == "" {
		backend := oldVMI.Spec.Backend
		if backend == "" {
			var err error
			backend, err = m.defaultBackend()
			if err != nil {
				return nil, err
			}
		}
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "add", "path": "/spec/backend", "value": "%s"}`, backend))
		newVMI.Spec.Backend = backend
	}

	mutatePatches, err := m.mutators[util.GetVMIBackend(newVMI)].Update(oldVMI, newVMI)
	if err != nil {
		return nil, err
	}

	patchOps = append(patchOps, mutatePatches...)
	return patchOps, nil
}
