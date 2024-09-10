package utils

import "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io"

const (
	KeyNetworkConf         = network.GroupName + "/route"
	KeyVlanLabel           = network.GroupName + "/vlan-id"
	KeyVlanConfigLabel     = network.GroupName + "/vlanconfig"
	KeyClusterNetworkLabel = network.GroupName + "/clusternetwork"
	KeyNodeLabel           = network.GroupName + "/node"
	KeyNetworkType         = network.GroupName + "/type"

	KeyMatchedNodes = network.GroupName + "/matched-nodes"

	ValueTrue = "true"
)
