package migration

import (
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
	rqutils "github.com/harvester/harvester/pkg/util/resourcequota"
)

const (
	// MigrationRunning
	StateMigrating         = "Migrating"
	StateAbortingMigration = "Aborting migration"
	// kubevirt MigrationPhaseUnset, MigrationPending, MigrationScheduling, MigrationScheduled, kubevirt MigrationPreparingTarget, MigrationTargetReady
	StateScheduling = "Scheduling"
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

	errWraper := func(uid, state, phase string, err error) error {
		return fmt.Errorf("fail to set vmim UID %v state %v to vmi in phase %v, %w", uid, state, phase, err)
	}

	// debug log shows, after vmim was deleted, the OnChange here may be called two times
	logrus.Debugf("syncing vmim %s/%s/%s phase %v abortRequested %v deleted %t", vmim.Namespace, vmim.Name, vmi.Name, vmim.Status.Phase, abortRequested, vmim.DeletionTimestamp != nil)

	switch phase := vmim.Status.Phase; phase {
	case kubevirtv1.MigrationPhaseUnset, kubevirtv1.MigrationPending, kubevirtv1.MigrationScheduling, kubevirtv1.MigrationScheduled, kubevirtv1.MigrationPreparingTarget, kubevirtv1.MigrationTargetReady:
		if !abortRequested {
			if err := h.setVmiMigrationUIDAnnotationAndSyncVM(vmi, string(vmim.UID), StateScheduling); err != nil {
				return vmim, errWraper(string(vmim.UID), StateScheduling, string(phase), err)
			}
			return vmim, h.scaleResourceQuota(vmi)
		}
		// aborted:
		if err := h.setVmiMigrationUIDAnnotationAndSyncVM(vmi, string(vmim.UID), StateAbortingMigration); err != nil {
			return vmim, errWraper(string(vmim.UID), StateAbortingMigration, string(phase), err)
		}
		return vmim, nil

	case kubevirtv1.MigrationRunning:
		if !abortRequested {
			if err := h.setVmiMigrationUIDAnnotationAndSyncVM(vmi, string(vmim.UID), StateMigrating); err != nil {
				return vmim, errWraper(string(vmim.UID), StateMigrating, string(phase), err)
			}
			return vmim, nil
		}
		// aborted:
		if err := h.setVmiMigrationUIDAnnotationAndSyncVM(vmi, string(vmim.UID), StateAbortingMigration); err != nil {
			return vmim, errWraper(string(vmim.UID), StateAbortingMigration, string(phase), err)
		}
		return vmim, nil

	case kubevirtv1.MigrationFailed:
		if vmi.Annotations[util.AnnotationMigrationUID] == string(vmim.UID) {
			// There are cases when VMIM failed but the status is not reported in VMI.status.migrationState
			// https://github.com/kubevirt/kubevirt/issues/5503
			if err := h.resetHarvesterMigrationStateInVmiAndSyncVM(vmi); err != nil {
				logrus.Infof("vmim %s/%s/%s has MigrationFailed but fail to reset vmi state %s", vmim.Namespace, vmim.Name, vmi.Name, err.Error())
				return vmim, err
			}
		}
		return vmim, nil

	case kubevirtv1.MigrationSucceeded:
		return vmim, nil

	default:
		return vmim, nil
	}
}

