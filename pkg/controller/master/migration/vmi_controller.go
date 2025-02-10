package migration

import (
	"fmt"
	"time"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlharvcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlvirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

// Handler resets vmi annotations and nodeSelector when a migration completes
type Handler struct {
	namespace  string
	rqs        ctlharvcorev1.ResourceQuotaClient
	rqCache    ctlharvcorev1.ResourceQuotaCache
	vmis       ctlvirtv1.VirtualMachineInstanceClient
	vmiCache   ctlvirtv1.VirtualMachineInstanceCache
	vms        ctlvirtv1.VirtualMachineClient
	vmCache    ctlvirtv1.VirtualMachineCache
	podCache   ctlcorev1.PodCache
	pods       ctlcorev1.PodClient
	restClient rest.Interface
}

func (h *Handler) OnVmiChanged(_ string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	if vmi == nil || vmi.DeletionTimestamp != nil ||
		vmi.Annotations == nil || vmi.Status.MigrationState == nil {
		return vmi, nil
	}

	if vmi.Annotations[util.AnnotationMigrationUID] == string(vmi.Status.MigrationState.MigrationUID) &&
		vmi.Status.MigrationState.Completed {
		if err := h.resetHarvesterMigrationStateInVmiAndSyncVM(vmi); err != nil {
			logrus.Infof("vmi %s/%s finished migration but fail to reset state %s", vmi.Namespace, vmi.Name, err.Error())
			return vmi, err
		}
	}

	if vmi.Status.MigrationState.Completed && vmi.Status.MigrationState.AbortStatus == kubevirtv1.MigrationAbortSucceeded {
		// clean up leftover pod on abortion success
		// https://github.com/kubevirt/kubevirt/issues/5373
		sets := labels.Set{
			kubevirtv1.MigrationJobLabel: string(vmi.Status.MigrationState.MigrationUID),
		}
		pods, err := h.podCache.List(vmi.Namespace, sets.AsSelector())
		if err != nil {
			return vmi, err
		}
		if len(pods) > 0 {
			if err := h.pods.Delete(vmi.Namespace, pods[0].Name, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return vmi, err
			}
		}
	}

	return vmi, nil
}

func (h *Handler) setVmiMigrationUIDAnnotationAndSyncVM(vmi *kubevirtv1.VirtualMachineInstance, UID string, state string) error {
	ts := vmi.Annotations[util.AnnotationMigrationStateTimestamp]
	checkTs := true
	// a new UID or new state
	if vmi.Annotations[util.AnnotationMigrationUID] != UID ||
		vmi.Annotations[util.AnnotationMigrationState] != state {
		toUpdate := vmi.DeepCopy()
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = make(map[string]string)
		}
		if UID != "" {
			toUpdate.Annotations[util.AnnotationMigrationUID] = UID
			toUpdate.Annotations[util.AnnotationMigrationState] = state
			// update the new ts to vmi
			toUpdate.Annotations[util.AnnotationMigrationStateTimestamp] = time.Now().Format(time.RFC3339)
			checkTs = false
		} else {
			delete(toUpdate.Annotations, util.AnnotationMigrationUID)
			delete(toUpdate.Annotations, util.AnnotationMigrationState)
			delete(toUpdate.Annotations, util.AnnotationMigrationStateTimestamp)
		}
		if _, err := h.vmis.Update(toUpdate); err != nil {
			return err
		}
	}
	// state/UID does not change, check if VM needs to be updated
	return h.syncVM(vmi, checkTs, ts)
}

// syncVM update vm so that UI gets websocket message with updated actions
// checkTs: true: check the vm.annotation AnnotationTimestamp with the input ts, false: do not
func (h *Handler) syncVM(vmi *kubevirtv1.VirtualMachineInstance, checkTs bool, ts string) error {
	vm, err := h.vmCache.Get(vmi.Namespace, vmi.Name)
	if err != nil {
		return err
	}

	curTs := vm.Annotations[util.AnnotationTimestamp]
	// update vm object timestamp anyway
	// or
	// check ts to decide if update is required
	// mainly for the case: setVmiMigrationUIDAnnotationAndSyncVM updated vmi but failed to update vm
	// then the timestamp on vmi is newer than on vm, following code will be executed
	if !checkTs || curTs < ts {
		toUpdateVM := vm.DeepCopy()
		if toUpdateVM.Annotations == nil {
			toUpdateVM.Annotations = make(map[string]string)
		}
		// get the current timestamp directly
		toUpdateVM.Annotations[util.AnnotationTimestamp] = time.Now().Format(time.RFC3339)
		_, err = h.vms.Update(toUpdateVM)
		return err
	}

	return nil
}

func (h *Handler) resetHarvesterMigrationStateInVMI(vmi *kubevirtv1.VirtualMachineInstance) error {
	toUpdate := vmi.DeepCopy()
	delete(toUpdate.Annotations, util.AnnotationMigrationUID)
	delete(toUpdate.Annotations, util.AnnotationMigrationState)
	if vmi.Annotations[util.AnnotationMigrationTarget] != "" {
		delete(toUpdate.Annotations, util.AnnotationMigrationTarget)
		delete(toUpdate.Spec.NodeSelector, corev1.LabelHostname)
	}

	if _, err := h.vmis.Update(toUpdate); err != nil {
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
		return fmt.Errorf("fail to reset vmi migration %w", err)
	}
	// without syncing VM, the UI may still show the actions deduced from old VM/VMI status combination
	if err := h.syncVM(vmi, false, ""); err != nil {
		return fmt.Errorf("fail to reset vmi migration to vm %w", err)
	}
	return nil
}
