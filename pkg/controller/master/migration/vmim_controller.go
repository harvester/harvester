package migration

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
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
		return vmim, err
	}

	// restore the resource quota when the migration is completed
	if err := h.restoreResourceQuota(vmim, vmi); err != nil {
		return vmim, err
	}

	abortRequested := isAbortRequest(vmim)
	phase := vmim.Status.Phase
	// debug log shows, after vmim was deleted, the OnChange here may be called two times
	logrus.Debugf("syncing vmim %s/%s/%s phase %v abortRequested %v deleted %t", vmim.Namespace, vmim.Name, vmi.Name, phase, abortRequested, vmim.DeletionTimestamp != nil)

	if phase == kubevirtv1.MigrationPhaseUnset && !abortRequested {
		// set StatePending for UI status showing and menu rendering
		return vmim, h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StatePending)
	} else if phase == kubevirtv1.MigrationPending && !abortRequested {
		err := h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StatePending)
		if err != nil {
			return nil, err
		}
		return vmim, h.scaleResourceQuota(vmi)
	} else if phase != kubevirtv1.MigrationFailed && abortRequested {
		return vmim, h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StateAbortingMigration)
	} else if phase == kubevirtv1.MigrationScheduling || phase == kubevirtv1.MigrationScheduled || phase == kubevirtv1.MigrationPreparingTarget || phase == kubevirtv1.MigrationTargetReady || phase == kubevirtv1.MigrationRunning {
		return vmim, h.setVmiMigrationUIDAnnotation(vmi, string(vmim.UID), StateMigrating)
	} else if vmi.Annotations[util.AnnotationMigrationUID] == string(vmim.UID) && phase == kubevirtv1.MigrationFailed {
		// There are cases when VMIM failed but the status is not reported in VMI.status.migrationState
		// https://github.com/kubevirt/kubevirt/issues/5503
		if err := h.resetHarvesterMigrationStateInVmiAndSyncVM(vmi); err != nil {
			logrus.Infof("vmim %s/%s/%s has MigrationFailed but fail to reset vmi state %s", vmim.Namespace, vmim.Name, vmi.Name, err.Error())
			return vmim, err
		}
		return vmim, nil
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
	delete(toUpdateVM.Spec.Template.Spec.NodeSelector, corev1.LabelHostname)

	_, err = h.vms.Update(toUpdateVM)
	return err
}

// scaleResourceQuota scales the ResourceQuota of the namespace to allow the migration to succeed
func (h *Handler) scaleResourceQuota(vmi *kubevirtv1.VirtualMachineInstance) error {
	// If the namespace is not managed by the resource quota, skip scaling
	if exist, err := h.isNamespaceManagedByResourceQuota(vmi.Namespace); !exist && err == nil {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check if the namespace is managed by the resource quota: %v", err)
	}

	// Scale ResourceQuota through VMI resource specifications
	if err := h.scaleResourceQuotaWithVMI(vmi); err != nil {
		return fmt.Errorf("failed to scale resource quota with vmi: %v", err)
	}

	return nil
}

// isNamespaceManagedByResourceQuota checks if the namespace is managed by the resource quota
func (h *Handler) isNamespaceManagedByResourceQuota(namespace string) (bool, error) {
	rqs, err := h.rqCache.List(namespace, labels.Everything())
	if err != nil {
		return false, err
	}

	// If there is any resource quota in the namespace,
	// return true check it if finding cpu or memory limit.
	for _, v := range rqs {
		if v.Spec.Hard.Cpu() != nil || v.Spec.Hard.Memory() != nil {
			return true, nil
		}
	}

	return false, nil
}

