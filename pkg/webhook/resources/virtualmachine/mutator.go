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
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	networkGroup      = "network.harvesterhci.io"
	keyClusterNetwork = networkGroup + "/clusternetwork"
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

func (m *vmMutator) Create(request *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	vm := newObj.(*kubevirtv1.VirtualMachine)

	logrus.Debugf("create VM %s/%s", vm.Namespace, vm.Name)

	patchOps, err := m.patchResourceOvercommit(vm)
	if err != nil {
		return patchOps, err
	}

	patchOps, err = m.patchAffinity(nil, vm, patchOps)
	if err != nil {
		return nil, err
	}

	return patchOps, nil
}

func (m *vmMutator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	newVm := newObj.(*kubevirtv1.VirtualMachine)
	oldVm := oldObj.(*kubevirtv1.VirtualMachine)

	logrus.Debugf("update VM %s/%s", newVm.Namespace, newVm.Name)

	var patchOps types.PatchOps
	var err error

	if needUpdateResourceOvercommit(oldVm, newVm) {
		patchOps, err = m.patchResourceOvercommit(newVm)
	}
	if err != nil {
		return patchOps, err
	}

	needUpdateRunStrategy, err := needUpdateRunStrategy(oldVm, newVm)
	if err != nil {
		return patchOps, err
	}

	if needUpdateRunStrategy {
		patchOps = patchRunStrategy(newVm, patchOps)
	}

	if needUpdateAffinity(oldVm, newVm) {
		patchOps, err = m.patchAffinity(oldVm, newVm, patchOps)
		if err != nil {
			return nil, err
		}
	}

	return patchOps, nil
}

func needUpdateRunStrategy(oldVm, newVm *kubevirtv1.VirtualMachine) (bool, error) {
	// no need to patch the run strategy if user uses the spec.running filed.
	if newVm.Spec.Running != nil {
		return false, nil
	}

	newRunStrategy, err := newVm.RunStrategy()
	if err != nil {
		return false, err
	}

	oldRunStrategy, err := oldVm.RunStrategy()
	if err != nil {
		return false, err
	}

	if oldRunStrategy == kubevirtv1.RunStrategyHalted && newRunStrategy != kubevirtv1.RunStrategyHalted {
		return true, nil
	}
	return false, nil
}

// add workaround for the issue https://github.com/kubevirt/kubevirt/issues/7295
func patchRunStrategy(newVm *kubevirtv1.VirtualMachine, patchOps types.PatchOps) types.PatchOps {
	runStrategy := newVm.Annotations[util.AnnotationRunStrategy]
	if string(runStrategy) == "" {
		runStrategy = string(kubevirtv1.RunStrategyRerunOnFailure)
	}
	patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/runStrategy", "value": "%s"}`, runStrategy))
	return patchOps
}

func needUpdateResourceOvercommit(oldVm, newVm *kubevirtv1.VirtualMachine) bool {
	var newReservedMemory, oldReservedMemory string
	newLimits := newVm.Spec.Template.Spec.Domain.Resources.Limits
	newCpu := newLimits.Cpu()
	newMem := newLimits.Memory()
	if newVm.Annotations != nil {
		newReservedMemory = newVm.Annotations[util.AnnotationReservedMemory]
	}

	oldLimits := oldVm.Spec.Template.Spec.Domain.Resources.Limits
	oldCpu := oldLimits.Cpu()
	oldMem := oldLimits.Memory()
	if oldVm.Annotations != nil {
		oldReservedMemory = oldVm.Annotations[util.AnnotationReservedMemory]
	}

	if !newCpu.IsZero() && (oldCpu.IsZero() || !newCpu.Equal(*oldCpu)) {
		return true
	}
	if !newMem.IsZero() && (oldMem.IsZero() || !newMem.Equal(*oldMem)) {
		return true
	}
	if newReservedMemory != oldReservedMemory {
		return true
	}
	return false
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

	if !cpu.IsZero() {
		newRequest := cpu.MilliValue() * int64(100) / int64(overcommit.Cpu)
		quantity := resource.NewMilliQuantity(newRequest, cpu.Format)
		if requestsMissing {
			requestsToMutate[v1.ResourceCPU] = *quantity
		} else {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/cpu", "value": "%s"}`, quantity))
		}
	}
	if !mem.IsZero() {
		// Truncate to MiB
		newRequest := mem.Value() * int64(100) / int64(overcommit.Memory) / 1048576 * 1048576
		quantity := resource.NewQuantity(newRequest, mem.Format)
		if requestsMissing {
			requestsToMutate[v1.ResourceMemory] = *quantity
		} else {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "%s"}`, quantity))
		}
		// Reserve 100MiB (104857600 Bytes) for QEMU on guest memory
		// Ref: https://github.com/harvester/harvester/issues/1234
		// TODO: handle hugepage memory
		reservedMemory := *resource.NewQuantity(104857600, resource.BinarySI)
		if vm.Annotations != nil && vm.Annotations[util.AnnotationReservedMemory] != "" {
			reservedMemory, err = resource.ParseQuantity(vm.Annotations[util.AnnotationReservedMemory])
			if err != nil {
				return patchOps, err
			}
		}
		guestMemory := *mem
		guestMemory.Sub(reservedMemory)
		if vm.Spec.Template.Spec.Domain.Memory == nil {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"%s"}}`, &guestMemory))
		} else {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/memory/guest", "value": "%s"}`, &guestMemory))
		}
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

