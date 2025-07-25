package utils

import (
	"fmt"

	mapset "github.com/deckarep/golang-set/v2"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

const (
	VMByNetworkIndex = "vm.harvesterhci.io/vm-by-network"
)

type VMGetter struct {
	vmCache ctlkubevirtv1.VirtualMachineCache
}

func NewVMGetter(vmCache ctlkubevirtv1.VirtualMachineCache) *VMGetter {
	return &VMGetter{vmCache: vmCache}
}

// WhoUseNad requires adding network indexer to the vm cache before invoking it
func (v *VMGetter) WhoUseNad(nad *nadv1.NetworkAttachmentDefinition) ([]*kubevirtv1.VirtualMachine, error) {
	// multus network name can be <networkName> or <namespace>/<networkName>
	// ref: https://github.com/kubevirt/client-go/blob/148fa0d1c7e83b7a56606a7ca92394ba6768c9ac/api/v1/schema.go#L1436-L1439

	networkName := fmt.Sprintf("%s/%s", nad.Namespace, nad.Name)
	vms, err := v.vmCache.GetByIndex(VMByNetworkIndex, networkName)
	if err != nil {
		return nil, fmt.Errorf("failed to get vm via VMByNetworkIndex %v, error %w", networkName, err)
	}

	vmsTmp, err := v.vmCache.GetByIndex(VMByNetworkIndex, nad.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get vm via VMByNetworkIndex %v, error %w", nad.Name, err)
	}
	for _, vm := range vmsTmp {
		if vm.Namespace != nad.Namespace {
			continue
		}
		vms = append(vms, vm)
	}

	return vms, nil
}

// Get the vm lists who uses a group of nads,
// note: duplicated vmis may exist when they attache to mutli nads
func (v *VMGetter) WhoUseNads(nads []*nadv1.NetworkAttachmentDefinition) ([]*kubevirtv1.VirtualMachine, error) {
	vms := make([]*kubevirtv1.VirtualMachine, 0)
	for _, nad := range nads {
		// get all vmis on this nad
		vmsTemp, err := v.WhoUseNad(nad)
		if err != nil {
			return nil, err
		}
		vms = append(vms, vmsTemp...)
	}
	return vms, nil
}

// Get the vm name list who uses the nad
func (v *VMGetter) VMNamesWhoUseNad(nad *nadv1.NetworkAttachmentDefinition) ([]string, error) {
	vms, err := v.WhoUseNad(nad)
	if err != nil {
		return nil, err
	}

	return generateVMNameList(vms), nil
}

// Get the vm name list who uses a group of nads,
// note: duplicated names are removed
func (v *VMGetter) VMNamesWhoUseNads(nads []*nadv1.NetworkAttachmentDefinition) ([]string, error) {
	vms, err := v.WhoUseNads(nads)
	if err != nil {
		return nil, err
	}

	cnt := len(vms)
	if cnt == 0 {
		return nil, nil
	}
	if cnt == 1 {
		return generateVMNameList(vms), nil
	}

	// use mapset to remove duplicated names
	return mapset.NewSet[string](generateVMNameList(vms)...).ToSlice(), nil
}

func generateVMNameList(vms []*kubevirtv1.VirtualMachine) []string {
	if len(vms) == 0 {
		return nil
	}
	vmStrList := make([]string, len(vms))
	for i, vm := range vms {
		vmStrList[i] = vm.Namespace + "/" + vm.Name
	}
	return vmStrList
}