// scaleResourceQuota scales the resource quota of the namespace to allow the migration to succeed
func (h *Handler) scaleResourceQuota(vmi *kubevirtv1.VirtualMachineInstance) error {
	// If the namespace is not managed by the resource quota, skip scaling
	if exist, err := h.isNamespaceManagedByResourceQuota(vmi.Namespace); exist == false && err == nil {
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

// scaleResourceQuotaWithVMI Scaling ResourceQuota through VMI resource specifications
func (h *Handler) scaleResourceQuotaWithVMI(vmi *kubevirtv1.VirtualMachineInstance) error {
	// Scale ResourceQuota through VMI resource specifications
	selector := labels.Set{util.LabelManagementDefaultResourceQuota: "true"}.AsSelector()
	rqs, err := h.rqCache.List(vmi.Namespace, selector)
	if err != nil {
		return err
	} else if len(rqs) == 0 {
		logrus.Debugf("scaleResourceQuotaWithVMI: can not find any default resource quota, skip updating namespace %s", vmi.Namespace)
		return nil
	}

	rqCpy := rqs[0].DeepCopy()
	if ok := rqutils.ContainsMigratingVM(rqCpy, vmi.Name); ok {
		logrus.Debugf("scaleResourceQuotaWithVMI: the resource quota in the namespace %s and vm %s is already scaled, skip updating", vmi.Namespace, vmi.Name)
		return nil
	}

	needUpdate, rqToUpdate, rl := rqutils.CalculateScaleResourceQuotaWithVMI(rqCpy, vmi)
	if !needUpdate {
		logrus.Debugf("scaleResourceQuotaWithVMI: no need to update resource quota, skip updating namespace %s and vm %s", vmi.Namespace, vmi.Name)
		return nil
	}

	// Update resource quota
	if err := rqutils.UpdateMigratingVM(rqToUpdate, vmi.Name, rl); err != nil {
		return err
	}
	_, err = h.rqs.Update(rqToUpdate)
	fmt.Println("scaled")
	return err
}

// restoreResourceQuota restores the resource quota when the migration is completed
func (h *Handler) restoreResourceQuota(vmim *kubevirtv1.VirtualMachineInstanceMigration, vmi *kubevirtv1.VirtualMachineInstance) error {
	if vmim.Status.Phase != kubevirtv1.MigrationFailed && vmim.Status.Phase != kubevirtv1.MigrationSucceeded {
		return nil
	}

	// If the namespace is not managed by the resource quota, skip scaling
	if exist, err := h.isNamespaceManagedByResourceQuota(vmi.Namespace); exist == false && err == nil {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check if the namespace is managed by the resource quota: %v", err)
	}

	// Restore ResourceQuota through VMI resource specifications
	if err := h.restoreResourceQuotaWithVMI(vmi); err != nil {
		return fmt.Errorf("failed to restore resource quota with vmi: %v", err)
	}

	return nil
}

// restoreResourceQuotaWithVMI restores the resource quota when the migration is completed
func (h *Handler) restoreResourceQuotaWithVMI(vmi *kubevirtv1.VirtualMachineInstance) error {
	// Restore ResourceQuota through VMI resource specifications
	selector := labels.Set{util.LabelManagementDefaultResourceQuota: "true"}.AsSelector()
	rqs, err := h.rqCache.List(vmi.Namespace, selector)
	if err != nil {
		return err
	} else if len(rqs) == 0 {
		logrus.Debugf("restoreResourceQuotaWithVMI: can not find any default resource quota, skip updating namespace %s", vmi.Namespace)
		return nil
	}

	rqCpy := rqs[0].DeepCopy()
	rl, err := rqutils.GetResourceListFromMigratingVM(rqCpy, vmi.Name)
	if err != nil {
		return err
	} else if rl == nil {
		logrus.Debugf("restoreResourceQuotaWithVMI: can not find migrating vm %s, skip updating namespace %s", vmi.Name, vmi.Namespace)
		return nil
	}

	needUpdate, rqToUpdate := rqutils.CalculateRestoreResourceQuotaWithVMI(rqCpy, vmi, rl)
	if !needUpdate {
		logrus.Debugf("restoreResourceQuotaWithVMI: no need to update resource quota, skip updating namespace %s and vm %s", vmi.Namespace, vmi.Name)
		return nil
	}

	// Update resource quota
	rqutils.RemoveMigratingVM(rqToUpdate, vmi.Name)
	_, err = h.rqs.Update(rqToUpdate)
	return err
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
