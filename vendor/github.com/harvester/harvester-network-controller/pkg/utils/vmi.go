package utils

import (
	"fmt"

	mapset "github.com/deckarep/golang-set/v2"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

type VmiGetter struct {
	VmiCache ctlkubevirtv1.VirtualMachineInstanceCache
}

func NewVmiGetter(vmiCache ctlkubevirtv1.VirtualMachineInstanceCache) *VmiGetter {
	return &VmiGetter{VmiCache: vmiCache}
}

// WhoUseNad requires adding network indexer to the vmi cache before invoking it
// when filterFlag is true, it ensures the return vmis are on the given nodesFilter
func (v *VmiGetter) WhoUseNad(nad *nadv1.NetworkAttachmentDefinition, filterFlag bool, nodesFilter mapset.Set[string]) ([]*kubevirtv1.VirtualMachineInstance, error) {
	// multus network name can be <networkName> or <namespace>/<networkName>
	// ref: https://github.com/kubevirt/client-go/blob/148fa0d1c7e83b7a56606a7ca92394ba6768c9ac/api/v1/schema.go#L1436-L1439

	// note: this is an inclusive filter, the target vmi must be in given nodes
	// when the nodes are empty, return nil directly
	if filterFlag && (nodesFilter == nil || nodesFilter.Cardinality() == 0) {
		return nil, nil
	}

	networkName := fmt.Sprintf("%s/%s", nad.Namespace, nad.Name)
	vmis, err := v.VmiCache.GetByIndex(VMByNetworkIndex, networkName)
	if err != nil {
		return nil, fmt.Errorf("failed to get vmi via VMByNetworkIndex %v, error %w", networkName, err)
	}

	vmisTmp, err := v.VmiCache.GetByIndex(VMByNetworkIndex, nad.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get vmi via VMByNetworkIndex %v, error %w", nad.Name, err)
	}
	for _, vmi := range vmisTmp {
		if vmi.Namespace != nad.Namespace {
			continue
		}
		vmis = append(vmis, vmi)
	}

	// don't filter, then return all above
	if !filterFlag {
		return vmis, nil
	}

	// filter out vmis whose status.nodeName is not in the nodes map, namely only return those vmis on given nodes
	afterFilter := make([]*kubevirtv1.VirtualMachineInstance, 0, len(vmis))
	for _, vmi := range vmis {
		if nodesFilter.Contains(vmi.Status.NodeName) {
			afterFilter = append(afterFilter, vmi)
		}
	}

	return afterFilter, nil
}

// Get the vmi lists who uses a group of nads,
// note: duplicated vmis may exist when they attache to mutli nads
func (v *VmiGetter) WhoUseNads(nads []*nadv1.NetworkAttachmentDefinition, filterFlag bool, nodesFilter mapset.Set[string]) ([]*kubevirtv1.VirtualMachineInstance, error) {
	vmis := make([]*kubevirtv1.VirtualMachineInstance, 0)
	for _, nad := range nads {
		// get all vmis on this nad
		vmisTemp, err := v.WhoUseNad(nad, filterFlag, nodesFilter)
		if err != nil {
			return nil, err
		}
		vmis = append(vmis, vmisTemp...)
	}
	return vmis, nil
}

// Get the vmi name list who uses the nad
func (v *VmiGetter) VmiNamesWhoUseNad(nad *nadv1.NetworkAttachmentDefinition, filterFlag bool, nodesFilter mapset.Set[string]) ([]string, error) {
	vmis, err := v.WhoUseNad(nad, filterFlag, nodesFilter)
	if err != nil {
		return nil, err
	}

	return generateVmiNameList(vmis), nil
}

// Get the vmi name list who uses a group of nads,
// note: duplicated names are removed
func (v *VmiGetter) VmiNamesWhoUseNads(nads []*nadv1.NetworkAttachmentDefinition, filterFlag bool, nodesFilter mapset.Set[string]) ([]string, error) {
	vmis, err := v.WhoUseNads(nads, filterFlag, nodesFilter)
	if err != nil {
		return nil, err
	}

	cnt := len(vmis)

	if cnt == 0 {
		return nil, nil
	}

	if cnt == 1 {
		return generateVmiNameList(vmis), nil
	}

	// use mapset to remove duplicated names
	return mapset.NewSet[string](generateVmiNameList(vmis)...).ToSlice(), nil
}

func generateVmiNameList(vmis []*kubevirtv1.VirtualMachineInstance) []string {
	if len(vmis) == 0 {
		return nil
	}
	vmiStrList := make([]string, len(vmis))
	for i, vmi := range vmis {
		vmiStrList[i] = vmi.Namespace + "/" + vmi.Name
	}
	return vmiStrList
}
