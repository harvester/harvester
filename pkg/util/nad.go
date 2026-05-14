package util

import (
	"encoding/json"
	"fmt"
	"strings"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

const (
	NetworkGroup      = "network.harvesterhci.io"
	KeyClusterNetwork = NetworkGroup + "/clusternetwork"
	OverlayNetwork    = "OverlayNetwork"
	KeyNetworkType    = NetworkGroup + "/type"
	UntaggedVlan      = 0
	DefaultVlan       = 1
)

type NetConf struct {
	cnitypes.PluginConf
	Vlan      int          `json:"vlan"`
	VlanTrunk []*VlanTrunk `json:"vlanTrunk,omitempty"`
}

type VlanTrunk struct {
	MinID *int `json:"minID,omitempty"`
	MaxID *int `json:"maxID,omitempty"`
	ID    *int `json:"id,omitempty"`
}

func DecodeNadConfigToNetConf(nad *cniv1.NetworkAttachmentDefinition) (*NetConf, error) {
	conf := &NetConf{}
	if nad == nil || nad.Spec.Config == "" {
		return conf, nil
	}

	if err := json.Unmarshal([]byte(nad.Spec.Config), conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal nad %v/%v config %s %w", nad.Namespace, nad.Name, nad.Spec.Config, err)
	}

	return conf, nil
}

func (nc *NetConf) IsBridgeCNI() bool {
	return nc.Type == "bridge"
}

func IsNadCreatedBySystem(nad *cniv1.NetworkAttachmentDefinition) bool {
	return nad.Namespace == HarvesterSystemNamespaceName
}

func (nc *NetConf) HasTrunkVlanID(target uint16) bool {
	for _, vt := range nc.VlanTrunk {
		// #nosec G115
		if vt.ID != nil && uint16(*vt.ID) == target {
			return true
		}
		// #nosec G115
		if vt.MinID != nil && vt.MaxID != nil && target >= uint16(*vt.MinID) && target <= uint16(*vt.MaxID) {
			return true
		}
	}
	return false
}

func (nc *NetConf) IsVMNetworkVlan(vid uint16) bool {
	// #nosec G115
	return uint16(nc.Vlan) == vid
}

func IsStorageNetworkNad(nad *cniv1.NetworkAttachmentDefinition) bool {
	return nad.Annotations[StorageNetworkAnnotation] == "true"
}

func IsNetworkTypeOverlay(nad *cniv1.NetworkAttachmentDefinition) bool {
	return strings.EqualFold(nad.Labels[KeyNetworkType], OverlayNetwork)
}
