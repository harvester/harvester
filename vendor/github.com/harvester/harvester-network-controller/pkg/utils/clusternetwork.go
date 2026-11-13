package utils

import (
	"fmt"
	"strings"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
)

const (
	MaxClusterNetworkNameLen = MaxDeviceNameLen - LenOfBridgeSuffix
)

func IsClusterNetworkNameValid(nm string) (bool, error) {
	if len(nm) > MaxClusterNetworkNameLen {
		return false, fmt.Errorf("the length of the clusterNetwork name %v is more than %d", nm, MaxClusterNetworkNameLen)
	}
	return true, nil
}

func AreClusterNetworkVlanAnnotationsUnchanged(cn *networkv1.ClusterNetwork, vidstr, vidhash string) bool {
	if cn == nil || cn.Annotations == nil {
		return false
	}
	return cn.Annotations[KeyVlanIDSetStr] == vidstr && cn.Annotations[KeyVlanIDSetStrHash] == vidhash
}

func SetClusterNetworkVlanAnnotations(cn *networkv1.ClusterNetwork, vidstr, vidhash string) {
	if cn == nil {
		return
	}
	if cn.Annotations == nil {
		cn.Annotations = make(map[string]string)
	}
	cn.Annotations[KeyVlanIDSetStr] = vidstr
	cn.Annotations[KeyVlanIDSetStrHash] = vidhash
}

func GetClusterNetworkName(iface string) (string, error) {
	cnName, _, found := strings.Cut(iface, BridgeSuffix)
	if !found {
		return "", fmt.Errorf("invalid interface format: %s", iface)
	}
	return cnName, nil
}
