package virtualmachine

import (
	"fmt"

	kubevirtapis "kubevirt.io/client-go/api/v1"
	cdiapis "kubevirt.io/containerized-data-importer/pkg/apis/core/v1beta1"

	cdictrl "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
)

// setOwnerlessDataVolumeReference tries to set the target VirtualMachine as the annotation schema owner of the DataVolume.
func setOwnerlessDataVolumeReference(dataVolumeClient cdictrl.DataVolumeClient, dv *cdiapis.DataVolume, vm *kubevirtapis.VirtualMachine) error {
	if vm == nil || dv == nil || dv.DeletionTimestamp != nil {
		return nil
	}

	var annotationSchemaOwners, err = ref.GetSchemaOwnersFromAnnotation(dv)
	if err != nil {
		return fmt.Errorf("failed to get schema owners from annotation: %w", err)
	}

	var notOwned = annotationSchemaOwners.Add(kubevirtapis.VirtualMachineGroupVersionKind.GroupKind(), vm)
	if !notOwned {
		return nil
	}

	dv = dv.DeepCopy()
	if err := annotationSchemaOwners.Bind(dv); err != nil {
		return fmt.Errorf("failed to apply schema owners to object: %w", err)
	}
	_, err = dataVolumeClient.Update(dv)
	return err
}

// unsetBoundedDataVolumeReference tries to unset the DataVolume's annotation schema owner of the target VirtualMachine.
func unsetBoundedDataVolumeReference(dataVolumeClient cdictrl.DataVolumeClient, dv *cdiapis.DataVolume, vm *kubevirtapis.VirtualMachine) error {
	if vm == nil || dv == nil || dv.DeletionTimestamp != nil {
		return nil
	}

	var annotationSchemaOwners, err = ref.GetSchemaOwnersFromAnnotation(dv)
	if err != nil {
		return fmt.Errorf("failed to get schema owners from object: %w", err)
	}

	var isOwned = annotationSchemaOwners.Remove(kubevirtapis.VirtualMachineGroupVersionKind.GroupKind(), vm)
	if !isOwned {
		return nil
	}

	dv = dv.DeepCopy()
	if err := annotationSchemaOwners.Bind(dv); err != nil {
		return fmt.Errorf("failed to apply schema owners to object: %w", err)
	}
	_, err = dataVolumeClient.Update(dv)
	return err
}
