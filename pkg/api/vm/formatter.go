package vm

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/v3/pkg/data/convert"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"

	apiutil "github.com/harvester/harvester/pkg/api/util"
	"github.com/harvester/harvester/pkg/controller/master/migration"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/virtualmachine"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
)

const (
	startVM                          = "start"
	stopVM                           = "stop"
	restartVM                        = "restart"
	softReboot                       = "softreboot"
	pauseVM                          = "pause"
	unpauseVM                        = "unpause"
	ejectCdRom                       = "ejectCdRom"
	migrate                          = "migrate"
	abortMigration                   = "abortMigration"
	findMigratableNodes              = "findMigratableNodes"
	backupVM                         = "backup"
	snapshotVM                       = "snapshot"
	restoreVM                        = "restore"
	createTemplate                   = "createTemplate"
	addVolume                        = "addVolume"
	removeVolume                     = "removeVolume"
	cloneVM                          = "clone"
	forceStopVM                      = "forceStop"
	dismissInsufficientResourceQuota = "dismissInsufficientResourceQuota"
	updateResourceQuotaAction        = "updateResourceQuota"
	deleteResourceQuotaAction        = "deleteResourceQuota"
	cpuAndMemoryHotplug              = "cpuAndMemoryHotplug"
)

type vmformatter struct {
	vmiCache      ctlkubevirtv1.VirtualMachineInstanceCache
	pvcCache      ctlcorev1.PersistentVolumeClaimCache
	nodeCache     ctlcorev1.NodeCache
	scCache       ctlstoragev1.StorageClassCache
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache
	clientSet     kubernetes.Clientset
}

func (vf *vmformatter) formatter(request *types.APIRequest, resource *types.RawResource) {
	// reset resource actions, because action map already be set when add actions handler,
	// but current framework can't support use formatter to remove key from action map
	resource.Actions = make(map[string]string, 1)
	if request.AccessControl.CanUpdate(request, resource.APIObject, resource.Schema) != nil {
		return
	}

	if ok, err := apiutil.CanUpdateResourceQuota(vf.clientSet, request.Namespace, request.GetUser()); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace": request.Namespace,
			"user":      request.GetUser(),
		}).Error("Failed to check update resource quota")
	} else if ok {
		resource.AddAction(request, updateResourceQuotaAction)
		resource.AddAction(request, deleteResourceQuotaAction)
	}

	vm := &kubevirtv1.VirtualMachine{}
	err := convert.ToObj(resource.APIObject.Data(), vm)
	if err != nil {
		return
	}

	resource.AddAction(request, addVolume)
	resource.AddAction(request, removeVolume)
	resource.AddAction(request, cloneVM)

	if canEjectCdRom(vm) {
		resource.AddAction(request, ejectCdRom)
	}

	vmi := vf.getVMI(vm)
	if vf.canStart(vm, vmi) {
		resource.AddAction(request, startVM)
	}

	if vf.canStop(vm, vmi) {
		resource.AddAction(request, stopVM)
	}

	if vf.canRestart(vm, vmi) {
		resource.AddAction(request, restartVM)
	}

	if vf.canSoftReboot(vmi) {
		resource.AddAction(request, softReboot)
	}

	if vf.canPause(vmi) {
		resource.AddAction(request, pauseVM)
	}

	if vf.canUnPause(vmi) {
		resource.AddAction(request, unpauseVM)
	}

	if canMigrate(vf.nodeCache, vmi) {
		resource.AddAction(request, migrate)
		resource.AddAction(request, findMigratableNodes)

		if canCPUAndMemoryHotplug(vm) {
			resource.AddAction(request, cpuAndMemoryHotplug)
		}
	}

	if canAbortMigrate(vmi) {
		resource.AddAction(request, abortMigration)
	}

	if vf.canDoBackup(vm, vmi) {
		resource.AddAction(request, backupVM)
	}

	if vf.canDoSnapshot(vm, vmi) {
		resource.AddAction(request, snapshotVM)
	}

	if vf.canDoRestore(vm, vmi) {
		resource.AddAction(request, restoreVM)
	}

	if vf.canCreateTemplate(vmi) {
		resource.AddAction(request, createTemplate)
	}

	if vf.canForceStop(vm) {
		resource.AddAction(request, forceStopVM)
	}

	if canDismissInsufficientResourceQuota(vm) {
		resource.AddAction(request, dismissInsufficientResourceQuota)
	}
}

