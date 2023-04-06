package resourcequota

import (
	"fmt"
	"runtime"

	"github.com/hashicorp/go-multierror"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/shopspring/decimal"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"
	kubevirtservices "kubevirt.io/kubevirt/pkg/virt-controller/services"

	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	ErrInsufficientResourcesFMT = "%s insufficient resources"
)

type resourceType string

var (
	zero = decimal.Zero

	AvailableResource   resourceType = "available"
	MaintenanceResource resourceType = "maintenance"
)

type MaintenancePercentage struct {
	nodeCache    v1.NodeCache
	podCache     v1.PodCache
	vmCache      ctlkubevirtv1.VirtualMachineCache
	vmimCache    ctlkubevirtv1.VirtualMachineInstanceMigrationCache
	settingCache v1beta1.SettingCache
}

func NewMaintenancePercentage(nodeCache v1.NodeCache,
	podCache v1.PodCache,
	vmCache ctlkubevirtv1.VirtualMachineCache,
	vmimCache ctlkubevirtv1.VirtualMachineInstanceMigrationCache,
	settingCache v1beta1.SettingCache,
) *MaintenancePercentage {
	return &MaintenancePercentage{
		nodeCache:    nodeCache,
		podCache:     podCache,
		vmCache:      vmCache,
		vmimCache:    vmimCache,
		settingCache: settingCache,
	}
}

func (m *MaintenancePercentage) IsGreaterThanMaintenanceResource(vmim *kubevirtv1.VirtualMachineInstanceMigration) (bool, error) {
	ok, err := m.IsLessAndEqualThanMaintenanceResource(vmim)
	return !ok, err
}

func (m *MaintenancePercentage) IsLessAndEqualThanMaintenanceResource(vmim *kubevirtv1.VirtualMachineInstanceMigration) (bool, error) {
	vmimsCPU, vmimsMem, err := m.getMigratingResources()
	if err != nil {
		return false, err
	}

	vmNameLabel := labels.Set{util.LabelVMName: vmim.Spec.VMIName}.AsSelector()
	pods, err := m.podCache.List(vmim.Namespace, vmNameLabel)

	for _, pod := range pods {
		if pod.DeletionTimestamp != nil {
			break
		}

		for _, container := range pod.Spec.Containers {
			vmimsCPU.Add(*container.Resources.Limits.Cpu())
			vmimsMem.Add(*container.Resources.Limits.Memory())
		}
		break
	}
	return m.isLessAndEqualThanMaintenanceResource(vmimsCPU, vmimsMem)
}

func (m *MaintenancePercentage) IsGreaterThanAvailableResource(vm *kubevirtv1.VirtualMachine) (bool, error) {
	ok, err := m.IsLessAndEqualThanAvailableResource(vm)
	return !ok, err
}

func (m *MaintenancePercentage) IsLessAndEqualThanAvailableResource(vm *kubevirtv1.VirtualMachine) (bool, error) {
	vmsCPU, vmsMem, err := m.getRunningVMResources(vm)
	if err != nil {
		return false, err
	}

	memOverhead := m.calculateVMActualOverhead(vm)

	vmCPU := vm.Spec.Template.Spec.Domain.Resources.Limits[corev1.ResourceCPU]
	vmMem := vm.Spec.Template.Spec.Domain.Resources.Limits[corev1.ResourceMemory]
	vmMem.Add(*memOverhead)

	vmsCPU.Add(vmCPU)
	vmsMem.Add(vmMem)

	return m.isLessAndEqualThanAvailableResource(vmsCPU, vmsMem)
}

func (m *MaintenancePercentage) getMigratingResources() (cpu, mem resource.Quantity, err error) {
	vmims, err := m.vmimCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return
	}

	for _, vmim := range vmims {
		if vmim.DeletionTimestamp != nil ||
			vmim.Status.Phase == kubevirtv1.MigrationSucceeded ||
			vmim.Status.Phase == kubevirtv1.MigrationFailed {
			continue
		}
		vmNameLabel := labels.Set{util.LabelVMName: vmim.Spec.VMIName}.AsSelector()
		pods, err := m.podCache.List(vmim.Namespace, vmNameLabel)

		if err != nil {
			return cpu, mem, err
		}

		for _, pod := range pods {
			if pod.DeletionTimestamp != nil {
				break
			}

			for _, container := range pod.Spec.Containers {
				cpu.Add(*container.Resources.Limits.Cpu())
				mem.Add(*container.Resources.Limits.Memory())
			}
			break
		}
	}
	return
}

func (m *MaintenancePercentage) getRunningVMResources(updateVM *kubevirtv1.VirtualMachine) (cpu, mem resource.Quantity, err error) {
	vmList, err := m.vmCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return
	}

	for _, vm := range vmList {
		if updateVM != nil && updateVM.UID == vm.UID {
			continue
		}

		if vm.DeletionTimestamp != nil || vm.Spec.RunStrategy == nil ||
			(*vm.Spec.RunStrategy == kubevirtv1.RunStrategyHalted &&
				(vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusStopped ||
					vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusTerminating)) {
			continue
		}

		vmNameLabel := labels.Set{util.LabelVMName: vm.Name}.AsSelector()
		pods, err := m.podCache.List(vm.Namespace, vmNameLabel)
		if err != nil {
			return cpu, mem, err
		}

		for _, pod := range pods {
			if pod.DeletionTimestamp != nil {
				break
			}

			for _, container := range pod.Spec.Containers {
				cpu.Add(*container.Resources.Limits.Cpu())
				mem.Add(*container.Resources.Limits.Memory())
			}
			break
		}
	}

	return
}

