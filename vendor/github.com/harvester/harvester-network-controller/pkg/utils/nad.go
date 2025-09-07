package utils

import (
	"encoding/json"
	"fmt"
	"net"

	cniv1 "github.com/containernetworking/cni/pkg/types"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

type Connectivity string

const (
	Connectable   Connectivity = "true"
	Unconnectable Connectivity = "false"
	DHCPFailed    Connectivity = "DHCP failed"
	PingFailed    Connectivity = "ping failed"
)

type Mode string

const (
	Auto   Mode = "auto"
	Manual Mode = "manual"
)

type NetworkType string

const (
	L2VlanNetwork   NetworkType = "L2VlanNetwork"
	UntaggedNetwork NetworkType = "UntaggedNetwork"
)

type NadSelectedNetworks []nadv1.NetworkSelectionElement

type Layer3NetworkConf struct {
	Mode         Mode         `json:"mode,omitempty"`
	CIDR         string       `json:"cidr,omitempty"`
	Gateway      string       `json:"gateway,omitempty"`
	ServerIPAddr string       `json:"serverIPAddr,omitempty"`
	Connectivity Connectivity `json:"connectivity,omitempty"`
	Outdated     bool         `json:"outdated,omitempty"`
}

func NewLayer3NetworkConf(conf string) (*Layer3NetworkConf, error) {
	if conf == "" {
		return &Layer3NetworkConf{}, nil
	}

	networkConf := &Layer3NetworkConf{}

	if err := json.Unmarshal([]byte(conf), networkConf); err != nil {
		return nil, fmt.Errorf("unmarshal %s faield, error: %w", conf, err)
	}

	// validate
	if networkConf.Mode != "" && networkConf.Mode != Auto && networkConf.Mode != Manual {
		return nil, fmt.Errorf("unknown mode %s", networkConf.Mode)
	}

	// validate cidr and gateway when the mode is manual
	if networkConf.Mode == Manual {
		_, ipnet, err := net.ParseCIDR(networkConf.CIDR)
		if err != nil || (ipnet != nil && isMaskZero(ipnet)) {
			return nil, fmt.Errorf("the CIDR %s is invalid", networkConf.CIDR)
		}

		if net.ParseIP(networkConf.Gateway) == nil {
			return nil, fmt.Errorf("the gateway %s is invalid", networkConf.Gateway)
		}
	}

	return networkConf, nil
}

func (c *Layer3NetworkConf) ToString() (string, error) {
	bytes, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func NewNADSelectedNetworks(conf string) (NadSelectedNetworks, error) {
	networks := make([]nadv1.NetworkSelectionElement, 1)
	if err := json.Unmarshal([]byte(conf), &networks); err != nil {
		return nil, err
	}

	return networks, nil
}

func (n NadSelectedNetworks) ToString() (string, error) {
	bytes, err := json.Marshal(n)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
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

func IsVlanNad(nad *nadv1.NetworkAttachmentDefinition) bool {
	if nad == nil || nad.Spec.Config == "" || nad.Labels == nil || nad.Labels[KeyNetworkType] == "" ||
		nad.Labels[KeyClusterNetworkLabel] == "" || nad.Labels[KeyVlanLabel] == "" {
		return false
	}

	return true
}

func isMaskZero(ipnet *net.IPNet) bool {
	for _, b := range ipnet.Mask {
		if b != 0 {
			return false
		}
	}

	return true
}
