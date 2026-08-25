package network

import (
	"crypto/rand"
	"fmt"
	"net"
)

const (
	BridgeSuffix = "-br"
	CNIVersion   = "0.3.1"
	DefaultPVID  = 1
	DefaultCNI   = "bridge"
	DefaultIPAM  = "whereabouts"
)

type Config struct {
	ClusterNetwork string   `json:"clusterNetwork,omitempty"`
	Vlan           uint16   `json:"vlan,omitempty"`
	ExclusiveVlan  bool     `json:"exclusiveVlan,omitempty"`
	Range          string   `json:"range,omitempty"`
	Exclude        []string `json:"exclude,omitempty"`
	RangeV6        string   `json:"rangeV6,omitempty"`
	ExcludeV6      []string `json:"excludeV6,omitempty"`
}

// Note: this data type should align with https://github.com/containernetworking/cni/blob/main/pkg/types/types.go#L64-L78
// and https://github.com/containernetworking/plugins/blob/main/plugins/main/bridge/bridge.go#L47-L75
type BridgeConfig struct {
	CNIVersion  string     `json:"cniVersion"`
	Type        string     `json:"type"`
	Bridge      string     `json:"bridge"`
	PromiscMode bool       `json:"promiscMode"`
	Vlan        int        `json:"vlan"`
	IPAM        IPAMConfig `json:"ipam"`
}

// Note: this data type should align with https://github.com/k8snetworkplumbingwg/whereabouts/blob/master/pkg/types/types.go#L48-L75
type IPAMConfig struct {
	Type     string               `json:"type"`
	Range    string               `json:"range,omitempty"`    // legacy single-stack
	Exclude  []string             `json:"exclude,omitempty"`  // legacy single-stack
	IPRanges []RangeConfiguration `json:"ipRanges,omitempty"` // dual-stack (Whereabouts v0.9.3+)
}

// RangeConfiguration is one entry in the Whereabouts ipRanges dual-stack list.
// It must be defined locally because the vendored Whereabouts package does not
// export this type.
type RangeConfiguration struct {
	Range   string   `json:"range"`
	Exclude []string `json:"exclude,omitempty"`
}

func CreateBridgeConfig(config Config) BridgeConfig {
	bridgeConfig := BridgeConfig{
		CNIVersion:  CNIVersion,
		Type:        DefaultCNI,
		PromiscMode: true,
		Vlan:        DefaultPVID,
		IPAM: IPAMConfig{
			Type: DefaultIPAM,
		},
	}
	bridgeConfig.Bridge = config.ClusterNetwork + BridgeSuffix

	if config.Vlan == 0 {
		config.Vlan = DefaultPVID
	}
	bridgeConfig.Vlan = int(config.Vlan)

	if config.RangeV6 != "" {
		// Dual-stack: use the Whereabouts ipRanges list format.
		// Both the IPv4 and IPv6 ranges are written; the flat range/exclude
		// fields are left empty so the two formats are never mixed.
		ipv4Entry := RangeConfiguration{Range: config.Range}
		if len(config.Exclude) > 0 {
			ipv4Entry.Exclude = config.Exclude
		}
		ipv6Entry := RangeConfiguration{Range: config.RangeV6}
		if len(config.ExcludeV6) > 0 {
			ipv6Entry.Exclude = config.ExcludeV6
		}
		bridgeConfig.IPAM.IPRanges = []RangeConfiguration{ipv4Entry, ipv6Entry}
	} else {
		// Single-stack: legacy flat range/exclude path.
		bridgeConfig.IPAM.Range = config.Range
		if len(config.Exclude) > 0 {
			bridgeConfig.IPAM.Exclude = config.Exclude
		}
	}

	return bridgeConfig
}

// generates a random Locally Administered Unicast MAC Address.
func GenerateLAAMacAddress() (net.HardwareAddr, error) {
	buf := make([]byte, 6)

	_, err := rand.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("error reading random bytes: %w", err)
	}

	// Set the Local Bit (the 2nd least significant bit) to 1.
	// Binary: 00000010 (Hex: 0x02).
	buf[0] |= 0x02

	// Clear the Multicast Bit (the least significant bit) to 0, ensuring Unicast.
	// Binary: 11111110 (Hex: 0xFE).
	buf[0] &= 0xFE

	return net.HardwareAddr(buf), nil
}