func (m *MaintenancePercentage) calculateVMActualOverhead(vm *kubevirtv1.VirtualMachine) *resource.Quantity {
	if vm.Spec.Template == nil || vm.Spec.Template.Spec.Domain.Resources.Limits == nil {
		return nil
	}

	vmi := &kubevirtv1.VirtualMachineInstance{
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Domain: kubevirtv1.DomainSpec{
				Resources: kubevirtv1.ResourceRequirements{
					Limits: vm.Spec.Template.Spec.Domain.Resources.Limits,
				},
			},
		},
	}

	overhead := kubevirtservices.GetMemoryOverhead(vmi, runtime.GOOS)
	mem := vm.Spec.Template.Spec.Domain.Resources.Limits.Memory()
	mem.Add(*overhead)

	return mem
}

func (m *MaintenancePercentage) isLessAndEqualThanMaintenanceResource(cpuQty, memoryQty resource.Quantity) (bool, error) {
	cpu := decimal.NewFromInt(cpuQty.MilliValue())
	memory := decimal.NewFromInt(memoryQty.Value())
	maintenanceCPU, maintenanceMemory, err := m.maintenanceResource()
	if err != nil {
		return false, err
	}

	if !maintenanceCPU.Equal(zero) && !cpu.LessThanOrEqual(maintenanceCPU) {
		return false, cpuInsufficientResourceError()
	}

	if !maintenanceMemory.Equal(zero) && !memory.LessThanOrEqual(maintenanceMemory) {
		return false, memInsufficientResourceError()
	}

	return true, nil
}

func (m *MaintenancePercentage) isLessAndEqualThanAvailableResource(cpuQty, memoryQty resource.Quantity) (bool, error) {
	cpu := decimal.NewFromInt(cpuQty.MilliValue())
	memory := decimal.NewFromInt(memoryQty.Value())
	availableCPU, availableMemory, err := m.availableResource()
	if err != nil {
		return false, err
	}

	if !availableCPU.Equal(zero) && !cpu.LessThanOrEqual(availableCPU) {
		return false, cpuInsufficientResourceError()
	}

	if !availableCPU.Equal(zero) && !memory.LessThanOrEqual(availableMemory) {
		return false, memInsufficientResourceError()
	}

	return true, nil
}

func (m *MaintenancePercentage) maintenanceResource() (decimal.Decimal, decimal.Decimal, error) {
	cpu, mem, err := m.ratio()
	if err != nil {
		return decimal.NewFromInt(0), decimal.NewFromInt(0), err
	}

	totalCPU, totalMemory := m.clusterResources()

	return cpu.Mul(totalCPU), mem.Mul(totalMemory), nil
}

func (m *MaintenancePercentage) availableResource() (decimal.Decimal, decimal.Decimal, error) {
	cpu, mem, err := m.ratio()
	if err != nil {
		return decimal.NewFromInt(0), decimal.NewFromInt(0), err
	}

	totalCPU, totalMemory := m.clusterResources()

	return totalCPU.Sub(cpu.Mul(totalCPU)), totalMemory.Sub(mem.Mul(totalMemory)), nil
}

func (m *MaintenancePercentage) calculateResourceQuota(rt resourceType) (decimal.Decimal, decimal.Decimal, error) {
	cpu, mem, err := m.ratio()
	if err != nil {
		return decimal.NewFromInt(0), decimal.NewFromInt(0), err
	}

	totalCPU, totalMemory := m.clusterResources()

	cpuMT := cpu.Mul(totalCPU)
	memMT := mem.Mul(totalMemory)
	if rt == AvailableResource {
		return totalCPU.Sub(cpuMT), totalMemory.Sub(memMT), nil
	} else if rt == MaintenanceResource {
		return cpuMT, memMT, nil
	}
	return zero, zero, nil
}

func (m *MaintenancePercentage) clusterResources() (decimal.Decimal, decimal.Decimal) {
	nodes, err := m.nodeCache.List(labels.Everything())
	if err != nil {
		return zero, zero
	}

	var (
		totalCPU    = zero
		totalMemory = zero
	)

	for _, node := range nodes {
		if isNotReady(node) || node.Spec.Unschedulable {
			continue
		}

		totalCPU = totalCPU.Add(decimal.NewFromInt(node.Status.Allocatable.Cpu().MilliValue()))
		totalMemory = totalMemory.Add(decimal.NewFromInt(node.Status.Allocatable.Memory().Value()))
	}

	return totalCPU, totalMemory
}

func isNotReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status != corev1.ConditionTrue {
			return true
		}
	}

	return false
}

func (m *MaintenancePercentage) ratio() (decimal.Decimal, decimal.Decimal, error) {
	cpu, cpuErr := m.getPercentageFromSettingKey("maintenance-cpu-percentage")
	memory, memErr := m.getPercentageFromSettingKey("maintenance-memory-percentage")

	return cpu, memory, multierror.Append(cpuErr, memErr).ErrorOrNil()
}

func (m *MaintenancePercentage) getPercentageFromSettingKey(key string) (decimal.Decimal, error) {
	setting, err := m.settingCache.Get(key)
	if err != nil {
		return zero, err
	}

	if setting.Value == "" {
		return zero, nil
	}

	percentage, err := decimal.NewFromString(setting.Value)
	if err != nil {
		return zero, err
	}

	return percentage.Div(decimal.NewFromInt(100)), nil
}

func insufficientResourceError(t string) error {
	return fmt.Errorf(ErrInsufficientResourcesFMT, t)
}

func cpuInsufficientResourceError() error {
	return insufficientResourceError("cpu")
}

func memInsufficientResourceError() error {
	return insufficientResourceError("memory")
}
