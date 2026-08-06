package virtualmachine

import (
	"fmt"
	"slices"
	"strings"

	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
)

var (
	ErrVolumeIsUsedByOtherVM = fmt.Errorf("the volume is non-shareable and already used by other VMs")
)

// IsVMStopped checks VM is stopped or not. It will check two cases
// 1. VM is stopped from GUI
// 2. VM is stopped inside VM
// These two cases are all stopped case.
func IsVMStopped(
	vm *kubevirtv1.VirtualMachine,
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache,
) (bool, error) {
	strategy, err := vm.RunStrategy()
	if err != nil {
		return false, fmt.Errorf("error getting run strategy for vm %s in namespace %s: %v", vm.Name, vm.Namespace, err)
	}

	if vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusStopped {
		return false, nil
	}

	// VM is stopped from GUI
	if strategy == kubevirtv1.RunStrategyHalted {
		return true, nil
	}

	// When vm is stopped inside VM, the vmi will not be deleted.
	// The status of vmi will be "Succeeded".
	vmi, err := vmiCache.Get(vm.Namespace, vm.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Debugf("VM %s/%s is deleted", vm.Namespace, vm.Name)
			return true, nil
		}
		return false, fmt.Errorf("error getting vmi %s in namespace %s: %v", vm.Name, vm.Namespace, err)
	}

	if vmi.IsFinal() {
		logrus.Debugf("VM %s/%s is stopped inside VM", vm.Namespace, vm.Name)
		return true, nil
	}

	return false, nil
}

func SupportCPUAndMemoryHotplug(vm *kubevirtv1.VirtualMachine) bool {
	if vm == nil || vm.Annotations == nil {
		return false
	}

	return strings.ToLower(vm.Annotations[util.AnnotationEnableCPUAndMemoryHotplug]) == "true"
}

func supportNicHotActionCommon(vm *kubevirtv1.VirtualMachine) (bool, error) {
	if vm == nil {
		return false, fmt.Errorf("vm shouldn't be nil")
	}

	// for stability, guest cluster VMs aren't allowed until integration with Rancher Manger is done
	if vm.Labels != nil && vm.Labels[util.LabelVMCreator] == util.VirtualMachineCreatorNodeDriver {
		return false, fmt.Errorf("%s/%s doesn't support both HotPlugNic and HotUnplugNic as it is a guest cluster node", vm.Namespace, vm.Name)
	}

	// to prevent unexpected RestartRequired condition due to missing macAddress in VM spec,
	// caused by our existing implmentation for preserving MAC addresses, VMs without macAddress defined in VM spec are not allowed
	// until backfilling MAC addresses to VM spec while stopping VM by the new reconciliation logic
	for _, iface := range vm.Spec.Template.Spec.Domain.Devices.Interfaces {
		if iface.MacAddress == "" {
			return false, fmt.Errorf("%s/%s doesn't support both HotPlugNic and HotUnplugNic as macAddress is missing for some interfaces in the VM spec", vm.Namespace, vm.Name)
		}
	}

	return true, nil
}

func SupportHotplugNic(vm *kubevirtv1.VirtualMachine) (bool, error) {
	return supportNicHotActionCommon(vm)
}

func IsInterfaceHotUnpluggable(iface kubevirtv1.Interface) (bool, error) {
	if iface.State == kubevirtv1.InterfaceStateAbsent {
		return false, fmt.Errorf("%s was already registered for hot-unplugging", iface.Name)
	}

	if iface.Bridge == nil {
		return false, fmt.Errorf("%s is not in bridge mode", iface.Name)
	}

	if iface.Model != "" && iface.Model != kubevirtv1.VirtIO {
		return false, fmt.Errorf("%s is not using virtio model", iface.Name)
	}

	return true, nil
}

func SupportHotUnplugNic(vm *kubevirtv1.VirtualMachine) (bool, error) {
	if _, err := supportNicHotActionCommon(vm); err != nil {
		return false, err
	}

	ifaces := vm.Spec.Template.Spec.Domain.Devices.Interfaces
	if len(ifaces) <= 1 {
		return false, fmt.Errorf("%s/%s doesn't support HotUnplugNic as it has only one nic", vm.Namespace, vm.Name)
	}

	errMsgs := make([]string, 0)
	for _, iface := range ifaces {
		ok, err := IsInterfaceHotUnpluggable(iface)
		if ok {
			// as long as there is at least one hot-unpluggable interface
			return true, nil
		}
		if err != nil {
			errMsgs = append(errMsgs, err.Error())
		}
	}

	return false, fmt.Errorf("%s/%s doesn't support HotUnplugNic as none of its interfaces is hot-unpluggable: %s", vm.Namespace, vm.Name, strings.Join(errMsgs, ", "))
}

// CheckBlockRWXVolumeForVM checks whether the given PVC is a RWX volume (exclude Longhorn) and whether it is already used by other VMs.
func CheckBlockRWXVolumeForVM(pvcCache v1.PersistentVolumeClaimCache, scCache ctlstoragev1.StorageClassCache, vmCache ctlkubevirtv1.VirtualMachineCache, pvcNS, pvcName, targetVMNS, targetVMName string) error {
	pvc, err := pvcCache.Get(pvcNS, pvcName)
	if apierrors.IsNotFound(err) {
		// means runtime creation, no need to check
		return nil
	}
	if err != nil {
		// any error here should be raised
		return fmt.Errorf("failed to get PVC %s/%s, err: %s", pvcNS, pvcName, err)
	}
	targetAccessMode := pvc.Spec.AccessModes
	targetProvisioner := util.GetProvisionedPVCProvisioner(pvc, scCache)
	if volumeSupportRWXForVM(targetAccessMode, targetProvisioner) {
		return nil
	}
	vms, err := vmCache.GetByIndex(indexeresutil.VMByNonShareablePVCIndex, ref.Construct(pvcNS, pvcName))
	if err != nil {
		return fmt.Errorf("failed to get VMs by index: %s, PVC: %s/%s, err: %s", indexeresutil.VMByNonShareablePVCIndex, pvcNS, pvcName, err)
	}
	for _, otherVM := range vms {
		if otherVM.Namespace != targetVMNS || otherVM.Name != targetVMName {
			return ErrVolumeIsUsedByOtherVM
		}
	}
	return nil
}