// scaleResourceQuotaWithVMI scales ResourceQuota through VMI resource specifications
func (h *Handler) scaleResourceQuotaWithVMI(vmi *kubevirtv1.VirtualMachineInstance) error {
	selector := labels.Set{util.LabelManagementDefaultResourceQuota: "true"}.AsSelector()
	rqs, err := h.rqCache.List(vmi.Namespace, selector)
	if err != nil {
		return err
	} else if len(rqs) == 0 {
		logrus.Debugf("scaleResourceQuotaWithVMI: can't find any default resource quota, skip updating namespace %s", vmi.Namespace)
		return nil
	}

	rqCpy := rqs[0].DeepCopy()
	if ok := rqutils.ContainsMigratingVM(rqCpy, vmi.Name, string(vmi.UID)); ok {
		logrus.Debugf("scaleResourceQuotaWithVMI: the resource quota in the namespace %s and vm %s is already scaled, skip updating", vmi.Namespace, vmi.Name)
		return nil
	}

	// only update to ResourceQuota annotation, do not change ResourceQuota spec directly in this step
	needUpdate, rqToUpdate, rl := rqutils.CalculateScaleResourceQuotaWithVMI(rqCpy, vmi, util.GetAdditionalGuestMemoryOverheadRatioWithoutError(h.settingCache))
	if !needUpdate {
		logrus.Debugf("scaleResourceQuotaWithVMI: no need to update resource quota, skip updating namespace %s and vm %s", vmi.Namespace, vmi.Name)
		return nil
	}

	// add migrating vmi information to ResourceQuota
	if err := rqutils.AddMigratingVM(rqToUpdate, vmi.Name, string(vmi.UID), rl); err != nil {
		return err
	}
	_, err = h.rqs.Update(rqToUpdate)
	return err
}

// restoreResourceQuota restores the ResourceQuota when the migration is completed
func (h *Handler) restoreResourceQuota(vmim *kubevirtv1.VirtualMachineInstanceMigration, vmi *kubevirtv1.VirtualMachineInstance) error {
	if vmim.Status.Phase != kubevirtv1.MigrationFailed && vmim.Status.Phase != kubevirtv1.MigrationSucceeded {
		return nil
	}

	// If the namespace is not managed by the ResourceQuota, skip scaling
	if exist, err := h.isNamespaceManagedByResourceQuota(vmi.Namespace); !exist && err == nil {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check if the namespace is managed by the resource quota: %v", err)
	}

	// restore ResourceQuota through vmi resource specifications
	if err := h.restoreResourceQuotaWithVMI(vmi); err != nil {
		return fmt.Errorf("failed to restore resource quota with vmi: %v", err)
	}

	return nil
}

// restoreResourceQuotaWithVMI restores the ResourceQuota when the migration is completed
func (h *Handler) restoreResourceQuotaWithVMI(vmi *kubevirtv1.VirtualMachineInstance) error {
	selector := labels.Set{util.LabelManagementDefaultResourceQuota: "true"}.AsSelector()
	rqs, err := h.rqCache.List(vmi.Namespace, selector)
	if err != nil {
		return err
	} else if len(rqs) == 0 {
		logrus.Debugf("restoreResourceQuotaWithVMI: can't find any default resource quota, skip updating namespace %s", vmi.Namespace)
		return nil
	}

	rqCpy := rqs[0].DeepCopy()

	// delete migrating vmi information from ResourceQuota annotation, then the ResourceQuota controller will re-calculate the final value
	if rqutils.DeleteMigratingVM(rqCpy, vmi.Name, string(vmi.UID)) {
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

// https://github.com/harvester/harvester/issues/6193
// The UI VM page is anchored on VM object, only when VM object's resorceVersion is changed
// then UI will update the VM action menu
// VM's migration is done mainly between vmi and vmim
// Propagate the changes to VM when necessary to keep UI updated
func (h *Handler) resetHarvesterMigrationStateInVmiAndSyncVM(vmi *kubevirtv1.VirtualMachineInstance) error {
	if err := h.resetHarvesterMigrationStateInVMI(vmi); err != nil {
		return fmt.Errorf("fail to reset vmi migration %w", err)
	}
	// without syncing VM, the UI may still show the actions deduced from old VM/VMI status combination
	if err := h.syncVM(vmi); err != nil {
		return fmt.Errorf("fail to reset vmi migration to vm %w", err)
	}
	return nil
}