func canEjectCdRom(vm *kubevirtv1.VirtualMachine) bool {
	if !vmReady.IsTrue(vm) {
		return false
	}

	for _, disk := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		if disk.CDRom != nil {
			return true
		}
	}
	return false
}

func (vf *vmformatter) canPause(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi == nil {
		return false
	}

	if vmi.Status.Phase != kubevirtv1.Running {
		return false
	}

	if vmi.Spec.LivenessProbe != nil {
		return false
	}

	return !vmiPaused.IsTrue(vmi)
}

func (vf *vmformatter) canUnPause(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi == nil {
		return false
	}

	if vmi.Status.Phase != kubevirtv1.Running {
		return false
	}

	return vmiPaused.IsTrue(vmi)
}

func (vf *vmformatter) canSoftReboot(vmi *kubevirtv1.VirtualMachineInstance) bool {
	return vf.canPause(vmi)
}

func (vf *vmformatter) canStart(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vf.isVMStarting(vm) {
		return false
	}

	if vmi != nil && !vmi.IsFinal() && vmi.Status.Phase != kubevirtv1.Unknown && vmi.Status.Phase != kubevirtv1.VmPhaseUnset {
		return false
	}
	return true
}

func (vf *vmformatter) canRestart(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vf.isVMStarting(vm) {
		return false
	}

	if runStrategy, err := vm.RunStrategy(); err != nil || runStrategy == kubevirtv1.RunStrategyHalted {
		return false
	}

	return vmi != nil
}

func (vf *vmformatter) canStop(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) bool {
	runStrategy, err := vm.RunStrategy()
	if err == nil {
		switch runStrategy {
		case kubevirtv1.RunStrategyHalted:
			return false
		case kubevirtv1.RunStrategyManual, kubevirtv1.RunStrategyRerunOnFailure:
			if vmi == nil {
				return false
			}

			switch vmi.Status.Phase {
			case kubevirtv1.Pending, kubevirtv1.Scheduling, kubevirtv1.Scheduled, kubevirtv1.Running:
				return true
			default:
				return false
			}
		case kubevirtv1.RunStrategyAlways:
			return true
		default:
			// skip to other condition
		}
	}

	return true
}