func volumeSupportRWXForVM(accessModes []corev1.PersistentVolumeAccessMode, provisioner string) bool {
	if provisioner == util.CSIProvisionerLonghorn {
		// Longhorn provisioner does not support RWX volume for VM
		return false
	}

	return slices.Contains(accessModes, corev1.ReadWriteMany)
}

// CheckShareableVolume checks whether the given PVC can back a shareable disk.
// Concurrent writers on different nodes require a block-mode RWX volume, and
// Longhorn does not support that for VMs, so a provisioner other than Longhorn
// is required. KubeVirt itself performs no admission validation on shareable
// disks, so this is the only gate.
func CheckShareableVolume(pvcCache v1.PersistentVolumeClaimCache, scCache ctlstoragev1.StorageClassCache, pvcNS, pvcName string) error {
	pvc, err := pvcCache.Get(pvcNS, pvcName)
	if apierrors.IsNotFound(err) {
		// Unlike other checks we do not skip runtime-created PVCs
		// (volumeClaimTemplates): a shareable disk shares an existing volume,
		// and skipping would admit a spec whose PVC turns out to be
		// non-shareable once created.
		return fmt.Errorf("PVC %s/%s does not exist: a shareable disk requires an existing volume", pvcNS, pvcName)
	}
	if err != nil {
		return fmt.Errorf("failed to get PVC %s/%s, err: %s", pvcNS, pvcName, err)
	}

	provisioner := util.GetProvisionedPVCProvisioner(pvc, scCache)
	if !volumeSupportRWXForVM(pvc.Spec.AccessModes, provisioner) {
		return fmt.Errorf("PVC %s/%s cannot back a shareable disk: a ReadWriteMany volume from a provisioner other than Longhorn is required", pvcNS, pvcName)
	}
	if pvc.Spec.VolumeMode == nil || *pvc.Spec.VolumeMode != corev1.PersistentVolumeBlock {
		return fmt.Errorf("PVC %s/%s cannot back a shareable disk: volumeMode must be Block", pvcNS, pvcName)
	}
	return nil
}

// HasShareableDisk returns the name of the first disk with shareable enabled.
func HasShareableDisk(vm *kubevirtv1.VirtualMachine) (string, bool) {
	if vm.Spec.Template == nil {
		return "", false
	}
	for _, disk := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		if disk.Shareable != nil && *disk.Shareable {
			return disk.Name, true
		}
	}
	return "", false
}

// VMUsesPVCAsShareable returns true if the VM attaches the given PVC through a
// disk with shareable enabled.
func VMUsesPVCAsShareable(vm *kubevirtv1.VirtualMachine, pvcName string) bool {
	if vm.Spec.Template == nil {
		return false
	}
	shareableDisks := map[string]struct{}{}
	for _, disk := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		if disk.Shareable != nil && *disk.Shareable {
			shareableDisks[disk.Name] = struct{}{}
		}
	}
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName != pvcName {
			continue
		}
		if _, ok := shareableDisks[volume.Name]; ok {
			return true
		}
	}
	return false
}

func isDiskSataCdRom(disk *kubevirtv1.Disk) bool {
	return disk.CDRom != nil && disk.CDRom.Bus == kubevirtv1.DiskBusSATA
}

func HasDiskSataCdRomWithName(vm *kubevirtv1.VirtualMachine, deviceName string) bool {
	for _, disk := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		if disk.Name == deviceName && isDiskSataCdRom(&disk) {
			return true
		}
	}
	return false
}

func SupportInsertCdRomVolume(vm *kubevirtv1.VirtualMachine) (bool, error) {
	volumeMaps := map[string]struct{}{}
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		volumeMaps[volume.Name] = struct{}{}
	}

	hasEmptyCdRom := false
	for _, disk := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		if disk.CDRom == nil {
			continue
		}
		_, hasVolume := volumeMaps[disk.Name]
		if hasVolume {
			continue
		}
		useSata := isDiskSataCdRom(&disk)
		if !useSata {
			return false, fmt.Errorf("empty cd-rom device should connect via SATA bus")
		}
		hasEmptyCdRom = true
	}
	return hasEmptyCdRom, nil
}

func SupportEjectCdRomVolume(vm *kubevirtv1.VirtualMachine) (bool, error) {
	volumeMaps := map[string]struct{}{}
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.Hotpluggable {
			volumeMaps[volume.Name] = struct{}{}
		}
	}

	if len(volumeMaps) == 0 {
		return false, nil
	}

	hasEjectableCdRom := false
	for _, disk := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		if disk.CDRom == nil {
			continue
		}
		_, hotpluggable := volumeMaps[disk.Name]
		if !hotpluggable {
			continue
		}
		useSata := isDiskSataCdRom(&disk)
		if !useSata {
			return false, fmt.Errorf("hotpluggable cd-rom volume should connect via SATA bus")
		}
		hasEjectableCdRom = true
	}
	return hasEjectableCdRom, nil
}
