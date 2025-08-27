package virtualmachine

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	runtime "k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlharvestercorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
	"github.com/harvester/harvester/pkg/util/resourcequota"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	indexerwebhook "github.com/harvester/harvester/pkg/webhook/indexeres"
	"github.com/harvester/harvester/pkg/webhook/types"
	webhookutil "github.com/harvester/harvester/pkg/webhook/util"
)

func NewValidator(
	nsCache v1.NamespaceCache,
	podCache v1.PodCache,
	pvcCache v1.PersistentVolumeClaimCache,
	rqCache ctlharvestercorev1.ResourceQuotaCache,
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache,
	vmimCache ctlkubevirtv1.VirtualMachineInstanceMigrationCache,
	vmCache ctlkubevirtv1.VirtualMachineCache,
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache,
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache,
	settingCache ctlharvesterv1.SettingCache,
) types.Validator {
	return &vmValidator{
		pvcCache:      pvcCache,
		vmBackupCache: vmBackupCache,
		vmCache:       vmCache,
		vmiCache:      vmiCache,
		nadCache:      nadCache,

		rqCalculator: resourcequota.NewCalculator(nsCache, podCache, rqCache, vmimCache, settingCache),
	}
}

type vmValidator struct {
	types.DefaultValidator
	pvcCache      v1.PersistentVolumeClaimCache
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache
	vmCache       ctlkubevirtv1.VirtualMachineCache
	vmiCache      ctlkubevirtv1.VirtualMachineInstanceCache
	nadCache      ctlcniv1.NetworkAttachmentDefinitionCache
	rqCalculator  *resourcequota.Calculator
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

func (v *vmValidator) getClusterNetworkForNad(nwName string) (clusterNetwork string, err error) {
	words := strings.Split(nwName, "/")
	var namespace, name string
	switch len(words) {
	case 1:
		namespace, name = "default", words[0]
	case 2:
		namespace, name = words[0], words[1]
	default:
		return clusterNetwork, fmt.Errorf("invalid network name %s", nwName)
	}

	nad, err := v.nadCache.Get(namespace, name)
	if err != nil {
		return clusterNetwork, err
	}
	clusterNetwork, ok := nad.Labels[keyClusterNetwork]
	if !ok {
		return clusterNetwork, err
	}

	return clusterNetwork, nil
}

func (v *vmValidator) getVMInterfaceInfo(vms []*kubevirtv1.VirtualMachine) (MacAddrPerCluster map[string]map[string]string, err error) {
	MacAddrPerCluster = make(map[string]map[string]string) //map[clusterName]map[macaddr]intfName

	for _, vm := range vms {
		vmName := vm.Namespace + "/" + vm.Name

		if vm.Spec.Template == nil {
			return MacAddrPerCluster, fmt.Errorf("vm %s template is nil", vm.Name)
		}

		vmCluster := make(map[string]string) // map[netIntfName]clusterNetwork
		vmNetworks := vm.Spec.Template.Spec.Networks
		for _, vmNetwork := range vmNetworks {
			if vmNetwork.Multus == nil || vmNetwork.Multus.NetworkName == "" {
				continue
			}
			cn, err := v.getClusterNetworkForNad(vmNetwork.Multus.NetworkName)
			if err != nil {
				return MacAddrPerCluster, err
			}
			vmCluster[vmNetwork.Name] = cn
		}

		vmInterfaces := vm.Spec.Template.Spec.Domain.Devices.Interfaces

		var macAddrs map[string]string
		for _, vmInterface := range vmInterfaces {
			if vmInterface.MacAddress == "" {
				continue
			}

			clusNet := vmCluster[vmInterface.Name]
			if _, exists := MacAddrPerCluster[clusNet]; !exists {
				macAddrs = make(map[string]string)
			} else {
				macAddrs = MacAddrPerCluster[clusNet]
			}
			macAddrs[vmInterface.MacAddress] = vmInterface.Name + "+" + vmName
			MacAddrPerCluster[clusNet] = macAddrs
		}
	}

	return MacAddrPerCluster, nil
}

func (v *vmValidator) checkForDuplicateMacAddrs(vm *kubevirtv1.VirtualMachine) (err error) {
	var newVMs []*kubevirtv1.VirtualMachine
	var oldVMsInfo map[string]map[string]string
	var oldVMsList []*kubevirtv1.VirtualMachine

	//get map[clustername]map[mac]string info for new vm
	newVMs = append(newVMs, vm)
	newVMsInfo, err := v.getVMInterfaceInfo(newVMs)
	if err != nil {
		return err
	}

	//newVMsInfo will be empty when new vms are created without any mac address config
	//skip processing further as there is no mac address configuration from user
	if len(newVMsInfo) == 0 {
		return nil
	}

	if vm.Spec.Template == nil {
		return fmt.Errorf("vm %s template is nil", vm.Name)
	}

	vmInterfaces := vm.Spec.Template.Spec.Domain.Devices.Interfaces

	for _, vmInterface := range vmInterfaces {
		oldVMs, err := v.vmCache.GetByIndex(indexerwebhook.VMByMacAddress, vmInterface.MacAddress)
		if err != nil {
			return err
		}

		oldVMsList = append(oldVMsList, oldVMs...)
	}

	//No matching mac addresses,skip duplicate mac address check
	if len(oldVMsList) == 0 {
		return nil
	}

	oldVMsInfo, err = v.getVMInterfaceInfo(oldVMsList)
	if err != nil {
		return err
	}

	for cn, macs := range newVMsInfo {
		if othermacs, exists := oldVMsInfo[cn]; exists {
			if comparemacs(macs, othermacs) { //duplicate present
				return fmt.Errorf("duplicate mac address present for vm %s for cluster network %s", vm.Name, cn)
			}
		}
	}

	return nil
}

func comparemacs(macList1 map[string]string, macList2 map[string]string) bool {
	for mac1, newvmIntfName := range macList1 {
		if oldVMIntfName, exists := macList2[mac1]; exists && oldVMIntfName != newvmIntfName { //skip same vms during vm migration,restore cases
			return true
		}
	}

	return false
}

func (v *vmValidator) Create(_ *types.Request, newObj runtime.Object) error {
	vm := newObj.(*kubevirtv1.VirtualMachine)
	if vm == nil {
		return nil
	}

	if err := v.checkVMSpec(vm); err != nil {
		return err
	}

	if err := v.checkStorageResourceQuota(vm, nil); err != nil {
		return err
	}

	if err := v.checkForDuplicateMacAddrs(vm); err != nil {
		return err
	}

	return nil
}

func (v *vmValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	newVM := newObj.(*kubevirtv1.VirtualMachine)
	if newVM == nil {
		return nil
	}

	if err := v.checkVMSpec(newVM); err != nil {
		return err
	}

	if err := v.checkVolumeReq(newVM); err != nil {
		return err
	}

	oldVM := oldObj.(*kubevirtv1.VirtualMachine)
	if oldVM == nil {
		return nil
	}

	if err := v.checkStorageResourceQuota(newVM, oldVM); err != nil {
		return err
	}

	// Prevent users to stop/restart VM when there is VMBackup in progress.
	if v.checkVMStoppingStatus(oldVM, newVM) {
		if err := v.checkVMBackup(newVM); err != nil {
			return err
		}
	}

	// Check resize volumes
	if err := v.checkResizeVolumes(oldVM, newVM); err != nil {
		return err
	}

	if oldVM.Spec.Template != nil && newVM.Spec.Template != nil && reflect.DeepEqual(oldVM.Spec.Template.Spec.Domain.Devices.Interfaces, newVM.Spec.Template.Spec.Domain.Devices.Interfaces) {
		return nil
	}

	if err := v.checkForDuplicateMacAddrs(newVM); err != nil {
		return err
	}

	return nil
}

func (v *vmValidator) checkVMSpec(vm *kubevirtv1.VirtualMachine) error {
	if err := v.checkVolumeClaimTemplatesAnnotation(vm); err != nil {
		message := fmt.Sprintf("the volumeClaimTemplates annotaion is invalid: %v", err)
		return werror.NewInvalidError(message, "metadata.annotations")
	}
	if err := v.checkGoldenImage(vm); err != nil {
		return err
	}
	if err := v.checkOccupiedPVCs(vm); err != nil {
		return err
	}
	if err := v.checkTerminationGracePeriodSeconds(vm); err != nil {
		return err
	}
	if err := v.checkReservedMemoryAnnotation(vm); err != nil {
		return err
	}
	return v.rqCalculator.CheckIfVMCanStartByResourceQuota(vm)
}

func (v *vmValidator) checkVolumeReq(newVM *kubevirtv1.VirtualMachine) error {
	if newVM.Spec.Template == nil {
		return nil
	}

	// if vm is being created by vm import controller, then we need to skip pvc golden image validation
	// this case needs to be treated differently from a standard image scenario as user will boot directly from disk
	imported := util.IsImportedByVMIC(newVM)

	for _, volReq := range newVM.Status.VolumeRequests {
		if volReq.AddVolumeOptions == nil {
			continue
		}
		if volReq.AddVolumeOptions.VolumeSource.PersistentVolumeClaim == nil {
			continue
		}
		pvcName := volReq.AddVolumeOptions.VolumeSource.PersistentVolumeClaim.ClaimName
		pvcNS := newVM.Namespace
		pvc, err := v.pvcCache.Get(pvcNS, pvcName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return werror.NewInternalError(fmt.Sprintf("failed to get PVC %s/%s, err: %s", pvcNS, pvcName, err))
		}

		if imported {
			continue
		}

		if _, ok := pvc.Annotations[util.AnnotationGoldenImage]; ok {
			if pvc.Annotations[util.AnnotationGoldenImage] == "true" {
				return werror.NewInvalidError(fmt.Sprintf("PVC %s/%s is a golden image, it can't be used as a hotplug volume in VM", pvcNS, pvcName), "status.volumeRequests")
			}
		}
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
		}
	}

	return nil
}

