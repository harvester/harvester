package network

import (
	"encoding/json"
	"fmt"
	"strings"

	cniv1 "github.com/containernetworking/cni/pkg/types"
	v1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/indexeres"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	fieldConfig     = "spec.config"
	fieldConfigVlan = "spec.config.vlan"
)

func NewValidator(netAttachDefs ctlcniv1.NetworkAttachmentDefinitionCache, vms ctlkubevirtv1.VirtualMachineCache) types.Validator {
	vms.AddIndexer(indexeres.VMByNetworkIndex, indexeres.VMByNetwork)
	return &networkAttachmentDefinitionValidator{
		netAttachDefs: netAttachDefs,
		vms:           vms,
	}
}

type networkAttachmentDefinitionValidator struct {
	types.DefaultValidator

	netAttachDefs ctlcniv1.NetworkAttachmentDefinitionCache
	vms           ctlkubevirtv1.VirtualMachineCache
}

func (v *networkAttachmentDefinitionValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"network-attachment-definitions"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1.SchemeGroupVersion.Group,
		APIVersion: v1.SchemeGroupVersion.Version,
		ObjectType: &v1.NetworkAttachmentDefinition{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Delete,
		},
	}
}

type NetConf struct {
	cniv1.NetConf
	BrName       string `json:"bridge"`
	IsGW         bool   `json:"isGateway"`
	IsDefaultGW  bool   `json:"isDefaultGateway"`
	ForceAddress bool   `json:"forceAddress"`
	IPMasq       bool   `json:"ipMasq"`
	MTU          int    `json:"mtu"`
	HairpinMode  bool   `json:"hairpinMode"`
	PromiscMode  bool   `json:"promiscMode"`
	Vlan         int    `json:"vlan"`
}

func (v *networkAttachmentDefinitionValidator) Create(request *types.Request, newObj runtime.Object) error {
	netAttachDef := newObj.(*v1.NetworkAttachmentDefinition)

	config := netAttachDef.Spec.Config
	if config == "" {
		return werror.NewInvalidError("config is empty", fieldConfig)
	}

	var bridgeConf = &NetConf{}
	err := json.Unmarshal([]byte(config), &bridgeConf)
	if err != nil {
		return fmt.Errorf("failed to decode NAD config value, error: %v", err)
	}

	if bridgeConf.Vlan < 1 || bridgeConf.Vlan > 4094 {
		return werror.NewInvalidError("bridge VLAN ID must >=1 and <=4094", fieldConfigVlan)
	}

	allocated, err := v.getVLAN(netAttachDef.Namespace, bridgeConf.Vlan)
	if err != nil {
		return err
	}
	if *allocated {
		message := fmt.Sprintf("VLAN ID %d is already allocated", bridgeConf.Vlan)
		return werror.NewInvalidError(message, fieldConfigVlan)
	}

	return nil
}

// getVLAN checks if vid is already allocated to any network defs in a namespace
func (v *networkAttachmentDefinitionValidator) getVLAN(namespace string, vid int) (*bool, error) {
	allocated := false
	nads, err := v.netAttachDefs.List(namespace, labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, nad := range nads {
		var bridgeConf = &NetConf{}
		err := json.Unmarshal([]byte(nad.Spec.Config), &bridgeConf)
		if err != nil {
			return nil, err
		}
		if bridgeConf.Vlan == vid {
			allocated = true
			break
		}
	}

	return &allocated, nil
}

func (v *networkAttachmentDefinitionValidator) Delete(request *types.Request, oldObj runtime.Object) error {
	netAttachDef := oldObj.(*v1.NetworkAttachmentDefinition)

	// multus network name can be <networkName> or <namespace>/<networkName>
	// ref: https://github.com/kubevirt/client-go/blob/148fa0d1c7e83b7a56606a7ca92394ba6768c9ac/api/v1/schema.go#L1436-L1439
	networkName := fmt.Sprintf("%s/%s", netAttachDef.Namespace, netAttachDef.Name)
	vms, err := v.vms.GetByIndex(indexeres.VMByNetworkIndex, networkName)
	if err != nil {
		return err
	}
	if vmsTmp, err := v.vms.GetByIndex(indexeres.VMByNetworkIndex, netAttachDef.Name); err != nil {
		return err
	} else {
		vms = append(vms, vmsTmp...)
	}

	if len(vms) > 0 {
		vmNameList := make([]string, 0, len(vms))
		for _, vm := range vms {
			vmNameList = append(vmNameList, vm.Name)
		}
		errorMessage := fmt.Sprintf("network %s is still used by vm(s): %s", networkName, strings.Join(vmNameList, ", "))
		return werror.NewBadRequest(errorMessage)
	}

	return nil
}
