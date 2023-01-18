package virtualmachine

import (
	"encoding/json"
	"errors"
	"fmt"

	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
	webhookutil "github.com/harvester/harvester/pkg/webhook/util"
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
		Names:      []string{"virtualmachines", "virtualmachines/status"},
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
	if vm == nil {
		return nil
	}

	if err := v.checkVMSpec(vm); err != nil {
		return err
	}
	return nil
}

func (v *vmValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	newVM := newObj.(*kubevirtv1.VirtualMachine)
	if newVM == nil {
		return nil
	}

	if err := v.checkVMSpec(newVM); err != nil {
		return err
	}

	oldVM := oldObj.(*kubevirtv1.VirtualMachine)
	if oldVM == nil {
		return nil
	}

	// Prevent users to stop/restart VM when there is VMBackup in progress.
	if v.checkVMStoppingStatus(oldVM, newVM) {
		if err := v.checkVMBackup(newVM); err != nil {
			return err
		}
	}

	// Check resize volumes
	return v.checkResizeVolumes(oldVM, newVM)
}

func (v *vmValidator) checkVMSpec(vm *kubevirtv1.VirtualMachine) error {
	if err := v.checkVolumeClaimTemplatesAnnotation(vm); err != nil {
		message := fmt.Sprintf("the volumeClaimTemplates annotaion is invalid: %v", err)
		return werror.NewInvalidError(message, "metadata.annotations")
	}
	if err := v.checkOccupiedPVCs(vm); err != nil {
		return err
	}
	if err := v.checkReservedMemoryAnnotation(vm); err != nil {
		return err
	}
	return nil
}

func (v *vmValidator) checkVMStoppingStatus(oldVM *kubevirtv1.VirtualMachine, newVM *kubevirtv1.VirtualMachine) bool {
	oldRunStrategy, _ := oldVM.RunStrategy()
	newRunStrategy, _ := newVM.RunStrategy()

	// KubeVirt send stop request or change running from true to false when users stop a VM.
	// use runStrategy to determine state rather than "running"
	if oldRunStrategy == kubevirtv1.RunStrategyAlways && newRunStrategy == kubevirtv1.RunStrategyHalted {
		return true
	}

	// KubeVirt send restart request (stop and start combination request) when users restart a VM.
	// Stop reference: https://github.com/kubevirt/kubevirt/blob/c9e87c4cb6292af33ccad8faa5fb9bf269c0fbf4/pkg/virt-api/rest/subresource.go#L508-L511
	// Restart reference: https://github.com/kubevirt/kubevirt/blob/c9e87c4cb6292af33ccad8faa5fb9bf269c0fbf4/pkg/virt-api/rest/subresource.go#L260-L263
	if len(newVM.Status.StateChangeRequests) != 0 && newVM.Status.StateChangeRequests[0].Action == kubevirtv1.StopRequest {
		return true
	}

	return false
}

