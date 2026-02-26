package resourcequota

import (
	"encoding/json"
	"fmt"
	"runtime"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ctlv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"
	kubevirtservices "kubevirt.io/kubevirt/pkg/virt-controller/services"

	ctlharvestercorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
)

var resourceQuotaConversion = map[string]string{
	"limitsCpu":       string(corev1.ResourceLimitsCPU),
	"limitsMemory":    string(corev1.ResourceLimitsMemory),
	"requestsStorage": string(corev1.ResourceRequestsStorage),
}

type Calculator struct {
	nsCache      ctlv1.NamespaceCache
	podCache     ctlv1.PodCache
	rqCache      ctlharvestercorev1.ResourceQuotaCache
	vmimCache    ctlkubevirtv1.VirtualMachineInstanceMigrationCache
	settingCache ctlharvesterv1.SettingCache
}

func NewCalculator(
	nsCache ctlv1.NamespaceCache,
	podCache ctlv1.PodCache,
	rqCache ctlharvestercorev1.ResourceQuotaCache,
	vmimCache ctlkubevirtv1.VirtualMachineInstanceMigrationCache,
	settingCache ctlharvesterv1.SettingCache) *Calculator {

	return &Calculator{
		nsCache:      nsCache,
		podCache:     podCache,
		rqCache:      rqCache,
		vmimCache:    vmimCache,
		settingCache: settingCache,
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

func (c *Calculator) getNamespaceResourceQuotaAndRQ(namespace string) (*v3.NamespaceResourceQuota, *corev1.ResourceQuota, error) {
	nrq, err := c.getNamespaceResourceQuota(namespace)
	if err != nil {
		return nil, nil, err
	}
	if nrq == nil {
		return nil, nil, nil
	}

	// get resource quota limits from ResourceQuota
	rq, err := c.getNamespaceDefaultResourceQuota(namespace)
	if err != nil {
		return nil, nil, err
	}
	if rq == nil {
		return nil, nil, nil
	}

	return nrq, rq, nil
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

	nrq, rq, err := c.getNamespaceResourceQuotaAndRQ(vm.Namespace)
	if err != nil {
		return err
	}
	if nrq == nil {
		logrus.Debugf("CheckIfVMCanStartByResourceQuota: rancher resource quota is not set in the namespace %s, skip check", vm.Namespace)
		return nil
	}
	if rq == nil {
		logrus.Debugf("CheckIfVMCanStartByResourceQuota: default resource quota is not found in the namespace %s, skip check", vm.Namespace)
		return nil
	}

	return c.containsEnoughResourceQuotaToStartVM(vm, nrq, rq)
}

func (c *Calculator) getNamespaceResourceQuota(namespace string) (*v3.NamespaceResourceQuota, error) {
	// check namespace ResourceQuota
	ns, err := c.nsCache.Get(namespace)
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

	if resourceQuota.Limit.LimitsCPU == "" && resourceQuota.Limit.LimitsMemory == "" && resourceQuota.Limit.RequestsStorage == "" {
		return nil, nil
	}

	return resourceQuota, nil
}

// get the first default resourcequota from a given namespace
func (c *Calculator) getNamespaceDefaultResourceQuota(namespace string) (*corev1.ResourceQuota, error) {
	selector := labels.Set{util.LabelManagementDefaultResourceQuota: "true"}.AsSelector()
	rqs, err := c.rqCache.List(namespace, selector)
	if err != nil {
		return nil, err
	}
	if len(rqs) == 0 {
		return nil, nil
	}
	return rqs[0], nil
}

// containsEnoughResourceQuotaToStartVM checks if the VM can be started based on the namespace resource quota limits
func (c *Calculator) containsEnoughResourceQuotaToStartVM(
	vm *kubevirtv1.VirtualMachine,
	namespaceResourceQuota *v3.NamespaceResourceQuota,
	rq *corev1.ResourceQuota) error {
	// get running migrations' used resource
	// vmimsCPU, vmimsMem, _, err := c.getVMIMResourcesFromRQAnnotation(rq)
	vmimsCPU, vmimsMem, _, err := GetVMIMResourcesAndCompensationFromRQAnnotation(rq)
	if err != nil {
		return err
	}

	usedCPU := rq.Status.Used.Name(corev1.ResourceLimitsCPU, resource.DecimalSI)
	usedMem := rq.Status.Used.Name(corev1.ResourceLimitsMemory, resource.BinarySI)
	// calculate vm actual used resource
	// note: this calculation is not 100% accurate as the lifecycles of RQ and VMIM are different
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

	logrus.Debugf("%s/%s CPU: used %v, vmim %v vm %v actual %v, memory: used %v, vmim %v vm %v actual %v", vm.Namespace, vm.Name, usedCPU.MilliValue(), vmimsCPU.MilliValue(), vmCPU.MilliValue(), actualCPU.MilliValue(), usedMem.Value(), vmimsMem.Value(), vmMem.Value(), actualMem.Value())

	// check if remaining quotas on namespace are sufficient to run this VM
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

	memoryOverhead := kubevirtservices.GetMemoryOverhead(vmi, runtime.GOARCH, util.GetAdditionalGuestMemoryOverheadRatioWithoutError(c.settingCache))
	return &memoryOverhead
}

// get the auto-scaled VMIM and compensation resources from RQ
func GetVMIMResourcesAndCompensationFromRQAnnotation(rq *corev1.ResourceQuota) (cpu, mem, storage resource.Quantity, err error) {
	vms, err := getResourceListOfMigratingVMsFromRQ(rq)
	if err != nil {
		return cpu, mem, storage, err
	}

	compRl, err := getResourceListOfMigratingCompensationFromRQ(rq)
	if err != nil {
		return cpu, mem, storage, err
	}

	// from each migrating vm
	for _, rl := range vms {
		cpu.Add(*rl.Name(corev1.ResourceLimitsCPU, resource.DecimalSI))
		mem.Add(*rl.Name(corev1.ResourceLimitsMemory, resource.BinarySI))
		storage.Add(*rl.Name(corev1.ResourceRequestsStorage, resource.BinarySI))
	}

	// from the compensation
	cpu.Add(*compRl.Name(corev1.ResourceLimitsCPU, resource.DecimalSI))
	mem.Add(*compRl.Name(corev1.ResourceLimitsMemory, resource.BinarySI))
	storage.Add(*compRl.Name(corev1.ResourceRequestsStorage, resource.BinarySI))

	return
}

// Get ResourceQuota annotations about vmim and convert them to quantity
// each running vmim has a correspoding auto-sacling annotation to record the scaled resources
func GetVMIMResourcesFromRQAnnotation(rq *corev1.ResourceQuota) (cpu, mem, storage resource.Quantity, err error) {
	vms, err := getResourceListOfMigratingVMsFromRQ(rq)
	if err != nil {
		return cpu, mem, storage, err
	}

	for _, rl := range vms {
		cpu.Add(*rl.Name(corev1.ResourceLimitsCPU, resource.DecimalSI))
		mem.Add(*rl.Name(corev1.ResourceLimitsMemory, resource.BinarySI))
		storage.Add(*rl.Name(corev1.ResourceRequestsStorage, resource.BinarySI))
	}

	return
}

// Get ResourceQuota annotations about compensation
func GetCompensationFromRQAnnotation(rq *corev1.ResourceQuota) (cpu, mem, storage resource.Quantity, err error) {
	rl, err := getResourceListOfMigratingCompensationFromRQ(rq)
	if err != nil {
		return cpu, mem, storage, err
	}

	cpu.Add(*rl.Name(corev1.ResourceLimitsCPU, resource.DecimalSI))
	mem.Add(*rl.Name(corev1.ResourceLimitsMemory, resource.BinarySI))
	storage.Add(*rl.Name(corev1.ResourceRequestsStorage, resource.BinarySI))
	return
}

// Get Rancher NamespaceResourceQuota LimitsCPU and LimitsMemory
func GetCPUMemoryLimitsFromRancherNamespaceResourceQuota(nrq *v3.NamespaceResourceQuota) (cpu, mem resource.Quantity, err error) {
	if nrq.Limit.LimitsCPU == "" {
		cpu = *resource.NewQuantity(0, resource.DecimalSI)
	} else {
		if cpu, err = resource.ParseQuantity(nrq.Limit.LimitsCPU); err != nil {
			return
		}
	}

	if nrq.Limit.LimitsMemory == "" {
		mem = *resource.NewQuantity(0, resource.BinarySI)
	} else {
		if mem, err = resource.ParseQuantity(nrq.Limit.LimitsMemory); err != nil {
			return
		}
	}
	return
}

func (c *Calculator) getVMPods(namespace, vmName string) ([]*corev1.Pod, error) {
	return c.podCache.GetByIndex(indexeresutil.PodByVMNameIndex, ref.Construct(namespace, vmName))
}

func (c *Calculator) CheckStorageResourceQuota(vm *kubevirtv1.VirtualMachine, oldVM *kubevirtv1.VirtualMachine) error {
	nrq, rq, err := c.getNamespaceResourceQuotaAndRQ(vm.Namespace)
	if err != nil {
		return err
	}
	if nrq == nil {
		logrus.Debugf("CheckStorageResourceQuota: resource quota not found in the namespace %s, skip check", vm.Namespace)
		return nil
	}
	if rq == nil {
		logrus.Debugf("CheckStorageResourceQuota: not found any default resource quota in the namespace %s, skip check", vm.Namespace)
		return nil
	}

	// _, _, vmimsStorage, err := c.getVMIMResourcesFromRQAnnotation(rq)
	_, _, vmimsStorage, err := GetVMIMResourcesAndCompensationFromRQAnnotation(rq)
	if err != nil {
		return err
	}

	usedStorage := rq.Status.Used.Name(corev1.ResourceRequestsStorage, resource.BinarySI)
	usedStorage.Sub(vmimsStorage)

	// Calculate the storage quantity of the VM.
	vmStorage, err := calculateVMStorageQuantity(vm)
	if err != nil {
		return err
	}

	// Calculate the storage quantity of the old VM.
	// Then compare both storage quantity to assess whether the namespace
	// resource quota can be exceeded at all. This is only the case when the
	// storage requirements of the new VM is greater than those of the old
	// one.
	if oldVM != nil {
		oldVMStorage, err := calculateVMStorageQuantity(oldVM)
		if err != nil {
			return err
		}
		// If the storage quantity of the VM is smaller than the old
		// one, then exit immediately because the namespace resource
		// quota cannot be exceeded in this case.
		if vmStorage.Cmp(oldVMStorage) != 1 {
			return nil
		}
		// Use the difference of the storage quantities as only this value
		// is decisive for the further assessment.
		vmStorage.Sub(oldVMStorage)
	}

	actualRq, err := convertNamespaceResourceLimitToResourceList(&nrq.Limit)
	if err != nil {
		return err
	}
	actualStorage := actualRq.Name(corev1.ResourceRequestsStorage, resource.BinarySI)

	if !actualStorage.IsZero() {
		actualStorage.Sub(*usedStorage)
		if actualStorage.Cmp(vmStorage) == -1 {
			return storageInsufficientResourceError()
		}
	}

	return nil
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
	ratio *string,
) (needUpdate bool, toUpdate *corev1.ResourceQuota, rl corev1.ResourceList) {

	vmiLimits := vmi.Spec.Domain.Resources.Limits
	if !checkResourceQuotaAndVMI(rq, vmiLimits) {
		return false, nil, nil
	}

	rl = corev1.ResourceList{}

	_, cpuOK := rq.Spec.Hard[corev1.ResourceLimitsCPU]
	if !vmiLimits.Cpu().IsZero() && cpuOK {
		rl[corev1.ResourceLimitsCPU] = vmiLimits[corev1.ResourceCPU]
	}

	_, memOK := rq.Spec.Hard[corev1.ResourceLimitsMemory]
	if !vmiLimits.Memory().IsZero() && memOK {
		mem := vmiLimits[corev1.ResourceMemory]
		mem.Add(kubevirtservices.GetMemoryOverhead(vmi, runtime.GOARCH, ratio))
		rl[corev1.ResourceLimitsMemory] = mem
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

// If base is zero, delta is not added
func CalculateNewResourceQuotaFromBaseDelta(rq *corev1.ResourceQuota, cpuBase, memBase, cpuDelta, memDelta, cpuCompensation, memCompensation resource.Quantity) (*corev1.ResourceQuota, bool) {
	needUpdate := false
	if !cpuBase.IsZero() {
		cpuBase.Add(cpuDelta)
		cpuBase.Add(cpuCompensation)
		if !rq.Spec.Hard[corev1.ResourceLimitsCPU].Equal(cpuBase) {
			needUpdate = true
			rq.Spec.Hard[corev1.ResourceLimitsCPU] = cpuBase
		}
	}
	if !memBase.IsZero() {
		memBase.Add(memDelta)
		memBase.Add(memCompensation)
		if !rq.Spec.Hard[corev1.ResourceLimitsMemory].Equal(memBase) {
			needUpdate = true
			rq.Spec.Hard[corev1.ResourceLimitsMemory] = memBase
		}
	}
	return rq, needUpdate
}

func checkResourceQuotaAndVMI(rq *corev1.ResourceQuota, limits corev1.ResourceList) bool {
	if isEmpty(rq) {
		return false
	}

	if limits.Cpu().IsZero() && limits.Memory().IsZero() {
		return false
	}
	return true
}

func isEmpty(rq *corev1.ResourceQuota) bool {
	if rq == nil {
		return true
	}
	hard := rq.Spec.Hard
	if hard == nil ||
		(hard.Name(corev1.ResourceLimitsCPU, resource.DecimalSI).IsZero() &&
			hard.Name(corev1.ResourceLimitsMemory, resource.BinarySI).IsZero()) {
		return true
	}
	return false
}

func isMemoryLimitEmpty(rq *corev1.ResourceQuota) bool {
	if rq == nil {
		return true
	}
	hard := rq.Spec.Hard
	if hard == nil || hard.Name(corev1.ResourceLimitsMemory, resource.DecimalSI).IsZero() {
		return true
	}
	return false
}

func IsEmptyResourceQuota(rq *corev1.ResourceQuota) bool {
	return isEmpty(rq)
}

func calculateVMStorageQuantity(vm *kubevirtv1.VirtualMachine) (resource.Quantity, error) {
	storage := *resource.NewQuantity(0, resource.BinarySI)

	volumeClaimTemplates, ok := vm.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok || volumeClaimTemplates == "" {
		return storage, nil
	}

	var pvcs []*corev1.PersistentVolumeClaim
	err := json.Unmarshal([]byte(volumeClaimTemplates), &pvcs)
	if err != nil {
		return storage, fmt.Errorf("failed to unmarshal the volumeClaimTemplates annotation: %w", err)
	}

	for _, pvc := range pvcs {
		storage.Add(*pvc.Spec.Resources.Requests.Storage())
	}

	return storage, nil
}

func CalculateCompensationResourceQuotaWithVMI(
	rq *corev1.ResourceQuota,
	vmi *kubevirtv1.VirtualMachineInstance,
	ratio *string,
) (needUpdate bool, toUpdate *corev1.ResourceQuota, rl corev1.ResourceList) {
	vmiLimits := vmi.Spec.Domain.Resources.Limits
	if isMemoryLimitEmpty(rq) || vmiLimits.Memory().IsZero() {
		return false, nil, nil
	}

	rl = corev1.ResourceList{}
	mem := vmiLimits[corev1.ResourceMemory]
	mem.Add(kubevirtservices.GetMemoryOverhead(vmi, runtime.GOARCH, ratio)) // add overhead

	// hard limit:
	limitMem := rq.Spec.Hard.Name(corev1.ResourceLimitsMemory, resource.BinarySI)
	// used
	usedMem := rq.Status.Used.Name(corev1.ResourceLimitsMemory, resource.BinarySI)

	usedMem.Add(mem)
	// used + migration target pod exceeds limit, compensate the delta
	if usedMem.Cmp(*limitMem) == 1 {
		usedMem.Sub(*limitMem)
		usedMem.Add(*resource.NewQuantity(additionalCompensationMemeory, resource.BinarySI))
		rl[corev1.ResourceLimitsMemory] = *usedMem
		return true, rq, rl
	}

	return false, rq, rl
}
