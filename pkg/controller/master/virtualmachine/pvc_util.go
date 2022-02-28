package virtualmachine

import (
	"fmt"

	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/ref"
)

// setOwnerlessPVCReference tries to set the target VirtualMachine as the annotation schema owner of the PVC.
func setOwnerlessPVCReference(pvcClient v1.PersistentVolumeClaimClient, pvc *corev1.PersistentVolumeClaim, vm *kubevirtv1.VirtualMachine) error {
	if vm == nil || pvc == nil || pvc.DeletionTimestamp != nil {
		return nil
	}

	var annotationSchemaOwners, err = ref.GetSchemaOwnersFromAnnotation(pvc)
	if err != nil {
		return fmt.Errorf("failed to get schema owners from annotation: %w", err)
	}

	var notOwned = annotationSchemaOwners.Add(kubevirtv1.VirtualMachineGroupVersionKind.GroupKind(), vm)
	if !notOwned {
		return nil
	}

	pvc = pvc.DeepCopy()
	if err := annotationSchemaOwners.Bind(pvc); err != nil {
		return fmt.Errorf("failed to apply schema owners to object: %w", err)
	}
	_, err = pvcClient.Update(pvc)
	return err
}

// numberOfBoundedPVCReference tries to get the number of bounded references on a PVC
func numberOfBoundedPVCReference(pvc *corev1.PersistentVolumeClaim) (int, error) {
	annotationSchemaOwners, err := ref.GetSchemaOwnersFromAnnotation(pvc)
	if err != nil {
		return 0, fmt.Errorf("failed to get schema owners from object: %w", err)
	}
	ownerGroupKind := kubevirtv1.VirtualMachineGroupVersionKind.GroupKind()
	length := len(annotationSchemaOwners.List(ownerGroupKind))
	return length, nil
}

// unsetBoundedPVCReference tries to unset the PVC's annotation schema owner of the target VirtualMachine.
func unsetBoundedPVCReference(pvcClient v1.PersistentVolumeClaimClient, pvc *corev1.PersistentVolumeClaim, vm *kubevirtv1.VirtualMachine) error {
	if vm == nil || pvc == nil || pvc.DeletionTimestamp != nil {
		return nil
	}

	var annotationSchemaOwners, err = ref.GetSchemaOwnersFromAnnotation(pvc)
	if err != nil {
		return fmt.Errorf("failed to get schema owners from object: %w", err)
	}

	var isOwned = annotationSchemaOwners.Remove(kubevirtv1.VirtualMachineGroupVersionKind.GroupKind(), vm)
	if !isOwned {
		return nil
	}

	pvc = pvc.DeepCopy()
	if err := annotationSchemaOwners.Bind(pvc); err != nil {
		return fmt.Errorf("failed to apply schema owners to object: %w", err)
	}
	_, err = pvcClient.Update(pvc)
	return err
}
