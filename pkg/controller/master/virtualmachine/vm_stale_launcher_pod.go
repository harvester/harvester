package virtualmachine

import (
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

const virtLauncherAppLabelValue = "virt-launcher"

// CleanupStaleLauncherPodsOnVMChange removes terminated virt-launcher pods of the
// running VMI that still reference a PVC the VM no longer uses.
//
// After a storage migration (UpdateVolumesStrategy=Migration), KubeVirt leaves the
// migration source virt-launcher pod in the Succeeded phase until the VMI is
// deleted. That pod still lists the migrated-away source PVC in spec.volumes, so
// the kubernetes.io/pvc-protection finalizer keeps the source PVC in Terminating
// until the VM is stopped. Deleting the finished pod releases the source PVC
// without touching the running VM.
func (h *VMController) CleanupStaleLauncherPodsOnVMChange(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil {
		return vm, nil
	}
	return vm, h.cleanupStaleLauncherPods(vm)
}

// CleanupStaleLauncherPodsOnPodChange triggers the same cleanup when a
// virt-launcher pod reaches a terminal phase. The migration source pod usually
// finishes after the VM object has settled, so a VM-only trigger can miss it.
func (h *VMController) CleanupStaleLauncherPodsOnPodChange(_ string, pod *corev1.Pod) (*corev1.Pod, error) {
	if pod == nil || pod.DeletionTimestamp != nil || !isVirtLauncherPod(pod) || !isPodTerminated(pod) {
		return pod, nil
	}

	vmiName := vmiOwnerName(pod)
	if vmiName == "" {
		return pod, nil
	}

	// A VMI owned by a VM shares the VM's name.
	vm, err := h.vmCache.Get(pod.Namespace, vmiName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return pod, nil
		}
		return pod, err
	}
	return pod, h.cleanupStaleLauncherPods(vm)
}

func (h *VMController) cleanupStaleLauncherPods(vm *kubevirtv1.VirtualMachine) error {
	if vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return nil
	}

	// Leave everything alone while a storage migration is pending or in progress.
	if _, ok := vm.Annotations[util.AnnotationWaitingStorageMigration]; ok {
		return nil
	}
	if hasVolumesChangeCondition(vm) {
		return nil
	}

	vmi, err := h.vmiCache.Get(vm.Namespace, vm.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if vmi.DeletionTimestamp != nil || vmi.IsFinal() {
		return nil
	}

	pods, err := h.podCache.List(vm.Namespace, labels.SelectorFromSet(labels.Set{
		kubevirtv1.AppLabel:       virtLauncherAppLabelValue,
		kubevirtv1.CreatedByLabel: string(vmi.UID),
	}))
	if err != nil {
		return fmt.Errorf("failed to list virt-launcher pods of VMI %s/%s: %w", vmi.Namespace, vmi.Name, err)
	}

	inUse := vmClaimNames(vm)
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil || !isPodTerminated(pod) {
			continue
		}

		stale := h.stalePVCs(pod, vm.Name, inUse)
		if len(stale) == 0 {
			continue
		}

		logrus.WithFields(logrus.Fields{
			"namespace": pod.Namespace,
			"pod":       pod.Name,
			"vm":        vm.Name,
			"phase":     pod.Status.Phase,
			"pvcs":      stale,
		}).Info("deleting terminated virt-launcher pod that still references PVCs no longer used by the VM")

		if err := h.podClient.Delete(pod.Namespace, pod.Name, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete stale virt-launcher pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}
	return nil
}

func isVirtLauncherPod(pod *corev1.Pod) bool {
	return pod.Labels[kubevirtv1.AppLabel] == virtLauncherAppLabelValue
}

func isPodTerminated(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

func vmiOwnerName(pod *corev1.Pod) string {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == kubevirtv1.VirtualMachineInstanceGroupVersionKind.Kind {
			return owner.Name
		}
	}
	return ""
}

// vmClaimNames returns every claim name the VM spec references directly or
// through a DataVolume, ephemeral or memory-dump volume.
func vmClaimNames(vm *kubevirtv1.VirtualMachine) map[string]bool {
	names := map[string]bool{}
	for _, vol := range vm.Spec.Template.Spec.Volumes {
		switch {
		case vol.PersistentVolumeClaim != nil:
			names[vol.PersistentVolumeClaim.ClaimName] = true
		case vol.DataVolume != nil:
			names[vol.DataVolume.Name] = true
		case vol.Ephemeral != nil && vol.Ephemeral.PersistentVolumeClaim != nil:
			names[vol.Ephemeral.PersistentVolumeClaim.ClaimName] = true
		case vol.MemoryDump != nil:
			names[vol.MemoryDump.ClaimName] = true
		}
	}
	return names
}

// stalePVCs returns the PVCs mounted by the pod that the VM no longer uses.
// The VM's KubeVirt backend-storage PVC (persistent TPM/EFI state) is mounted by
// the launcher pod without appearing in the VM spec, so it never counts as stale.
func (h *VMController) stalePVCs(pod *corev1.Pod, vmName string, inUse map[string]bool) []string {
	var stale []string
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim == nil || vol.PersistentVolumeClaim.ClaimName == "" {
			continue
		}
		name := vol.PersistentVolumeClaim.ClaimName
		if inUse[name] || h.isBackendStoragePVC(pod.Namespace, name, vmName) {
			continue
		}
		stale = append(stale, name)
	}
	return stale
}

// isBackendStoragePVC reports whether the claim is the backend-storage PVC of
// the named VM. Like KubeVirt's backend-storage lookup, it matches the
// persistent-state-for label value against the VM name, so a claim labeled for
// another VM is not exempt.
func (h *VMController) isBackendStoragePVC(namespace, name, vmName string) bool {
	pvc, err := h.pvcCache.Get(namespace, name)
	if err != nil {
		// A PVC that is gone or unreadable can't be a live backend-storage volume.
		return false
	}
	owner, ok := pvc.Labels[util.LabelKubeVirtPersistentState]
	return ok && owner == vmName
}
