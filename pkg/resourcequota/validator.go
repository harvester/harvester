package resourcequota

import (
	"encoding/json"
	"fmt"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/shopspring/decimal"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
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
	var maintQuota MaintenanceQuota
	if err := json.Unmarshal([]byte(ns.Annotations[util.AnnotationMaintenanceQuota]), &maintQuota); err != nil {
		return err
	}

	if !(maintQuota.Limit.LimitsCpuPercent >= 0 && maintQuota.Limit.LimitsCpuPercent <= 100) {
		return fmt.Errorf(errInvalidQuotaFmt,
			"LimitsCpuPercent",
			maintQuota.Limit.LimitsCpuPercent)
	}
	if !(maintQuota.Limit.LimitsMemoryPercent >= 0 && maintQuota.Limit.LimitsMemoryPercent <= 100) {
		return fmt.Errorf(errInvalidQuotaFmt,
			"LimitsMemoryPercent",
			maintQuota.Limit.LimitsMemoryPercent)
	}

	return nil
}

func (q *AvailableResourceQuota) ValidateAvailableResources(ns *corev1.Namespace) error {
	if ns == nil || ns.DeletionTimestamp != nil || ns.Annotations == nil {
		return nil
	}

	vmAvail, maintAvail, err := calcAvailableResourcesByNamespace(ns)
	if err != nil || (vmAvail == nil && maintAvail == nil && err == nil) {
		return err
	}

	if err := q.checkVMAvailableResource(ns, vmAvail, nil); err != nil {
		return err
	}

	return q.checkMaintenanceAvailableResource(ns, maintAvail, nil)
}

func (q *AvailableResourceQuota) checkVMAvailableResource(ns *corev1.Namespace, vmAvail *ResourceAvailable, updateVM *kubevirtv1.VirtualMachine) error {
	usedVMResource, err := q.getVMsTotalResources(ns, updateVM)
	if err != nil {
		return werror.NewInternalError(err.Error())
	}
	return cmpResource(vmAvail, usedVMResource)
}

func (q *AvailableResourceQuota) checkMaintenanceAvailableResource(ns *corev1.Namespace, maintAvail *ResourceAvailable, migrationVM *kubevirtv1.VirtualMachine) error {
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
		totalResources = addResource(totalResources, updateVM.Spec.Template.Spec.Domain.Resources.Limits)
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

		totalResources = addResource(totalResources, vm.Spec.Template.Spec.Domain.Resources.Limits)
	}

	// add restoring vmspecs to totalresources
	for vmname, vmspec := range restoringVMs {
		// updateVM is restoring by create a new vm, ignore it
		if vmname == updateVM.Name {
			continue
		}
		totalResources = addResource(totalResources,
			vmspec.Template.Spec.Domain.Resources.Limits)
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

		totalResources = addResource(totalResources, vm.Spec.Template.Spec.Domain.Resources.Limits)
	}

	if migrationVM != nil {
		totalResources = addResource(totalResources, migrationVM.Spec.Template.Spec.Domain.Resources.Limits)
	}

	return totalResources, nil
}

// CheckVMAvailableResoruces Calculate vm(create/update) and vms if all resource > available resource, return error
func (q *AvailableResourceQuota) CheckVMAvailableResoruces(vm *kubevirtv1.VirtualMachine) error {
	ns, err := q.nsCache.Get(vm.Namespace)
	if err != nil {
		return err
	}

	vmAvail, _, err := calcAvailableResourcesByNamespace(ns)
	if err != nil || (vmAvail == nil && err == nil) {
		return err
	}

	return q.checkVMAvailableResource(ns, vmAvail, vm)
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

	_, maintAvail, err := calcAvailableResourcesByNamespace(ns)
	if err != nil || (maintAvail == nil && err == nil) {
		return err
	}

	return q.checkMaintenanceAvailableResource(ns, maintAvail, migrateVM)
}

// calcAvailableResources calculates the available resources for the VM and maintenance namespaces. The resources are calculated
// by comparing the current namespace resource quota to the desired maintenance quota. the maintenance quota must be percentage,
// the desired maintenance quota is calculated by multiplying the namespace resource quota by the percentage.
func calcAvailableResources(resourceQuotaStr string, maintQuotaStr string) (
	vmAvailStr *ResourceAvailable, maintAvailStr *ResourceAvailable, err error) {

	var (
		resourceQuota v3.NamespaceResourceQuota
		maintQuota    MaintenanceQuota
	)
	if err := json.Unmarshal([]byte(resourceQuotaStr), &resourceQuota); err != nil {
		return nil, nil, err
	} else if err := json.Unmarshal([]byte(maintQuotaStr), &maintQuota); err != nil {
		return nil, nil, err
	}

	vmAvail, maintAvail := &ResourceAvailable{}, &ResourceAvailable{}

	if maintQuota.Limit.LimitsCpuPercent > 0 {
		// Calculate the available CPU resources for the VM and the
		// available CPU resources for maintenance.
		vmAvail.Limit.LimitsCpu, maintAvail.Limit.LimitsCpu, err = calcResources(resourceQuota.Limit.LimitsCPU,
			maintQuota.Limit.LimitsCpuPercent)
		if err != nil {
			return nil, nil, err
		}
	}

	if maintQuota.Limit.LimitsMemoryPercent > 0 {
		// Calculate the available memory in bytes for the VM and the
		// available memory resources for maintenance.
		vmAvail.Limit.LimitsMemory, maintAvail.Limit.LimitsMemory, err = calcResources(resourceQuota.Limit.LimitsMemory,
			maintQuota.Limit.LimitsMemoryPercent)
		if err != nil {
			return nil, nil, err
		}
	}

	return vmAvail, maintAvail, nil
}

