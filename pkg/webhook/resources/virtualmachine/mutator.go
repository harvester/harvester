package virtualmachine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/virtualmachine"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	networkGroup      = "network.harvesterhci.io"
	keyClusterNetwork = networkGroup + "/clusternetwork"

	memory10M  = 10485760
	memory100M = 104857600
	memory256M = 268435456
)

func NewMutator(
	setting ctlharvesterv1.SettingCache,
	nad ctlcniv1.NetworkAttachmentDefinitionCache,
) types.Mutator {
	return &vmMutator{
		setting: setting,
		nad:     nad,
	}
}

type vmMutator struct {
	types.DefaultMutator
	setting ctlharvesterv1.SettingCache
	nad     ctlcniv1.NetworkAttachmentDefinitionCache
}

func (m *vmMutator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"virtualmachines"},
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

func (m *vmMutator) Create(_ *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	vm := newObj.(*kubevirtv1.VirtualMachine)

	logrus.Debugf("create VM %s/%s", vm.Namespace, vm.Name)

	patchOps, err := m.patchResourceOvercommit(vm)
	if err != nil {
		return patchOps, err
	}

	patchOps, err = m.patchAffinity(vm, patchOps)
	if err != nil {
		return nil, err
	}

	patchOps, err = m.patchTerminationGracePeriodSeconds(vm, patchOps)
	if err != nil {
		return nil, err
	}

	return patchOps, nil
}

func (m *vmMutator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	newVM := newObj.(*kubevirtv1.VirtualMachine)
	oldVM := oldObj.(*kubevirtv1.VirtualMachine)

	if newVM == nil || newVM.DeletionTimestamp != nil {
		return nil, nil
	}

	logrus.Debugf("update VM %s/%s", newVM.Namespace, newVM.Name)

	var patchOps types.PatchOps
	var err error

	if needUpdateResourceOvercommit(oldVM, newVM) {
		patchOps, err = m.patchResourceOvercommit(newVM)
	}
	if err != nil {
		return patchOps, err
	}

	needUpdateRunStrategy, err := needUpdateRunStrategy(oldVM, newVM)
	if err != nil {
		return patchOps, err
	}

	if needUpdateRunStrategy {
		patchOps = patchRunStrategy(newVM, patchOps)
	}

	patchOps, err = m.patchAffinity(newVM, patchOps)
	if err != nil {
		return nil, err
	}

	patchOps, err = m.patchTerminationGracePeriodSeconds(newVM, patchOps)
	if err != nil {
		return nil, err
	}

	return patchOps, nil
}

func needUpdateRunStrategy(oldVM, newVM *kubevirtv1.VirtualMachine) (bool, error) {
	// no need to patch the run strategy if user uses the spec.running filed.
	if newVM.Spec.Running != nil {
		return false, nil
	}

	newRunStrategy, err := newVM.RunStrategy()
	if err != nil {
		return false, err
	}

	oldRunStrategy, err := oldVM.RunStrategy()
	if err != nil {
		return false, err
	}

	if oldRunStrategy == kubevirtv1.RunStrategyHalted && newRunStrategy != kubevirtv1.RunStrategyHalted {
		return true, nil
	}
	return false, nil
}

// add workaround for the issue https://github.com/kubevirt/kubevirt/issues/7295
func patchRunStrategy(newVM *kubevirtv1.VirtualMachine, patchOps types.PatchOps) types.PatchOps {
	runStrategy := newVM.Annotations[util.AnnotationRunStrategy]
	if string(runStrategy) == "" {
		runStrategy = string(kubevirtv1.RunStrategyRerunOnFailure)
	}
	patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/runStrategy", "value": "%s"}`, runStrategy))
	return patchOps
}

