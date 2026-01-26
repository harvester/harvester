package migration

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
	rqutils "github.com/harvester/harvester/pkg/util/resourcequota"
)

const (
	// StateMigrating represents kubevirt MigrationScheduling, MigrationScheduled, MigrationPreparingTarget, MigrationTargetReady, MigrationRunning
	StateMigrating = "Migrating"

	StateAbortingMigration = "Aborting migration"

	// StatePending represents kubevirt MigrationPhaseUnset, MigrationPending
	StatePending = "Pending migration"
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
		// the vmim might be a leftover object when it's parent vmi object was deleted
		if apierrors.IsNotFound(err) {
			return vmim, nil
		}
		return nil, err
	}

	// when a migration is aborted, the phase will transfer to kubevirtv1.MigrationFailed finally
	abortRequested := isAbortRequest(vmim)
	phase := vmim.Status.Phase
	// debug log shows, after vmim was deleted, the OnChange here may be called two times
	logrus.Debugf("syncing vmim %s/%s/%s phase %v abortRequested %v deleted %t", vmim.Namespace, vmim.Name, vmi.Name, phase, abortRequested, vmim.DeletionTimestamp != nil)

	switch phase {
	case kubevirtv1.MigrationPhaseUnset:
		if abortRequested {
			return vmim, h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StateAbortingMigration)
		}
		return vmim, h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StatePending)
	case kubevirtv1.MigrationPending:
		if abortRequested {
			return vmim, h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StateAbortingMigration)
		}
		return h.handlePendingMigration(vmi, vmim)
	case kubevirtv1.MigrationScheduling,
		kubevirtv1.MigrationScheduled,
		kubevirtv1.MigrationPreparingTarget,
		kubevirtv1.MigrationTargetReady,
		kubevirtv1.MigrationRunning:
		if abortRequested {
			return vmim, h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StateAbortingMigration)
		}
		return vmim, h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StateMigrating)
	case kubevirtv1.MigrationFailed, kubevirtv1.MigrationSucceeded:
		// restore the resource quota and clear vmi migration related information
		return h.handleFinishedMigration(vmi, vmim)
	}

	return vmim, nil
}

// when vmim is pending
func (h *Handler) handlePendingMigration(vmi *kubevirtv1.VirtualMachineInstance, vmim *kubevirtv1.VirtualMachineInstanceMigration) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	err := h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StatePending)
	if err != nil {
		return nil, err
	}

	// scale ResourceQuota through VMI resource specifications
	if err := h.scaleResourceQuotaWithVMI(vmi); err != nil {
		return nil, fmt.Errorf("failed to scale resource quota with vmi %s/%s: %w", vmi.Namespace, vmi.Name, err)
	}

	// if vmim is blocked by RQ, it is on phase MigrationPending
	err = h.compensatePendingMigration(vmim, vmi)
	if err != nil {
		return nil, err
	}
	return vmim, nil
}