// Get available resource quota from namespace annotation
func calcAvailableResourcesByNamespace(ns *corev1.Namespace) (*ResourceAvailable, *ResourceAvailable, error) {
	resourceQuotaStr, ok1 := ns.Annotations[util.AnnotationResourceQuota]
	maintenanceQuotaStr, ok2 := ns.Annotations[util.AnnotationMaintenanceQuota]
	if !ok1 || !ok2 || resourceQuotaStr == "" || maintenanceQuotaStr == "" {
		return nil, nil, nil
	}

	vmAvail, maintAvail, err := calcAvailableResources(resourceQuotaStr, maintenanceQuotaStr)
	if err != nil {
		return nil, nil, err
	}
	return vmAvail, maintAvail, nil
}

func CalcAvailableResourcesToString(resourceQuotaStr string, maintQuotaStr string) (
	vmAvailStr string, maintAvailStr string, err error) {

	vmAvail, maintAvail, err := calcAvailableResources(resourceQuotaStr, maintQuotaStr)
	if err != nil {
		return "", "", err
	}

	vmAvailBytes, err := json.Marshal(vmAvail)
	if err != nil {
		return "", "", err
	}

	maintAvailBytes, err := json.Marshal(maintAvail)
	if err != nil {
		return "", "", err
	}

	return string(vmAvailBytes), string(maintAvailBytes), nil
}

func calcResources(limitStr string, percent int64) (vmAvail int64, maintAvail int64, err error) {
	quota, err := resource.ParseQuantity(limitStr)
	if err != nil {
		return 0, 0, err
	}

	maintQty, err := calcMaintResources(quota, percent)
	if err != nil {
		return 0, 0, err
	}

	vmQty := calcVmResources(quota, maintQty)
	return vmQty.MilliValue(), maintQty.MilliValue(), nil
}

// calcMaintResources calculates the amount of resources that can be
// reserved for maintenance. The maintenance ratio is a percentage of
// the total available resources in the cluster.
func calcMaintResources(quota resource.Quantity, percent int64) (resource.Quantity, error) {
	ratio := decimal.NewFromInt(percent).Div(decimal.NewFromInt(100))

	// Caclulate the maintenance available resources
	maintQty := *resource.NewMilliQuantity(
		decimal.NewFromInt(quota.MilliValue()).
			Mul(ratio).
			IntPart(),
		resource.DecimalSI)

	return maintQty, nil
}

func calcVmResources(quota resource.Quantity, maintQty resource.Quantity) resource.Quantity {
	// Caclulate the vm available resources
	quota.Sub(maintQty)
	return quota
}

func addResource(totalResource corev1.ResourceList, resource corev1.ResourceList) corev1.ResourceList {
	// Add resource to totalResource
	for k, v := range resource {
		if _, ok := totalResource[k]; ok {
			quantity := totalResource[k]
			quantity.Add(v)
			totalResource[k] = quantity
		} else {
			totalResource[k] = v
		}
	}
	return totalResource
}

func cmpResource(avail *ResourceAvailable, usedResource corev1.ResourceList) error {
	if err := cmpCPUResource(avail, usedResource); err != nil {
		return err
	}
	if err := cmpMemoryResource(avail, usedResource); err != nil {
		return err
	}
	return nil
}

func cmpCPUResource(avail *ResourceAvailable, usedResource corev1.ResourceList) error {
	if cpus, ok := usedResource[corev1.ResourceCPU]; ok && avail.Limit.LimitsCpu > 0 {
		limitsCPU := resource.NewMilliQuantity(avail.Limit.LimitsCpu, resource.DecimalSI)
		if limitsCPU.Cmp(cpus) == -1 {
			return ErrVMAvailableResourceNotEnoughCPU
		}
	}
	return nil
}

func cmpMemoryResource(avail *ResourceAvailable, usedResource corev1.ResourceList) error {
	if memory, ok := usedResource[corev1.ResourceMemory]; ok && avail.Limit.LimitsMemory > 0 {
		limitsMemory := resource.NewMilliQuantity(avail.Limit.LimitsMemory, resource.DecimalSI)
		if limitsMemory.Cmp(memory) == -1 {
			return ErrVMAvailableResourceNotEnoughMemory
		}
	}
	return nil
}
