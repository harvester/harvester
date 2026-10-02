package utils

import (
	"github.com/harvester/webhook/pkg/server/admission"
)

const (
	ManagementClusterNetworkName = "mgmt"
	MgmtVlanConfigPrefix         = "mgmt-vlanconfig-"
)

func IsManagementClusterNetwork(cnName string) bool {
	return cnName == ManagementClusterNetworkName
}

func IsUserRequestForMgmtCluster(req *admission.Request, clusterNetwork string) bool {
	if clusterNetwork != ManagementClusterNetworkName {
		return false
	}

	if req != nil && !req.IsFromController() {
		return true
	}

	return false
}

func GetMgmtVlanConfigName(nodeName string) string {
	return MgmtVlanConfigPrefix + nodeName
}
