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

	patchOps, err = m.patchAffinity(vm, patchOps)
	if err != nil {
		return nil, err
	}

	return patchOps, nil
}

func (m *vmMutator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	newVM := newObj.(*kubevirtv1.VirtualMachine)
	oldVM := oldObj.(*kubevirtv1.VirtualMachine)

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
		newRequest := cpu.MilliValue() * int64(100) / int64(overcommit.CPU)
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

func (m *vmMutator) patchAffinity(vm *kubevirtv1.VirtualMachine, patchOps types.PatchOps) (types.PatchOps, error) {
	if vm == nil || vm.Spec.Template == nil {
		return patchOps, nil
	}

	affinity := makeAffinityFromVMTemplate(vm.Spec.Template)
	nodeSelector := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution

	newVMNetworks := vm.Spec.Template.Spec.Networks
	logrus.Debugf("newNetworks: %+v", newVMNetworks)

	if err := m.addNodeSelectorTerms(vm.Namespace, nodeSelector, newVMNetworks); err != nil {
		return patchOps, err
	}

	// The .spec.affinity could not be like `{nodeAffinity:requireDuringSchedulingIgnoreDuringExecution:[]}` if there is not any rules.
	if len(nodeSelector.NodeSelectorTerms) == 0 {
		return append(patchOps, fmt.Sprintf(`{"op":"replace","path":"/spec/template/spec/affinity","value":{}}`)), nil
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

	// clear node selector terms whose key contains the prefix "network.harvesterhci.io"
	nodeSelectorTerms := make([]v1.NodeSelectorTerm, 0, len(affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms))
	for _, term := range affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		isNetworkAffinity := false
		for _, expression := range term.MatchExpressions {
			if strings.Contains(expression.String(), networkGroup) {
				isNetworkAffinity = true
			}
		}
		if !isNetworkAffinity {
			nodeSelectorTerms = append(nodeSelectorTerms, term)
		}
	}
	affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = nodeSelectorTerms

	return affinity
}

func (m *vmMutator) addNodeSelectorTerms(defaultNamespace string, nodeSelector *v1.NodeSelector, incrementalNetworks []kubevirtv1.Network) error {
	for _, network := range incrementalNetworks {
		nodeSelectorRequirement, err := m.getNodeSelectorRequirementFromNetwork(defaultNamespace, network)
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

	return nil
}
