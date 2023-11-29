package resourcequota

import (
	"encoding/json"
	"fmt"
	"runtime"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ctlv1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"
	kubevirtservices "kubevirt.io/kubevirt/pkg/virt-controller/services"

	ctlharvestercorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/util"
)

var resourceQuotaConversion = map[string]string{
	"limitsCpu":    "limits.cpu",
	"limitsMemory": "limits.memory",
}

type Calculator struct {
	nsCache   ctlv1.NamespaceCache
	podCache  ctlv1.PodCache
	rqCache   ctlharvestercorev1.ResourceQuotaCache
	vmimCache ctlkubevirtv1.VirtualMachineInstanceMigrationCache
}

func NewCalculator(
	nsCache ctlv1.NamespaceCache,
	podCache ctlv1.PodCache,
	rqCache ctlharvestercorev1.ResourceQuotaCache,
	vmimCache ctlkubevirtv1.VirtualMachineInstanceMigrationCache) *Calculator {

	return &Calculator{
		nsCache:   nsCache,
		podCache:  podCache,
		rqCache:   rqCache,
		vmimCache: vmimCache,
	}
}

// VMPodsExist checks if the VM pod exists
func (c *Calculator) VMPodsExist(namespace, vmName string) (bool, error) {
	pods, err := c.getVMPods(namespace, vmName)
	if err != nil {
		return false, err
	} else if len(pods) == 0 {
		return false, nil
	}
	return true, nil
}

// CheckIfVMCanStartByResourceQuota checks if the VM can be started based on the resource quota limits
func (c *Calculator) CheckIfVMCanStartByResourceQuota(vm *kubevirtv1.VirtualMachine) error {
	// return if the VM status is empty.
	if vm.Status.PrintableStatus == "" {
		return nil
	}

	strategy, err := vm.RunStrategy()
	if err != nil {
		return err
	}
	if strategy == kubevirtv1.RunStrategyHalted {
		logrus.Debugf("CheckIfVMCanStartByResourceQuota: VM %s/%s is halted, skip check", vm.Namespace, vm.Name)
		return nil
	}

	exist, err := c.VMPodsExist(vm.Namespace, vm.Name)
	if err != nil {
		return err
	}
	if exist {
		logrus.Debugf("CheckIfVMCanStartByResourceQuota: VM %s/%s has running pod, skip check", vm.Namespace, vm.Name)
		return nil
	}

	nrq, err := c.getNamespaceResourceQuota(vm)
	if err != nil {
		return err
	}
	if nrq == nil {
		logrus.Debugf("CheckIfVMCanStartByResourceQuota: skipping check, resource quota not found in the namespace %s", vm.Namespace)
		return nil
	}

	// get resource quota limits from ResourceQuota
	selector := labels.Set{util.LabelManagementDefaultResourceQuota: "true"}.AsSelector()
	rqs, err := c.rqCache.List(vm.Namespace, selector)
	if err != nil {
		return err
	} else if len(rqs) == 0 {
		logrus.Debugf("CheckIfVMCanStartByResourceQuota: no found any default resource quota in the namespace %s", vm.Namespace)
		return nil
	}

	return c.containsEnoughResourceQuotaToStartVM(vm, nrq, rqs[0])
}

func (c *Calculator) getNamespaceResourceQuota(vm *kubevirtv1.VirtualMachine) (*v3.NamespaceResourceQuota, error) {
	// check namespace ResourceQuota
	ns, err := c.nsCache.Get(vm.Namespace)
	if err != nil {
		return nil, err
	}

	// if not ResourceQuota annotation in namespace, return nil
	var resourceQuota *v3.NamespaceResourceQuota
	if rqStr, ok := ns.Annotations[util.CattleAnnotationResourceQuota]; !ok {
		return nil, nil
	} else if err := json.Unmarshal([]byte(rqStr), &resourceQuota); err != nil {
		return nil, err
	}

	if resourceQuota.Limit.LimitsCPU == "" && resourceQuota.Limit.LimitsMemory == "" {
		return nil, nil
	}

	return resourceQuota, nil
}

// containsEnoughResourceQuotaToStartVM checks if the VM can be started based on the namespace resource quota limits
func (c *Calculator) containsEnoughResourceQuotaToStartVM(
	vm *kubevirtv1.VirtualMachine,
	namespaceResourceQuota *v3.NamespaceResourceQuota,
	rq *corev1.ResourceQuota) error {
	// get running migrations' used resource
	vmimsCPU, vmimsMem, err := c.getRunningVMIMResources(rq)
	if err != nil {
		return err
	}

	usedCPU := rq.Status.Used.Name(corev1.ResourceLimitsCPU, resource.DecimalSI)
	usedMem := rq.Status.Used.Name(corev1.ResourceLimitsMemory, resource.BinarySI)
	// calculate vm actual used resource
	usedCPU.Sub(vmimsCPU)
	usedMem.Sub(vmimsMem)

	memOverhead := c.calculateVMActualOverhead(vm)
	vmCPU := vm.Spec.Template.Spec.Domain.Resources.Limits[corev1.ResourceCPU]
	vmMem := vm.Spec.Template.Spec.Domain.Resources.Limits[corev1.ResourceMemory]
	vmMem.Add(*memOverhead)

	actualRq, err := convertNamespaceResourceLimitToResourceList(&namespaceResourceQuota.Limit)
	if err != nil {
		return err
	}
	actualCPU := actualRq.Name(corev1.ResourceLimitsCPU, resource.DecimalSI)
	actualMem := actualRq.Name(corev1.ResourceLimitsMemory, resource.BinarySI)

	// check if vms actual used resource is less than namespace resource quota limits
	// if not, return insufficient resource error
	if !actualCPU.IsZero() {
		actualCPU.Sub(*usedCPU)
		if actualCPU.Cmp(vmCPU) == -1 {
			return cpuInsufficientResourceError()
		}
	}
	if !actualMem.IsZero() {
		actualMem.Sub(*usedMem)
		if actualMem.Cmp(vmMem) == -1 {
			return memInsufficientResourceError()
		}
	}

	return nil
}