func needUpdateResourceOvercommit(oldVM, newVM *kubevirtv1.VirtualMachine) bool {
	var newReservedMemory, oldReservedMemory string
	newLimits := newVM.Spec.Template.Spec.Domain.Resources.Limits
	newCPU := newLimits.Cpu()
	newMem := newLimits.Memory()
	if newVM.Annotations != nil {
		newReservedMemory = newVM.Annotations[util.AnnotationReservedMemory]
	}

	oldLimits := oldVM.Spec.Template.Spec.Domain.Resources.Limits
	oldCPU := oldLimits.Cpu()
	oldMem := oldLimits.Memory()
	if oldVM.Annotations != nil {
		oldReservedMemory = oldVM.Annotations[util.AnnotationReservedMemory]
	}

	if !newCPU.IsZero() && (oldCPU.IsZero() || !newCPU.Equal(*oldCPU)) {
		return true
	}
	if !newMem.IsZero() && (oldMem.IsZero() || !newMem.Equal(*oldMem)) {
		return true
	}
	if newReservedMemory != oldReservedMemory {
		return true
	}
	if isDedicatedCPU(oldVM) != isDedicatedCPU(newVM) {
		return true
	}

	return hostDevicesOvercommitNeeded(oldVM, newVM)
}

func (m *vmMutator) patchResourceOvercommit(vm *kubevirtv1.VirtualMachine) ([]string, error) {
	var patchOps types.PatchOps
	limits := vm.Spec.Template.Spec.Domain.Resources.Limits
	cpu := limits.Cpu()
	mem := limits.Memory()
	requestsMissing := len(vm.Spec.Template.Spec.Domain.Resources.Requests) == 0
	requestsToMutate := v1.ResourceList{}
	overcommit, err := m.getOvercommit()
	if err != nil || overcommit == nil {
		return patchOps, err
	}
	agmorc, err := m.getAdditionalGuestMemoryOverheadRatioConfig()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace": vm.Namespace,
			"name":      vm.Name,
		}).Warnf("fail to get setting %v, fallback to legacy reserved memory strategy, error: %v", settings.AdditionalGuestMemoryOverheadRatioName, err.Error())
		agmorc = nil
		err = nil
	}

	if !cpu.IsZero() {
		var newRequest int64
		if isDedicatedCPU(vm) {
			// do not apply overcommitted resource since dedicated CPU requires guaranteed QoS
			// more info, please check https://github.com/kubevirt/kubevirt/blob/8fe1d71accd7d6f5837de514d6b9ddc782c5dd41/pkg/virt-api/webhooks/validating-webhook/admitters/vmi-create-admitter.go#L619
			newRequest = cpu.MilliValue()
		} else {
			newRequest = cpu.MilliValue() * int64(100) / int64(overcommit.CPU)
		}
		quantity := resource.NewMilliQuantity(newRequest, cpu.Format)
		if requestsMissing {
			requestsToMutate[v1.ResourceCPU] = *quantity
		} else {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/cpu", "value": "%s"}`, quantity))
		}
	}
	if !mem.IsZero() {
		memPatch, err := generateMemoryPatch(vm, mem, overcommit, agmorc, requestsMissing, requestsToMutate)
		if err != nil {
			return patchOps, err
		}
		patchOps = append(patchOps, memPatch...)
	}
	if len(requestsToMutate) > 0 {
		bytes, err := json.Marshal(requestsToMutate)
		if err != nil {
			return patchOps, err
		}
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests", "value": %s}`, string(bytes)))
	}
	return patchOps, nil
}

