package network

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
