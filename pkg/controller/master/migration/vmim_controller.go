package migration

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

const (
	StateMigrating         = "Migrating"
	StateAbortingMigration = "Aborting migration"
)

// The handler adds the AnnotationMigrationUID annotation to the VMI when vmim starts.
// This is mainly for the period when vmim is created but VMI.status.migrationState is not updated before
// the target pod is running.

func (h *Handler) OnVmimChanged(_ string, vmim *kubevirtv1.VirtualMachineInstanceMigration) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	if vmim == nil {
		return nil, nil
	}
	vmi, err := h.vmiCache.Get(vmim.Namespace, vmim.Spec.VMIName)
	if err != nil {
		return vmim, err
	}
	abortRequested := false
	for _, cond := range vmim.Status.Conditions {
		if cond.Type == kubevirtv1.VirtualMachineInstanceMigrationAbortRequested && cond.Status == corev1.ConditionTrue {
			abortRequested = true
		}
	}
	logrus.Debugf("syncing vmim for migration annotation, phase: %v,abortRequested: %v", vmim.Status.Phase, abortRequested)
	if vmim.Status.Phase != kubevirtv1.MigrationFailed && abortRequested {
		if err := h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StateAbortingMigration); err != nil {
			return vmim, err
		}
	} else if vmim.Status.Phase == kubevirtv1.MigrationScheduling {
		return vmim, h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StateMigrating)
	} else if vmi.Annotations[util.AnnotationMigrationUID] == string(vmim.UID) && vmim.Status.Phase == kubevirtv1.MigrationFailed {
		// There are cases when VMIM failed but the status is not reported in VMI.status.migrationState
		// https://github.com/kubevirt/kubevirt/issues/5503
		if err := h.resetHarvesterMigrationStateInVMI(vmi); err != nil {
			return vmim, err
		}
	}
	return vmim, nil
}

func (h *Handler) setVmiMigrationUIDAnnotation(vmi *kubevirtv1.VirtualMachineInstance, UID string, state string) error {
	if vmi.Annotations[util.AnnotationMigrationUID] == UID &&
		vmi.Annotations[util.AnnotationMigrationState] == state {
		return nil
	}
	toUpdate := vmi.DeepCopy()
	if toUpdate.Annotations == nil {
		toUpdate.Annotations = make(map[string]string)
	}
	if UID != "" {
		toUpdate.Annotations[util.AnnotationMigrationUID] = UID
		toUpdate.Annotations[util.AnnotationMigrationState] = state
	} else {
		delete(toUpdate.Annotations, util.AnnotationMigrationUID)
		delete(toUpdate.Annotations, util.AnnotationMigrationState)
	}
	if err := util.VirtClientUpdateVmi(context.Background(), h.restClient, h.namespace, vmi.Namespace, vmi.Name, toUpdate); err != nil {
		return err
	}
	return h.syncVM(vmi)
}

// syncVM update vm so that UI gets websocket message with updated actions
func (h *Handler) syncVM(vmi *kubevirtv1.VirtualMachineInstance) error {
	vm, err := h.vmCache.Get(vmi.Namespace, vmi.Name)
	if err != nil {
		return err
	}
	toUpdateVM := vm.DeepCopy()
	if toUpdateVM.Annotations == nil {
		toUpdateVM.Annotations = make(map[string]string)
	}
	toUpdateVM.Annotations[util.AnnotationTimestamp] = time.Now().Format(time.RFC3339)

	_, err = h.vms.Update(toUpdateVM)
	return err
}