func isReady(vmi *kubevirtv1.VirtualMachineInstance) bool {
	for _, cond := range vmi.Status.Conditions {
		if cond.Type == kubevirtv1.VirtualMachineInstanceReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func canMigrate(nodeCache ctlcorev1.NodeCache, vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi == nil || vmi.DeletionTimestamp != nil || vmi.Annotations[util.AnnotationMigrationState] != "" {
		return false
	}

	nodes, err := nodeCache.List(labels.Everything())
	if err != nil {
		logrus.WithError(err).Error("Failed to list nodes for migration check")
		return false
	}

	if len(nodes) < 2 {
		return false
	}

	if err := virtualmachineinstance.ValidateVMMigratable(vmi); err != nil {
		return false
	}

	return true
}

func canAbortMigrate(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi == nil {
		return false
	}
	return vmi.Annotations[util.AnnotationMigrationState] == migration.StateMigrating || vmi.Annotations[util.AnnotationMigrationState] == migration.StatePending
}

func (vf *vmformatter) canDoBackup(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vm.Status.SnapshotInProgress != nil {
		return false
	}

	if vm.DeletionTimestamp != nil || (vmi != nil && vmi.DeletionTimestamp != nil) {
		return false
	}

	if vmi != nil && vmi.Status.Phase != kubevirtv1.Running && vmi.Status.Phase != kubevirtv1.Succeeded {
		return false
	}

	// additional check, we did not support backup with CDI volume
	volumes := vm.Spec.Template.Spec.Volumes
	for _, vol := range volumes {
		if vol.PersistentVolumeClaim == nil {
			continue
		}
		pvc, err := vf.pvcCache.Get(vm.Namespace, vol.PersistentVolumeClaim.ClaimName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			logrus.Errorf("Can't get PVC %s/%s, err: %+v", vm.Namespace, vol.PersistentVolumeClaim.ClaimName, err)
			return false
		}
		if _, find := pvc.Annotations[cdicommon.AnnCreatedForDataVolume]; find {
			return false
		}
	}
	return true
}

func (vf *vmformatter) canDoSnapshot(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vm.Status.SnapshotInProgress != nil {
		return false
	}

	if vm.DeletionTimestamp != nil || (vmi != nil && vmi.DeletionTimestamp != nil) {
		return false
	}

	if vmi != nil && vmi.Status.Phase != kubevirtv1.Running && vmi.Status.Phase != kubevirtv1.Succeeded {
		return false
	}

	volumes := vm.Spec.Template.Spec.Volumes
	for _, vol := range volumes {
		if vol.PersistentVolumeClaim == nil {
			continue
		}
		pvc, err := vf.pvcCache.Get(vm.Namespace, vol.PersistentVolumeClaim.ClaimName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			logrus.Errorf("Can't get PVC %s/%s, err: %+v", vm.Namespace, vol.PersistentVolumeClaim.ClaimName, err)
			return false
		}
		provisioner := util.GetProvisionedPVCProvisioner(pvc, vf.scCache)
		if find := util.GetCSIProvisionerSnapshotCapability(provisioner); !find {
			return false
		}
	}
	return true
}

func (vf *vmformatter) canDoRestore(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vm.Status.Ready || vm.Status.SnapshotInProgress != nil || vmi != nil {
		return false
	}
	vmBackups, err := vf.vmBackupCache.GetByIndex(indexeres.VMBackupBySourceVMUIDIndex, string(vm.UID))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		logrus.Errorf("Can't list VM Backups by index %s, err: %+v", indexeres.VMBackupBySourceVMUIDIndex, err)
		return false
	}
	return len(vmBackups) != 0
}

func (vf *vmformatter) isVMStarting(vm *kubevirtv1.VirtualMachine) bool {
	for _, req := range vm.Status.StateChangeRequests {
		if req.Action == kubevirtv1.StartRequest {
			return true
		}
	}
	return false
}

func (vf *vmformatter) canCreateTemplate(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi != nil && vmi.DeletionTimestamp != nil {
		return false
	}

	if vmi != nil && vmi.Status.Phase != kubevirtv1.Running && vmi.Status.Phase != kubevirtv1.Succeeded {
		return false
	}

	return true
}

func (vf *vmformatter) getVMI(vm *kubevirtv1.VirtualMachine) *kubevirtv1.VirtualMachineInstance {
	if vmi, err := vf.vmiCache.Get(vm.Namespace, vm.Name); err == nil {
		return vmi
	}
	return nil
}

func (vf *vmformatter) canForceStop(vm *kubevirtv1.VirtualMachine) bool {
	if vm == nil {
		return false
	}

	if vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusStopping {
		return false
	}
	return true
}

func canDismissInsufficientResourceQuota(vm *kubevirtv1.VirtualMachine) bool {
	if vm.Annotations == nil {
		return false
	}

	if _, ok := vm.Annotations[util.AnnotationInsufficientResourceQuota]; !ok {
		return false
	}
	return true
}

func canCPUAndMemoryHotplug(vm *kubevirtv1.VirtualMachine) bool {
	if vm.Spec.Template.Spec.Domain.CPU != nil && (vm.Spec.Template.Spec.Domain.CPU.Cores != 1 || vm.Spec.Template.Spec.Domain.CPU.Threads != 1) {
		return false
	}
	if vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusRunning {
		return false
	}
	if !virtualmachine.SupportCPUAndMemoryHotplug(vm) {
		return false
	}

	hasRestartRequiredOrHotplugMigration := false
	for _, condition := range vm.Status.Conditions {
		if condition.Type == kubevirtv1.VirtualMachineRestartRequired && condition.Status == corev1.ConditionTrue {
			hasRestartRequiredOrHotplugMigration = true
			break
		}
		if string(condition.Type) == string(kubevirtv1.VirtualMachineInstanceVCPUChange) && condition.Status == corev1.ConditionTrue {
			hasRestartRequiredOrHotplugMigration = true
			break
		}
		if string(condition.Type) == string(kubevirtv1.VirtualMachineInstanceMemoryChange) && condition.Status == corev1.ConditionTrue {
			hasRestartRequiredOrHotplugMigration = true
			break
		}
	}
	return !hasRestartRequiredOrHotplugMigration
}
