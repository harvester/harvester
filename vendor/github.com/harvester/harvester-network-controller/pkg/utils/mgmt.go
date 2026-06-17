package utils

const ManagementClusterNetworkName = "mgmt"

func IsManagementClusterNetwork(cnName string) bool {
	return cnName == ManagementClusterNetworkName
}