// when vmim is finished
func (h *Handler) handleFinishedMigration(vmi *kubevirtv1.VirtualMachineInstance, vmim *kubevirtv1.VirtualMachineInstanceMigration) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	// restore the resource quota when the migration is completed
	if err := h.restoreResourceQuotaWithVMI(vmi); err != nil {
		logrus.Infof("vmim %s/%s is on phase %s but fail to restore vmi %s on resource quota %s", vmim.Namespace, vmim.Name, vmim.Status.Phase, vmi.Name, err.Error())
		return nil, err
	}
	if err := h.resetHarvesterMigrationStateInVmiAndSyncVM(vmi); err != nil {
		logrus.Infof("vmim %s/%s is on phase %s but fail to reset vmi %s state %s", vmim.Namespace, vmim.Name, vmim.Status.Phase, vmi.Name, err.Error())
		return nil, err
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
		// headless vmi
		if apierrors.IsNotFound(err) {
			return nil
		}
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

// scaleResourceQuotaWithVMI scales ResourceQuota through VMI resource specifications
func (h *Handler) scaleResourceQuotaWithVMI(vmi *kubevirtv1.VirtualMachineInstance) error {
	rq, err := h.getResourceQuotaFromNamespace(vmi.Namespace)
	if err != nil {
		return err
	}
	if rq == nil {
		return nil
	}
	// already included
	if rqutils.ContainsMigratingVM(rq, string(vmi.UID)) {
		return nil
	}

	rqCpy := rq.DeepCopy()
	// only update to ResourceQuota annotation, do not change ResourceQuota spec directly in this step
	needUpdate, rqToUpdate, rl := rqutils.CalculateScaleResourceQuotaWithVMI(rqCpy, vmi, util.GetAdditionalGuestMemoryOverheadRatioWithoutError(h.settingCache))
	if !needUpdate {
		return nil
	}

	// add migrating vmi information to ResourceQuota
	if err := rqutils.AddMigratingVM(rqToUpdate, vmi.Name, string(vmi.UID), rl); err != nil {
		return err
	}
	logrus.Debugf("scaleResourceQuotaWithVMI: update resource quota with vmi %s/%s", vmi.Namespace, vmi.Name)
	_, err = h.rqs.Update(rqToUpdate)
	return err
}

// getResourceQuotaFromNamespace fetches the first default RQ from given namespace
func (h *Handler) getResourceQuotaFromNamespace(namespace string) (*corev1.ResourceQuota, error) {
	selector := labels.Set{util.LabelManagementDefaultResourceQuota: "true"}.AsSelector()
	rqs, err := h.rqCache.List(namespace, selector)
	if err != nil {
		return nil, err
	}
	if len(rqs) == 0 {
		return nil, nil
	}
	return rqs[0], nil
}

// restoreResourceQuotaWithVMI restores the ResourceQuota when the migration is completed/aborted
func (h *Handler) restoreResourceQuotaWithVMI(vmi *kubevirtv1.VirtualMachineInstance) error {
	rq, err := h.getResourceQuotaFromNamespace(vmi.Namespace)
	if err != nil {
		return err
	}
	if rq == nil {
		return nil
	}
	rqCpy := rq.DeepCopy()

	// delete migrating vmi information from ResourceQuota annotation, then the ResourceQuota controller will re-calculate the final value
	if rqutils.DeleteMigratingVM(rqCpy, vmi.Name, string(vmi.UID)) {
		// when there is compensation, no matter for which vmim, delete it
		// if a pending vmi is blocked due to quota, the rq controller will re-add the compensation
		_ = rqutils.DeleteMigratingCompensation(rqCpy)
		_, err = h.rqs.Update(rqCpy)
		return err
	}
	return nil
}

func isAbortRequest(vmim *kubevirtv1.VirtualMachineInstanceMigration) bool {
	abortCond := kubevirtv1.VirtualMachineInstanceMigrationAbortRequested
	for _, cond := range vmim.Status.Conditions {
		if cond.Type == abortCond && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (h *Handler) resetHarvesterMigrationStateInVMI(vmi *kubevirtv1.VirtualMachineInstance) error {
	toUpdate := vmi.DeepCopy()
	len1 := len(toUpdate.Annotations)
	delete(toUpdate.Annotations, util.AnnotationMigrationUID)
	delete(toUpdate.Annotations, util.AnnotationMigrationState)
	delete(toUpdate.Annotations, util.AnnotationMigrationTarget)
	len2 := len(toUpdate.Annotations)
	if len1 == len2 {
		return nil
	}
	if err := util.VirtClientUpdateVmi(context.Background(), h.restClient, h.namespace, vmi.Namespace, vmi.Name, toUpdate); err != nil {
		return err
	}
	return nil
}

// https://github.com/harvester/harvester/issues/6193
// The UI VM page is anchored on VM object, only when VM object's resorceVersion is changed
// then UI will update the VM action menu
// VM's migration is done mainly between vmi and vmim
// Propagate the changes to VM when necessary to keep UI updated
func (h *Handler) resetHarvesterMigrationStateInVmiAndSyncVM(vmi *kubevirtv1.VirtualMachineInstance) error {
	if err := h.resetHarvesterMigrationStateInVMI(vmi); err != nil {
		return fmt.Errorf("fail to reset vmi migration state %w", err)
	}
	// without syncing VM, the UI may still show the actions deduced from old VM/VMI status combination
	if err := h.syncVM(vmi); err != nil {
		return fmt.Errorf("fail to sync vmi migration state to vm %w", err)
	}
	return nil
}

// compensate the ResourceQuota when the migration is blocked due to quota exceeds limit
func (h *Handler) compensatePendingMigration(vmim *kubevirtv1.VirtualMachineInstanceMigration, vmi *kubevirtv1.VirtualMachineInstance) error {
	if vmim.Status.Conditions == nil {
		return nil
	}
	for _, cond := range vmim.Status.Conditions {
		if cond.Type == kubevirtv1.VirtualMachineInstanceMigrationRejectedByResourceQuota {
			// vmim is blocked due to quota, compensate in special cases
			if cond.Status == corev1.ConditionTrue {
				return h.compensateResourceQuotaBase(vmim, vmi)
			}
			return nil
		}
	}

	return nil
}

// compute the real usage of VM's POD, if it exceeds the hard limit, then compensate the delta
// when user changes the global setting additional-guest-memory-overhead-ratio after the VM is up and then migrates the vm
// the RQ usage can exceed the hard limit
// this function will ensure the already running VMs can still migrate
func (h *Handler) compensateResourceQuotaBase(vmim *kubevirtv1.VirtualMachineInstanceMigration, vmi *kubevirtv1.VirtualMachineInstance) error {
	rq, err := h.getResourceQuotaFromNamespace(vmi.Namespace)
	if err != nil {
		return err
	}
	if rq == nil {
		return nil
	}

	// wait until auto-scaling has been applied to RQ, if it is still not enough, then consider to compensate
	if ok := rqutils.ContainsMigratingVM(rq, string(vmi.UID)); !ok {
		logrus.Debugf("compensateResourceQuotaBase: the resource quota in the namespace %s does not include migrating vm %s resource scaling info, don't compensate, wait", vmi.Namespace, vmi.Name)
		h.vmimController.EnqueueAfter(vmim.Namespace, vmim.Name, 1*time.Second)
		return nil
	}

	rqToUpdate := rq.DeepCopy()

	// only update to ResourceQuota annotation, do not change ResourceQuota spec directly in this step
	needUpdate, rqToUpdate, rl := rqutils.CalculateCompensationResourceQuotaWithVMI(rqToUpdate, vmi, util.GetAdditionalGuestMemoryOverheadRatioWithoutError(h.settingCache))
	if !needUpdate {
		logrus.Debugf("compensateResourceQuotaBase: no need to update resource quota, skip updating namespace %s and vm %s", vmi.Namespace, vmi.Name)
		return nil
	}

	logrus.Debugf("compensateResourceQuotaBase: compensate resource quota %s in namespace %s for vm %s : %v", rq.Name, vmi.Namespace, vmi.Name, rl)
	// add compensation information to ResourceQuota
	if err := rqutils.AddMigratingCompensation(rqToUpdate, rl); err != nil {
		return err
	}
	_, err = h.rqs.Update(rqToUpdate)
	return err
}