func generateMemoryPatch(vm *kubevirtv1.VirtualMachine, mem *resource.Quantity, overcommit *settings.Overcommit, agmorc *settings.AdditionalGuestMemoryOverheadRatioConfig, requestsMissing bool, requestsToMutate v1.ResourceList) (types.PatchOps, error) {
	// Truncate to MiB
	var patchOps types.PatchOps
	var err error
	newRequest := mem.Value() * int64(100) / int64(overcommit.Memory) / 1048576 * 1048576
	quantity := resource.NewQuantity(newRequest, mem.Format)
	enableCPUAndMemoryHotplug := virtualmachine.SupportCPUAndMemoryHotplug(vm)

	// Reserve 100MiB (104857600 Bytes) for QEMU on guest memory
	// Ref: https://github.com/harvester/harvester/issues/1234
	// TODO: handle hugepage memory
	reservedMemory := *resource.NewQuantity(memory100M, resource.BinarySI)
	useReservedMemory := true

	// user has set AnnotationReservedMemory, then use it anyway
	if vm.Annotations != nil && vm.Annotations[util.AnnotationReservedMemory] != "" {
		reservedMemory, err = resource.ParseQuantity(vm.Annotations[util.AnnotationReservedMemory])
		if err != nil {
			return patchOps, fmt.Errorf("annotation %v can't be converted to memory unit", vm.Annotations[util.AnnotationReservedMemory])
		}
		// already checked on validator: reservedMemory < mem
		if reservedMemory.Value() >= mem.Value() {
			return patchOps, fmt.Errorf("reservedMemory can't be equal or greater than limits.memory  %v %v", reservedMemory.Value(), mem.Value())
		}
	} else {
		// if user has set a valid AdditionalGuestMemoryOverheadRatio value
		if agmorc != nil && !agmorc.IsEmpty() {
			useReservedMemory = false
		}
	}

	// when AnnotationReservedMemory is set, or AdditionalGuestMemoryOverheadRatioConfig is not set
	// if AdditionalGuestMemoryOverheadRatioConfig and AdditionalGuestMemoryOverheadRatioConfig are both set
	// then the VM will benefit from both

	// if hotpluggable guest memory is set by the user, use it for subsequent calculations.
	// otherwise, set the initial guest memory to be the same as the vm's memory limit.
	guestMemory := *mem
	if enableCPUAndMemoryHotplug && vm.Spec.Template.Spec.Domain.Memory != nil && vm.Spec.Template.Spec.Domain.Memory.Guest != nil {
		guestMemory = *vm.Spec.Template.Spec.Domain.Memory.Guest
	}

	if useReservedMemory {
		guestMemory.Sub(reservedMemory)
	}
	if guestMemory.Value() < memory256M {
		// some test cases use a small value to test, but for production such VM makes no sense, add a warning here
		logrus.WithFields(logrus.Fields{
			"namespace": vm.Namespace,
			"name":      vm.Name,
		}).Warnf("guest memory is under the suggested minimum requirement (256 Mi), original: %v, final: %v", mem.Value(), guestMemory.Value())
	}
	if guestMemory.Value() < memory10M {
		// should not be < 10 Mi
		return patchOps, fmt.Errorf("guest memory is under the minimum requirement (10 Mi), original: %v, final: %v", mem.Value(), guestMemory.Value())
	}

	if isDedicatedCPU(vm) {
		// do not apply overcommitted resource since dedicated CPU requires guaranteed QoS
		// more info, please check https://github.com/kubevirt/kubevirt/blob/8fe1d71accd7d6f5837de514d6b9ddc782c5dd41/pkg/virt-api/webhooks/validating-webhook/admitters/vmi-create-admitter.go#L619
		quantity = mem
	} else if hostDevicesPresent(vm) {
		// Needed to avoid issue due to overcommit when GPU or HostDevices are passed to a VM.
		// Addresses issue: `https://github.com/kubevirt/kubevirt/issues/10379`
		// This condition can be removed once a fix is available upstream
		guestMemory.DeepCopyInto(quantity)
	}

	if requestsMissing {
		requestsToMutate[v1.ResourceMemory] = *quantity
	} else {
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "%s"}`, quantity))
	}

	// patch guest memory
	if vm.Spec.Template.Spec.Domain.Memory == nil {
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"%s"}}`, &guestMemory))
	} else if !enableCPUAndMemoryHotplug {
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/memory/guest", "value": "%s"}`, &guestMemory))
	}

	// patch maxSockets
	if !enableCPUAndMemoryHotplug {
		patchOps = append(patchOps, `{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`)
	}

	return patchOps, nil
}

