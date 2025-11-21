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
	Range          string   `json:"range,omitempty"`
	Exclude        []string `json:"exclude,omitempty"`
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
	Type    string   `json:"type"`
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
	bridgeConfig.IPAM.Range = config.Range

	if config.Vlan == 0 {
		config.Vlan = DefaultPVID
	}
	bridgeConfig.Vlan = int(config.Vlan)

	if len(config.Exclude) > 0 {
		bridgeConfig.IPAM.Exclude = config.Exclude
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
