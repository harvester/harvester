package util

import (
	"fmt"
	"strings"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

// if this nad is a storagenetwork nad
func IsStorageNetworkNad(nad *nadv1.NetworkAttachmentDefinition) bool {
	if nad == nil || nad.Namespace != StorageNetworkNetAttachDefNamespace {
		return false
	}

	// seems Harvester webhook has no protection on this annotation
	if nad.Annotations != nil && nad.Annotations[StorageNetworkAnnotation] == "true" {
		return true
	}

	// check name prefix, if StorageNetworkAnnotation was removed
	if strings.HasPrefix(nad.Name, StorageNetworkNetAttachDefPrefix) {
		return true
	}

	return false
}

// valid format is "vlan1" or "ns2/vlan2"
func GetNadNamespaceNameFromVMNetworkName(nwName string) (namespace, name string, err error) {
	words := strings.Split(nwName, "/")
	switch len(words) {
	case 1:
		return DefaultNamespace, words[0], nil
	case 2:
		return words[0], words[1], nil
	default:
		return "", "", fmt.Errorf("invalid network name %s", nwName)
	}
}