func (m *vmMutator) getOvercommit() (*settings.Overcommit, error) {
	s, err := m.setting.Get(settings.OvercommitConfigSettingName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	value := s.Value
	if value == "" {
		value = s.Default
	}
	if value == "" {
		return nil, nil
	}
	overcommit := &settings.Overcommit{}
	if err := json.Unmarshal([]byte(value), overcommit); err != nil {
		return overcommit, err
	}
	return overcommit, nil
}

func (m *vmMutator) getAdditionalGuestMemoryOverheadRatioConfig() (*settings.AdditionalGuestMemoryOverheadRatioConfig, error) {
	s, err := m.setting.Get(settings.AdditionalGuestMemoryOverheadRatioName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	value := s.Value
	if value == "" {
		value = s.Default
	}
	return settings.NewAdditionalGuestMemoryOverheadRatioConfig(value)
}

func (m *vmMutator) patchAffinity(vm *kubevirtv1.VirtualMachine, patchOps types.PatchOps) (types.PatchOps, error) {
	if vm == nil || vm.Spec.Template == nil {
		return patchOps, nil
	}

	affinity := makeAffinityFromVMTemplate(vm.Spec.Template)
	requiredNodeSelector := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	preferredNodeSelector := affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution

	newVMNetworks := vm.Spec.Template.Spec.Networks
	logrus.Debugf("newNetworks: %+v", newVMNetworks)

	if err := m.addNodeSelectorTerms(vm, requiredNodeSelector, newVMNetworks); err != nil {
		return patchOps, err
	}

	// The .spec.affinity could not be like `{nodeAffinity:requireDuringSchedulingIgnoreDuringExecution:[]}` if there is not any rules.
	if len(requiredNodeSelector.NodeSelectorTerms) == 0 {
		if len(preferredNodeSelector) == 0 {
			affinity.NodeAffinity = nil
		} else {
			affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = nil
		}
	}

	bytes, err := json.Marshal(affinity)
	if err != nil {
		return patchOps, err
	}
	return append(patchOps, fmt.Sprintf(`{"op":"replace","path":"/spec/template/spec/affinity","value":%s}`, string(bytes))), nil
}

func (m *vmMutator) getNodeSelectorRequirementFromNetwork(defaultNamespace string, network kubevirtv1.Network) (*v1.NodeSelectorRequirement, error) {
	if network.Multus == nil || network.Multus.NetworkName == "" {
		return nil, nil
	}

	words := strings.Split(network.Multus.NetworkName, "/")
	var namespace, name string
	switch len(words) {
	case 1:
		namespace, name = defaultNamespace, words[0]
	case 2:
		namespace, name = words[0], words[1]
	default:
		return nil, fmt.Errorf("invalid network name %s", network.Multus.NetworkName)
	}

	nad, err := m.nad.Get(namespace, name)
	if err != nil {
		return nil, err
	}
	clusterNetwork, ok := nad.Labels[keyClusterNetwork]
	if !ok {
		return nil, nil
	}

	return &v1.NodeSelectorRequirement{
		Key:      fmt.Sprintf("%s/%s", networkGroup, clusterNetwork),
		Operator: v1.NodeSelectorOpIn,
		Values:   []string{"true"},
	}, nil
}

func isContainTargetNodeSelectorRequirement(term v1.NodeSelectorTerm, target v1.NodeSelectorRequirement) (int, bool) {
	for i, expression := range term.MatchExpressions {
		if expression.String() == target.String() {
			return i, true
		}
	}

	return -1, false
}

func makeAffinityFromVMTemplate(template *kubevirtv1.VirtualMachineInstanceTemplateSpec) *v1.Affinity {
	affinity := template.Spec.Affinity
	if affinity == nil {
		affinity = &v1.Affinity{}
	}
	if affinity.NodeAffinity == nil {
		affinity.NodeAffinity = &v1.NodeAffinity{}
	}
	if affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &v1.NodeSelector{}
	}

	// clear node selector terms whose key contains the prefix "network.harvesterhci.io" or equals "cpumanager"
	nodeSelectorTerms := make([]v1.NodeSelectorTerm, 0, len(affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms))
	for _, term := range affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		expressions := make([]v1.NodeSelectorRequirement, 0, len(term.MatchExpressions))
		for _, expression := range term.MatchExpressions {
			if !strings.HasPrefix(expression.Key, networkGroup) && (expression.Key != kubevirtv1.CPUManager) {
				expressions = append(expressions, expression)
			}
		}
		if len(expressions) != 0 {
			term.MatchExpressions = expressions
			nodeSelectorTerms = append(nodeSelectorTerms, term)
		}
	}
	affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = nodeSelectorTerms

	return affinity
}