func (m *vmMutator) getOvercommit() (*settings.Overcommit, error) {
	s, err := m.setting.Get("overcommit-config")
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

func needUpdateAffinity(oldVm, newVm *kubevirtv1.VirtualMachine) bool {
	if oldVm.Spec.Template == nil || newVm.Spec.Template == nil {
		return false
	}

	if isContainMultusNetwork(newVm.Spec.Template.Spec.Networks) {
		return true
	}
	// if there are networks removed, update the affinity
	reducedNetworks := getMultusNetworkIncrement(newVm.Spec.Template.Spec.Networks, oldVm.Spec.Template.Spec.Networks)
	if len(reducedNetworks) != 0 {
		return true
	}

	oldVmAffinity, newVmAffinity := oldVm.Spec.Template.Spec.Affinity, newVm.Spec.Template.Spec.Affinity
	// the affinity.String() method already checks the nil
	if oldVmAffinity.String() != newVmAffinity.String() {
		return true
	}

	return false
}

func (m *vmMutator) patchAffinity(oldVm, newVm *kubevirtv1.VirtualMachine, patchOps types.PatchOps) (types.PatchOps, error) {
	if newVm == nil || newVm.Spec.Template == nil {
		return patchOps, nil
	}

	affinity := makeAffinityFromVMTemplate(newVm.Spec.Template)
	nodeSelector := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution

	var oldVmNetworks []kubevirtv1.Network
	if oldVm != nil && oldVm.Spec.Template != nil {
		oldVmNetworks = oldVm.Spec.Template.Spec.Networks
	}
	newVmNetworks := newVm.Spec.Template.Spec.Networks
	logrus.Debugf("newNetworks: %+v", newVmNetworks)
	reducedNetworks := getMultusNetworkIncrement(newVmNetworks, oldVmNetworks)
	logrus.Debugf("reducedNetworks: %+v", reducedNetworks)

	isAdded, err := m.addNodeSelectorRequirements(newVm.Namespace, nodeSelector, newVmNetworks)
	if err != nil {
		return patchOps, err
	}
	isReduced, err := m.deleteNodeNetworkRequirements(newVm.Namespace, nodeSelector, reducedNetworks)
	if err != nil {
		return patchOps, err
	}

	if !(isAdded || isReduced) {
		return patchOps, nil
	}

	bytes, err := json.Marshal(affinity)
	if err != nil {
		return patchOps, err
	}
	return append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/affinity", "value": %s}`, string(bytes))), nil
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

	return affinity
}

func getMultusNetworkIncrement(before, after []kubevirtv1.Network) []kubevirtv1.Network {
	increment := make([]kubevirtv1.Network, 0, len(after))

	for _, a := range after {
		if a.Multus == nil {
			continue
		}

		isExisting := false

		for _, b := range before {
			if b.Multus != nil && a.Multus.NetworkName == b.Multus.NetworkName {
				isExisting = true
				break
			}
		}

		if !isExisting {
			increment = append(increment, a)
		}
	}

	return increment
}

func (m *vmMutator) addNodeSelectorRequirements(defaultNamespace string, nodeSelector *v1.NodeSelector, incrementalNetworks []kubevirtv1.Network) (bool, error) {
	isChanged := false

	for _, network := range incrementalNetworks {
		nodeSelectorRequirement, err := m.getNodeSelectorRequirementFromNetwork(defaultNamespace, network)
		if err != nil {
			return false, err
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
				isChanged = true
			}
		}
		// If there is no term, initialize one with the cluster network requirement to prove that the cluster network
		// requirement is added
		if len(nodeSelector.NodeSelectorTerms) == 0 {
			nodeSelector.NodeSelectorTerms = []v1.NodeSelectorTerm{{
				MatchExpressions: []v1.NodeSelectorRequirement{*nodeSelectorRequirement},
			}}
			isChanged = true
		}
	}

	return isChanged, nil
}

func (m *vmMutator) deleteNodeNetworkRequirements(defaultNamespace string, nodeSelector *v1.NodeSelector, reducedNetworks []kubevirtv1.Network) (bool, error) {
	isChanged := false

	for _, network := range reducedNetworks {
		nodeSelectorRequirement, err := m.getNodeSelectorRequirementFromNetwork(defaultNamespace, network)
		if err != nil {
			return false, err
		}
		if nodeSelectorRequirement == nil {
			continue
		}
		for i, term := range nodeSelector.NodeSelectorTerms {
			if index, ok := isContainTargetNodeSelectorRequirement(term, *nodeSelectorRequirement); ok {
				term.MatchExpressions = append(term.MatchExpressions[:index], term.MatchExpressions[index+1:]...)
				nodeSelector.NodeSelectorTerms[i] = term
				isChanged = true
			}
		}
	}

	return isChanged, nil
}

func isContainMultusNetwork(networks []kubevirtv1.Network) bool {
	for _, network := range networks {
		if network.Multus != nil && network.Multus.NetworkName != "" {
			return true
		}
	}

	return false
}
