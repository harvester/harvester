package utils

import (
	"github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io"
	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
)

const (
	KeyVlanLabel             = network.GroupName + "/vlan-id"
	KeyVlanConfigLabel       = network.GroupName + "/vlanconfig"
	KeyClusterNetworkLabel   = network.GroupName + "/clusternetwork"
	KeyNodeLabel             = network.GroupName + "/node"
	KeyNetworkType           = network.GroupName + "/type"
	KeyLastNetworkType       = network.GroupName + "/last-type"
	KeyNetworkReady          = network.GroupName + "/ready"
	KeyNetworkRoute          = network.GroupName + "/route"
	KeyNetworkRouteSourceVID = network.GroupName + "/route-source-vid" // the source vid of this route
	KeyMTUSourceVlanConfig   = network.GroupName + "/mtu-source-vc"    // the VC which syncs MTU to CN
	KeyUplinkMTU             = network.GroupName + "/uplink-mtu"       // configured MTU on the VC'uplink

	KeyMatchedNodes = network.GroupName + "/matched-nodes"

	KeyVlanIDSetStr     = network.GroupName + "/vlan-id-set-str"      // all vlan ids under current cluster network, format "1,2,3..."
	KeyVlanIDSetStrHash = network.GroupName + "/vlan-id-set-str-hash" // hash value of above string

	KeyVlanDHCPServerIP = network.GroupName + "/vlan-dhcp-server-ip"

	ValueTrue  = "true"
	ValueFalse = "false"

	HarvesterWitnessNodeLabelKey = "node-role.harvesterhci.io/witness"

	HarvesterMgmtClusterNetworkLabeyKey = network.GroupName + "/" + ManagementClusterNetworkName

	// defined in Harvester pkg/util/constants.go
	// to avoid loop references, needs to wait until Harvester move this to its const and then refers to
	StorageNetworkAnnotation         = "storage-network.settings.harvesterhci.io"
	StorageNetworkNetAttachDefPrefix = "storagenetwork-"
	RWXNetworkAnnotation             = "rwx-network.settings.harvesterhci.io"
	RWXNetworkNetAttachDefPrefix     = "rwx-network-"
	ExclusiveVlanStorageNetworkLabel = StorageNetworkAnnotation + "/exclusivevlan"
)

func GetLabelKeyOfClusterNetwork(clusterNetwork string) string {
	return network.GroupName + "/" + clusterNetwork
}

func HasWitnessNodeLabelKey(lbs map[string]string) bool {
	return HasLabelKey(lbs, HarvesterWitnessNodeLabelKey, ValueTrue)
}

func HasMgmtClusterNetworkLabelKey(lbs map[string]string) bool {
	return HasLabelKey(lbs, HarvesterMgmtClusterNetworkLabeyKey, ValueTrue)
}

func SetMgmtClusterNetworkLabelKey(lbs map[string]string) {
	if lbs == nil {
		return
	}
	lbs[HarvesterMgmtClusterNetworkLabeyKey] = ValueTrue
}

func HasLabelKey(lbs map[string]string, key string, value string) bool {
	return lbs[key] == value
}

func IsClusterNetworkLabelValid(vc *networkv1.VlanConfig) (bool, string) {
	label := ""
	if vc.Labels != nil {
		label = vc.Labels[KeyClusterNetworkLabel]
	}

	if label == "" || label != vc.Spec.ClusterNetwork {
		return false, label
	}

	return true, ""
}