func (m *vmMutator) addNodeSelectorTerms(vm *kubevirtv1.VirtualMachine, nodeSelector *v1.NodeSelector, incrementalNetworks []kubevirtv1.Network) error {
	for _, network := range incrementalNetworks {
		nodeSelectorRequirement, err := m.getNodeSelectorRequirementFromNetwork(vm.Namespace, network)
		if err != nil {
			return err
		}
		if nodeSelectorRequirement == nil {
			continue
		}
		// Since the terms of node selector are ANDed and the requirements of every term are ORed, we have to add
		// cluster network requirements to all terms if they are existing.
		for i, term := range nodeSelector.NodeSelectorTerms {
			if _, ok := isContainTargetNodeSelectorRequirement(term, *nodeSelectorRequirement); !ok {
				term.MatchExpressions = append(term.MatchExpressions, *nodeSelectorRequirement)
				nodeSelector.NodeSelectorTerms[i] = term
			}
		}
		// If there is no term, initialize one with the cluster network requirement to prove that the cluster network
		// requirement is added
		if len(nodeSelector.NodeSelectorTerms) == 0 {
			nodeSelector.NodeSelectorTerms = []v1.NodeSelectorTerm{{
				MatchExpressions: []v1.NodeSelectorRequirement{*nodeSelectorRequirement},
			}}
		}
	}

	if isDedicatedCPU(vm) {
		requirement := v1.NodeSelectorRequirement{
			Key:      kubevirtv1.CPUManager,
			Operator: v1.NodeSelectorOpIn,
			Values:   []string{"true"},
		}

		if len(nodeSelector.NodeSelectorTerms) == 0 {
			nodeSelector.NodeSelectorTerms = []v1.NodeSelectorTerm{{
				MatchExpressions: []v1.NodeSelectorRequirement{requirement},
			}}
		} else {
			for i, term := range nodeSelector.NodeSelectorTerms {
				if _, ok := isContainTargetNodeSelectorRequirement(term, requirement); !ok {
					term.MatchExpressions = append(term.MatchExpressions, requirement)
					nodeSelector.NodeSelectorTerms[i] = term
				}
			}
		}
	}

	return nil
}

func (m *vmMutator) patchTerminationGracePeriodSeconds(vm *kubevirtv1.VirtualMachine, patchOps types.PatchOps) (types.PatchOps, error) {
	if vm == nil || vm.Spec.Template == nil {
		return patchOps, nil
	}

	if vm.Spec.Template.Spec.TerminationGracePeriodSeconds != nil {
		return patchOps, nil
	}

	s, err := m.setting.Get(settings.DefaultVMTerminationGracePeriodSecondsSettingName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return patchOps, nil
		}
		return patchOps, err
	}
	value := s.Default
	if s.Value != "" {
		value = s.Value
	}

	patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/terminationGracePeriodSeconds", "value": %s}`, value))
	return patchOps, nil
}

func hostDevicesPresent(vm *kubevirtv1.VirtualMachine) bool {
	if len(vm.Spec.Template.Spec.Domain.Devices.HostDevices) > 0 {
		return true
	}

	if len(vm.Spec.Template.Spec.Domain.Devices.GPUs) > 0 {
		return true
	}

	return false
}

func hostDevicesOvercommitNeeded(oldVM, newVM *kubevirtv1.VirtualMachine) bool {
	// if host devices or GPUs are added or removed then resource update is needed
	if len(newVM.Spec.Template.Spec.Domain.Devices.HostDevices) != len(oldVM.Spec.Template.Spec.Domain.Devices.HostDevices) || len(newVM.Spec.Template.Spec.Domain.Devices.GPUs) != len(oldVM.Spec.Template.Spec.Domain.Devices.GPUs) {
		return true
	}

	// during upgrade path VMs with host devices are turned off. Post upgrade the memory needs to be patched
	// to ensure devices with hostDevices/GPUs can be booted
	if len(newVM.Spec.Template.Spec.Domain.Devices.HostDevices) > 0 || len(newVM.Spec.Template.Spec.Domain.Devices.GPUs) > 0 {
		if newVM.Spec.Template.Spec.Domain.Memory != nil && (newVM.Spec.Template.Spec.Domain.Memory.Guest.Value() != newVM.Spec.Template.Spec.Domain.Resources.Requests.Memory().Value()) {
			return true
		}
	}
	return false
}

func isDedicatedCPU(vm *kubevirtv1.VirtualMachine) bool {
	return vm.Spec.Template.Spec.Domain.CPU != nil && vm.Spec.Template.Spec.Domain.CPU.DedicatedCPUPlacement
}
