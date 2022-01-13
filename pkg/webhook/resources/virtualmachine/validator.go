package virtualmachine

import (
	"encoding/json"
	"errors"
	"fmt"

	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/client-go/api/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(
	pvcCache v1.PersistentVolumeClaimCache,
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache,
) types.Validator {
	return &vmValidator{
		pvcCache:      pvcCache,
		vmBackupCache: vmBackupCache,
	}
}

type vmValidator struct {
	types.DefaultValidator
	pvcCache      v1.PersistentVolumeClaimCache
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache
}

func (v *vmValidator) Resource() types.Resource {
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

func (v *vmValidator) Create(request *types.Request, newObj runtime.Object) error {
	vm := newObj.(*kubevirtv1.VirtualMachine)

	if err := v.checkVMSpec(vm); err != nil {
		return err
	}
	return nil
}

func (v *vmValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	vm := newObj.(*kubevirtv1.VirtualMachine)

	if err := v.checkVMSpec(vm); err != nil {
		return err
	}
	return nil
}

func (v *vmValidator) checkVMSpec(vm *kubevirtv1.VirtualMachine) error {
	if err := v.checkVolumeClaimTemplatesAnnotation(vm); err != nil {
		message := fmt.Sprintf("the volumeClaimTemplates annotaion is invalid: %v", err)
		return werror.NewInvalidError(message, "metadata.annotations")
	}
	if err := v.checkOccupiedPVCs(vm); err != nil {
		return err
	}
	return nil
}

func (v *vmValidator) checkVolumeClaimTemplatesAnnotation(vm *kubevirtv1.VirtualMachine) error {
	if vm == nil {
		return nil
	}
	volumeClaimTemplates, ok := vm.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok || volumeClaimTemplates == "" {
		return nil
	}
	var pvcs []*corev1.PersistentVolumeClaim
	if err := json.Unmarshal([]byte(volumeClaimTemplates), &pvcs); err != nil {
		return err
	}
	for _, pvc := range pvcs {
		if pvc.Name == "" {
			return errors.New("PVC name is required")
		}
	}
	return nil
}

func (v *vmValidator) checkOccupiedPVCs(vm *kubevirtv1.VirtualMachine) error {
	if vm == nil {
		return nil
	}
	vmID := ref.Construct(vm.Namespace, vm.Name)
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvc, err := v.pvcCache.Get(vm.Namespace, volume.PersistentVolumeClaim.ClaimName)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}
			owners, err := ref.GetSchemaOwnersFromAnnotation(pvc)
			if err != nil {
				return err
			}
			ids := owners.List(kubevirtv1.VirtualMachineGroupVersionKind.GroupKind())

			for _, id := range ids {
				if id != vmID {
					message := fmt.Sprintf("the volume %s is already used by VM %s", volume.PersistentVolumeClaim.ClaimName, id)
					return werror.NewInvalidError(message, "spec.templates.spec.volumes")
				}
			}
		}
	}

	return nil
}