func (v *vmValidator) checkVolumeClaimTemplatesAnnotation(vm *kubevirtv1.VirtualMachine) error {
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

func (v *vmValidator) checkResizeVolumes(oldVM, newVM *kubevirtv1.VirtualMachine) error {
	if oldVM.Annotations[util.AnnotationVolumeClaimTemplates] == "" || newVM.Annotations[util.AnnotationVolumeClaimTemplates] == "" {
		return nil
	}

	var oldPvcs, newPvcs []*corev1.PersistentVolumeClaim
	if err := json.Unmarshal([]byte(oldVM.Annotations[util.AnnotationVolumeClaimTemplates]), &oldPvcs); err != nil {
		return werror.NewInvalidError(fmt.Sprintf("failed to unmarshal %s", oldVM.Annotations[util.AnnotationVolumeClaimTemplates]), fmt.Sprintf("metadata.annotations.%s", util.AnnotationVolumeClaimTemplates))
	}
	if err := json.Unmarshal([]byte(newVM.Annotations[util.AnnotationVolumeClaimTemplates]), &newPvcs); err != nil {
		return werror.NewInvalidError(fmt.Sprintf("failed to unmarshal %s", newVM.Annotations[util.AnnotationVolumeClaimTemplates]), fmt.Sprintf("metadata.annotations.%s", util.AnnotationVolumeClaimTemplates))
	}

	oldPvcMap, newPvcMap := map[string]*corev1.PersistentVolumeClaim{}, map[string]*corev1.PersistentVolumeClaim{}
	for _, pvc := range oldPvcs {
		oldPvcMap[pvc.Name] = pvc
	}
	for _, pvc := range newPvcs {
		newPvcMap[pvc.Name] = pvc
	}

	isResizeVolume := false
	for name, oldPvc := range oldPvcMap {
		newPvc, ok := newPvcMap[name]
		if !ok || newPvc == nil {
			continue
		}

		// ref: https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity.Cmp
		// -1 means newPvc < oldPvc
		// 1 means newPVC > oldPVC
		if newPvc.Spec.Resources.Requests.Storage().Cmp(*oldPvc.Spec.Resources.Requests.Storage()) == -1 {
			return werror.NewInvalidError(fmt.Sprintf("%s PVC requests storage can't be less than previous value", newPvc.Name), fmt.Sprintf("metadata.annotations.%s", util.AnnotationVolumeClaimTemplates))
		} else if newPvc.Spec.Resources.Requests.Storage().Cmp(*oldPvc.Spec.Resources.Requests.Storage()) == 1 {
			isResizeVolume = true
		}
	}

	if isResizeVolume {
		runStrategy, err := newVM.RunStrategy()
		if err != nil {
			return werror.NewInternalError(fmt.Sprintf("failed to get run strategy, err: %s", err.Error()))
		}
		if runStrategy != kubevirtv1.RunStrategyHalted {
			return werror.NewInvalidError("please stop the VM before resizing volumes", fmt.Sprintf("metadata.annotations.%s", util.AnnotationVolumeClaimTemplates))
		}
	}
	return nil
}

func (v *vmValidator) checkOccupiedPVCs(vm *kubevirtv1.VirtualMachine) error {
	vmID := ref.Construct(vm.Namespace, vm.Name)
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvc, err := v.pvcCache.Get(vm.Namespace, volume.PersistentVolumeClaim.ClaimName)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return werror.NewInternalError(err.Error())
			}
			owners, err := ref.GetSchemaOwnersFromAnnotation(pvc)
			if err != nil {
				return werror.NewInvalidError(err.Error(), "spec.templates.spec.volumes")
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

func (v *vmValidator) checkVMBackup(vm *kubevirtv1.VirtualMachine) error {
	if exist, err := webhookutil.HasInProgressingVMBackupBySourceUID(v.vmBackupCache, string(vm.UID)); err != nil {
		return werror.NewInternalError(err.Error())
	} else if exist {
		return werror.NewBadRequest(fmt.Sprintf("there is vmbackup in progress for vm %s/%s, please wait for the vmbackup or remove it before stop/restart the vm", vm.Namespace, vm.Name))
	}
	return nil
}

func (v *vmValidator) checkReservedMemoryAnnotation(vm *kubevirtv1.VirtualMachine) error {
	mem := vm.Spec.Template.Spec.Domain.Resources.Limits.Memory()
	if mem.IsZero() {
		return nil
	}

	reservedMemoryStr, ok := vm.Annotations[util.AnnotationReservedMemory]
	if !ok || reservedMemoryStr == "" {
		return nil
	}

	field := fmt.Sprintf("metadata.annotations[%s]", util.AnnotationReservedMemory)
	reservedMemory, err := resource.ParseQuantity(reservedMemoryStr)
	if err != nil {
		return werror.NewInvalidError(err.Error(), field)
	}
	if reservedMemory.Cmp(*mem) >= 0 {
		return werror.NewInvalidError("reservedMemory cannot be equal or greater than limits.memory", field)
	}
	if reservedMemory.CmpInt64(0) == -1 {
		return werror.NewInvalidError("reservedMemory cannot be less than 0", field)
	}
	return nil
}