func (v *vmValidator) checkGoldenImage(vm *kubevirtv1.VirtualMachine) error {
	if vm.Spec.Template == nil {
		return nil
	}

	// if vm is being created by vm import controller, then we need to skip pvc golden image validation
	// this case needs to be treated differently from a standard image scenario as user will boot directly from disk
	imported := util.IsImportedByVMIC(vm)

	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}
		targetPVC, err := v.pvcCache.Get(vm.Namespace, volume.PersistentVolumeClaim.ClaimName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return werror.NewInternalError(fmt.Sprintf("failed to get PVC %s/%s, err: %s", vm.Namespace, volume.PersistentVolumeClaim.ClaimName, err))
		}

		if targetPVC.Annotations == nil {
			continue
		}

		if imported {
			continue
		}

		if _, ok := targetPVC.Annotations[util.AnnotationGoldenImage]; ok {
			if targetPVC.Annotations[util.AnnotationGoldenImage] == "true" {
				return werror.NewInvalidError(fmt.Sprintf("PVC %s/%s is a golden image, it can't be used as a volume in VM", targetPVC.Namespace, targetPVC.Name), "spec.templates.spec.volumes")
			}
		}
	}
	return nil
}

func (v *vmValidator) checkOccupiedPVCs(vm *kubevirtv1.VirtualMachine) error {
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			vms, err := v.vmCache.GetByIndex(indexeresutil.VMByPVCIndex, ref.Construct(vm.Namespace, volume.PersistentVolumeClaim.ClaimName))
			if err != nil {
				return werror.NewInternalError(fmt.Sprintf("failed to get VMs by index: %s, PVC: %s/%s, err: %s", indexeresutil.VMByPVCIndex, vm.Namespace, volume.PersistentVolumeClaim.ClaimName, err))
			}
			for _, otherVM := range vms {
				if otherVM.Namespace != vm.Namespace || otherVM.Name != vm.Name {
					message := fmt.Sprintf("the volume %s is already used by VM %s/%s", volume.PersistentVolumeClaim.ClaimName, otherVM.Namespace, otherVM.Name)
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

func (v *vmValidator) checkTerminationGracePeriodSeconds(vm *kubevirtv1.VirtualMachine) error {
	terminationGracePeriodSeconds := vm.Spec.Template.Spec.TerminationGracePeriodSeconds

	//During vm's "TerminationGracePeriodSeconds" field is not specified,
	//vm mutator will set this field as "default-vm-termination-grace-period-seconds" from settings.
	//However, we still check it for robustness.
	if terminationGracePeriodSeconds == nil {
		return nil
	}

	if *terminationGracePeriodSeconds < 0 {
		return werror.NewInvalidError("Termination grace period can't be negative",
			"spec.Template.Spec.TerminationGracePeriodSeconds")
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
	if vm.Spec.Template.Spec.Domain.Memory != nil && vm.Spec.Template.Spec.Domain.Memory.Guest != nil {
		guestMemory := vm.Spec.Template.Spec.Domain.Memory.Guest
		totalMemory := *guestMemory
		totalMemory.Add(reservedMemory)
		if totalMemory.Cmp(*mem) > 0 {
			return werror.NewInvalidError(fmt.Sprintf("guest memory %v plus reservedMemory %v cannot be greater than limits.memory %v", guestMemory.Value(), reservedMemory.Value(), mem.Value()), field)
		}
	}
	if reservedMemory.CmpInt64(0) == -1 {
		return werror.NewInvalidError("reservedMemory cannot be less than 0", field)
	}
	return nil
}

func (v *vmValidator) checkStorageResourceQuota(vm *kubevirtv1.VirtualMachine, oldVM *kubevirtv1.VirtualMachine) error {
	return v.rqCalculator.CheckStorageResourceQuota(vm, oldVM)
}