func (c *Calculator) calculateVMActualOverhead(vm *kubevirtv1.VirtualMachine) *resource.Quantity {
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

	memoryOverhead := kubevirtservices.GetMemoryOverhead(vmi, runtime.GOARCH, nil)
	return &memoryOverhead
}

func (c *Calculator) getRunningVMIMResources(rq *corev1.ResourceQuota) (cpu, mem resource.Quantity, err error) {
	vms, err := GetResourceListFromMigratingVMs(rq)
	if err != nil {
		return cpu, mem, err
	}

	for _, rl := range vms {
		cpu.Add(*rl.Name(corev1.ResourceLimitsCPU, resource.DecimalSI))
		mem.Add(*rl.Name(corev1.ResourceLimitsMemory, resource.BinarySI))
	}

	return
}

func (c *Calculator) getVMPods(namespace, vmName string) ([]*corev1.Pod, error) {
	return c.podCache.GetByIndex(indexeres.PodByVMNameIndex, fmt.Sprintf("%s/%s", namespace, vmName))
}

func convertNamespaceResourceLimitToResourceList(limit *v3.ResourceQuotaLimit) (corev1.ResourceList, error) {
	in, err := json.Marshal(limit)
	if err != nil {
		return nil, err
	}
	limitsMap := map[string]string{}
	if err = json.Unmarshal(in, &limitsMap); err != nil {
		return nil, err
	}

	limits := corev1.ResourceList{}
	for key, value := range limitsMap {
		var resourceName corev1.ResourceName
		if val, ok := resourceQuotaConversion[key]; ok {
			resourceName = corev1.ResourceName(val)
		} else {
			resourceName = corev1.ResourceName(key)
		}

		resourceQuantity, err := resource.ParseQuantity(value)
		if err != nil {
			return nil, err
		}

		limits[resourceName] = resourceQuantity
	}
	return limits, nil
}

func CalculateScaleResourceQuotaWithVMI(
	rq *corev1.ResourceQuota,
	vmi *kubevirtv1.VirtualMachineInstance,
) (needUpdate bool, toUpdate *corev1.ResourceQuota, rl corev1.ResourceList) {

	vmiLimits := vmi.Spec.Domain.Resources.Limits
	if !checkResourceQuotaAndVMI(rq, vmiLimits) {
		return false, nil, nil
	}

	rl = corev1.ResourceList{}
	currentCPULimit, cpuOK := rq.Spec.Hard[corev1.ResourceLimitsCPU]
	if !vmiLimits.Cpu().IsZero() && cpuOK {
		currentCPULimit.Add(vmiLimits[corev1.ResourceCPU])
		rl[corev1.ResourceLimitsCPU] = vmiLimits[corev1.ResourceCPU]

		rq.Spec.Hard[corev1.ResourceLimitsCPU] = currentCPULimit
	}

	currentMemoryLimit, memOK := rq.Spec.Hard[corev1.ResourceLimitsMemory]
	if !vmiLimits.Memory().IsZero() && memOK {
		mem := vmiLimits[corev1.ResourceMemory]
		mem.Add(kubevirtservices.GetMemoryOverhead(vmi, runtime.GOARCH, nil))

		currentMemoryLimit.Add(mem)
		rl[corev1.ResourceLimitsMemory] = mem

		rq.Spec.Hard[corev1.ResourceLimitsMemory] = currentMemoryLimit
	}

	return true, rq, rl
}

func CalculateRestoreResourceQuotaWithVMI(
	rq *corev1.ResourceQuota,
	vmi *kubevirtv1.VirtualMachineInstance,
	rl corev1.ResourceList,
) (needUpdate bool, toUpdate *corev1.ResourceQuota) {

	vmiLimits := vmi.Spec.Domain.Resources.Limits
	if !checkResourceQuotaAndVMI(rq, vmiLimits) {
		return false, nil
	}

	currentCPULimit, cpuOK := rq.Spec.Hard[corev1.ResourceLimitsCPU]
	if cpuOK && !vmiLimits.Cpu().IsZero() {
		currentCPULimit.Sub(rl[corev1.ResourceLimitsCPU])

		rq.Spec.Hard[corev1.ResourceLimitsCPU] = currentCPULimit
	}

	currentMemoryLimit, memOK := rq.Spec.Hard[corev1.ResourceLimitsMemory]
	if memOK && !vmiLimits.Memory().IsZero() {
		currentMemoryLimit.Sub(rl[corev1.ResourceLimitsMemory])

		rq.Spec.Hard[corev1.ResourceLimitsMemory] = currentMemoryLimit
	}

	return true, rq
}

func checkResourceQuotaAndVMI(rq *corev1.ResourceQuota, limits corev1.ResourceList) bool {
	if isEmptyQuota(rq) {
		return false
	}

	if limits.Cpu().IsZero() && limits.Memory().IsZero() {
		return false
	}
	return true
}

func isEmptyQuota(rq *corev1.ResourceQuota) bool {
	hard := rq.Spec.Hard
	if hard == nil ||
		(hard.Name(corev1.ResourceLimitsCPU, resource.DecimalSI).IsZero() &&
			hard.Name(corev1.ResourceLimitsMemory, resource.BinarySI).IsZero()) {
		return true
	}
	return false
}
