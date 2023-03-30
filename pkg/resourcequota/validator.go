package resourcequota

import (
	"encoding/json"
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	quotautils "k8s.io/apiserver/pkg/quota/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

const (
	errInvalidQuotaFmt = "invalid maintenance quota %s %d%%, the value must be in the range of 0-100"
)

var (
	ErrVMAvailableResourceNotEnoughCPU = werror.NewInvalidError("limit.CPU resource is not enough",
		"spec.template.spec.domain.resources.limits.cpu")
	ErrVMAvailableResourceNotEnoughMemory = werror.NewInvalidError("limit.memory resource is not enough",
		"spec.template.spec.domain.resources.limits.memory")
)

type AvailableResourceQuota struct {
	vmCache       ctlkubevirtv1.VirtualMachineCache
	vmimCache     ctlkubevirtv1.VirtualMachineInstanceMigrationCache
	vmbackupCache ctlharvesterv1.VirtualMachineBackupCache
	vmrCache      ctlharvesterv1.VirtualMachineRestoreCache
	nsCache       ctlcorev1.NamespaceCache
}

func NewAvailableResourceQuota(vmCache ctlkubevirtv1.VirtualMachineCache,
	vmimCache ctlkubevirtv1.VirtualMachineInstanceMigrationCache,
	vmbackupCache ctlharvesterv1.VirtualMachineBackupCache,
	vmrCache ctlharvesterv1.VirtualMachineRestoreCache,
	nsCache ctlcorev1.NamespaceCache,
) *AvailableResourceQuota {
	return &AvailableResourceQuota{
		vmCache:       vmCache,
		vmimCache:     vmimCache,
		vmbackupCache: vmbackupCache,
		vmrCache:      vmrCache,
		nsCache:       nsCache,
	}
}

func (q *AvailableResourceQuota) ValidateMaintenanceResourcesField(ns *corev1.Namespace) error {
	var maintQuota MaintenanceQuotaPercentage
	if err := json.Unmarshal([]byte(ns.Annotations[util.AnnotationMaintenanceQuota]), &maintQuota); err != nil {
		return err
	}

	if !(isPercentage(maintQuota.Limit.LimitsCPUPercent)) {
		return fmt.Errorf(errInvalidQuotaFmt, "LimitsCPUPercent", maintQuota.Limit.LimitsCPUPercent)
	}
	if !(isPercentage(maintQuota.Limit.LimitsMemoryPercent)) {
		return fmt.Errorf(errInvalidQuotaFmt, "LimitsMemoryPercent", maintQuota.Limit.LimitsMemoryPercent)
	}

	return nil
}

func (q *AvailableResourceQuota) ValidateAvailableResources(ns *corev1.Namespace) error {
	if ns == nil || ns.DeletionTimestamp != nil || ns.Annotations == nil {
		return nil
	}

	calc, err := newCalculatorByAnnotations(ns.Annotations)
	if err != nil || calc == nil {
		return err
	}
	//vmAvail, maintAvail, err := calcAvailableResourcesByNamespace(ns)
	vmAvail, maintenanceAvail, err := calc.calculateResources()
	if err != nil || (vmAvail == nil && maintenanceAvail == nil && err == nil) {
		return err
	}

	if err := q.checkVMAvailableResource(ns, vmAvail, nil); err != nil {
		return err
	}

	return q.checkMaintenanceAvailableResource(ns, maintenanceAvail, nil)
}

func (q *AvailableResourceQuota) checkVMAvailableResource(ns *corev1.Namespace, vmAvail *Quotas, updateVM *kubevirtv1.VirtualMachine) error {
	usedVMResource, err := q.getVMsTotalResources(ns, updateVM)
	if err != nil {
		return werror.NewInternalError(err.Error())
	}
	return cmpResource(vmAvail, usedVMResource)
}

func (q *AvailableResourceQuota) checkMaintenanceAvailableResource(ns *corev1.Namespace, maintAvail *Quotas, migrationVM *kubevirtv1.VirtualMachine) error {
	usedVMIMResource, err := q.getVMIMsTotalResources(ns, migrationVM)
	if err != nil {
		return werror.NewInternalError(err.Error())
	}
	return cmpResource(maintAvail, usedVMIMResource)
}

// Get all vms in the namespace and calculate the total resources
func (q *AvailableResourceQuota) getVMsTotalResources(ns *corev1.Namespace, updateVM *kubevirtv1.VirtualMachine) (corev1.ResourceList, error) {
	vms, err := q.vmCache.List(ns.Name, labels.Everything())
	if err != nil {
		return nil, err
	}

	vmrs, err := q.vmrCache.List(ns.Name, labels.Everything())
	if err != nil {
		return nil, err
	}

	// Get restoring vms
	restoringVMs := map[string]*kubevirtv1.VirtualMachineSpec{}
	for _, vmr := range vmrs {
		if *vmr.Status.Complete {
			continue
		}
		bak, err := q.vmbackupCache.Get(vmr.Spec.VirtualMachineBackupNamespace,
			vmr.Spec.VirtualMachineBackupName)
		if err != nil {
			return nil, err
		}
		restoringVMs[vmr.Spec.Target.Name] = &bak.Status.SourceSpec.Spec
	}

	totalResources := corev1.ResourceList{}
	// Create/Update VM
	if updateVM != nil {
		totalResources = quotautils.Add(totalResources, updateVM.Spec.Template.Spec.Domain.Resources.Limits)
	}
	for _, vm := range vms {
		// remove restoring vms if they are in the vm list
		if _, ok := restoringVMs[vm.Name]; ok {
			delete(restoringVMs, vm.Name)
		}

		// ignore the vm that is updating
		if updateVM != nil && updateVM.UID == vm.UID {
			continue
		}

		// check if vm is stopped status, ignore it
		if vm.DeletionTimestamp != nil || vm.Spec.RunStrategy == nil ||
			(*vm.Spec.RunStrategy == kubevirtv1.RunStrategyHalted &&
				(vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusStopped ||
					vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusTerminating)) {
			continue
		}

		totalResources = quotautils.Add(totalResources, vm.Spec.Template.Spec.Domain.Resources.Limits)
	}

	// add restoring vmspecs to totalresources
	for vmname, vmspec := range restoringVMs {
		// updateVM is restoring by create a new vm, ignore it
		if vmname == updateVM.Name {
			continue
		}
		totalResources = quotautils.Add(totalResources, vmspec.Template.Spec.Domain.Resources.Limits)
	}

	return totalResources, nil
}

// Get all vmims in the namespace and calculate the total resources
func (q *AvailableResourceQuota) getVMIMsTotalResources(ns *corev1.Namespace, migrationVM *kubevirtv1.VirtualMachine) (corev1.ResourceList, error) {
	vmims, err := q.vmimCache.List(ns.Name, labels.Everything())
	if err != nil {
		return nil, err
	}

	totalResources := corev1.ResourceList{}
	for _, vmim := range vmims {
		// check if vmim is completed status, ignore it
		if vmim.DeletionTimestamp != nil ||
			vmim.Status.Phase == kubevirtv1.MigrationSucceeded ||
			vmim.Status.Phase == kubevirtv1.MigrationFailed {
			continue
		}
		vm, err := q.vmCache.Get(vmim.Namespace, vmim.Spec.VMIName)
		if err != nil {
			return nil, err
		}

		totalResources = quotautils.Add(totalResources, vm.Spec.Template.Spec.Domain.Resources.Limits)
	}

	if migrationVM != nil {
		totalResources = quotautils.Add(totalResources, migrationVM.Spec.Template.Spec.Domain.Resources.Limits)
	}

	return totalResources, nil
}

// CheckVMAvailableResoruces Calculate vm(create/update) and vms if all resource > available resource, return error
func (q *AvailableResourceQuota) CheckVMAvailableResoruces(vm *kubevirtv1.VirtualMachine) error {
	ns, err := q.nsCache.Get(vm.Namespace)
	if err != nil {
		return err
	}

	calc, err := newCalculatorByAnnotations(ns.Annotations)
	if err != nil || calc == nil {
		return err
	}

	availableQuota, _, err := calc.calculateResources()
	if err != nil || (availableQuota == nil && err == nil) {
		return err
	}

	return q.checkVMAvailableResource(ns, availableQuota, vm)
}

// CheckMaintenanceAvailableResoruces Calculate vmims if all resource > available resource, return error
func (q *AvailableResourceQuota) CheckMaintenanceAvailableResoruces(vmim *kubevirtv1.VirtualMachineInstanceMigration) error {
	ns, err := q.nsCache.Get(vmim.Namespace)
	if err != nil {
		return err
	}

	migrateVM, err := q.vmCache.Get(vmim.Namespace, vmim.Spec.VMIName)
	if err != nil {
		return err
	}

	calc, err := newCalculatorByAnnotations(ns.Annotations)
	if err != nil || calc == nil {
		return err
	}

	_, maintenanceQuota, err := calc.calculateResources()
	if err != nil || (maintenanceQuota == nil && err == nil) {
		return err
	}

	return q.checkMaintenanceAvailableResource(ns, maintenanceQuota, migrateVM)
}

func CalcAvailableResourcesToString(resourceQuotaStr string, maintQuotaStr string) (
	vmAvailStr string, maintAvailStr string, err error) {

	calc, err := newCalculator(resourceQuotaStr, maintQuotaStr)
	if err != nil {
		return "", "", err
	}

	vmAvail, maintenanceAvail, err := calc.calculateResources()
	if err != nil {
		return "", "", err
	}

	vmAvailBytes, err := json.Marshal(vmAvail)
	if err != nil {
		return "", "", err
	}

	maintAvailBytes, err := json.Marshal(maintenanceAvail)
	if err != nil {
		return "", "", err
	}

	return string(vmAvailBytes), string(maintAvailBytes), nil
}

func cmpResource(avail *Quotas, usedResource corev1.ResourceList) error {
	if err := cmpCPUResource(avail, usedResource); err != nil {
		return err
	}
	if err := cmpMemoryResource(avail, usedResource); err != nil {
		return err
	}
	return nil
}

func cmpCPUResource(avail *Quotas, usedResource corev1.ResourceList) error {
	if cpus, ok := usedResource[corev1.ResourceCPU]; ok && !avail.Limit.LimitsCPU.IsZero() {
		if avail.Limit.LimitsCPU.Cmp(cpus) == -1 {
			return ErrVMAvailableResourceNotEnoughCPU
		}
	}
	return nil
}

func cmpMemoryResource(avail *Quotas, usedResource corev1.ResourceList) error {
	if memory, ok := usedResource[corev1.ResourceMemory]; ok && !avail.Limit.LimitsMemory.IsZero() {
		if avail.Limit.LimitsMemory.Cmp(memory) == -1 {
			return ErrVMAvailableResourceNotEnoughMemory
		}
	}
	return nil
}

func isPercentage(i int64) bool {
	return i >= 0 && i <= 100
}
