package utils

import (
	"fmt"
	"strings"
)

const (
	BridgeSuffix       = "-br"
	BondSuffix         = "-bo"
	DefaultValueMiimon = 100

	LenOfBridgeSuffix = 3 // length of BridgeSuffix
	LenOfBondSuffix   = 3 // length of BondSuffix

	MaxDeviceNameLen = 15

	VlanSubInterfaceSpliter = "."

	// format: e.g. mgmt-br.2025
	ManagementClusterNetworkVlanDevicePrefix = ManagementClusterNetworkName + BridgeSuffix + VlanSubInterfaceSpliter

	// format: e.g. mgmt-br
	ManagementClusterNetworkDevicePrefix = ManagementClusterNetworkName + BridgeSuffix
)

func HasMgmtClusterNetworkVlanDevicePrefix(link string) bool {
	return strings.HasPrefix(link, ManagementClusterNetworkVlanDevicePrefix)
}

func HasMgmtClusterNetworkBridgePrefix(link string) bool {
	return strings.HasPrefix(link, ManagementClusterNetworkDevicePrefix)
}

// e.g. cn2-br.2025 is a valid vlan sub interface, and the device prefix is `cn2-br.`
func GetClusterNetworkDevicePrefix(cnName string) string {
	return fmt.Sprintf("%s%s%s", cnName, BridgeSuffix, VlanSubInterfaceSpliter)
}

// e.g. cn2-br.2025
func GetClusterNetworkBrVlanDevice(cnBrName string, vlanId uint16) string {
	return fmt.Sprintf("%s%s%s", cnBrName, VlanSubInterfaceSpliter, fmt.Sprint(vlanId))
}

// e.g. cn2-br.2025
func GetClusterNetworkVlanDevice(cnName string, vlanId uint16) string {
	return fmt.Sprintf("%s%s", GetClusterNetworkDevicePrefix(cnName), fmt.Sprint(vlanId))
}

func HasClusterNetworkDevicePrefix(link, prefix string) bool {
	return strings.HasPrefix(link, prefix)
}

func IsBridgeNameValid(brName string) (bool, error) {
	lenOfBrName := len(brName)
	if lenOfBrName <= LenOfBridgeSuffix {
		return false, fmt.Errorf("the length of bridge name %v is less than %v", brName, LenOfBridgeSuffix)
	}
	if lenOfBrName > MaxDeviceNameLen {
		return false, fmt.Errorf("the length of the bridge name %v can't be more than %v", brName, MaxDeviceNameLen)
	}
	if !strings.HasSuffix(brName, BridgeSuffix) {
		return false, fmt.Errorf("the bridge name %v does not include suffix %v", brName, BridgeSuffix)
	}
	return true, nil
}

func GetClusterNetworkFromBridgeName(brName string) (string, error) {
	if _, err := IsBridgeNameValid(brName); err != nil {
		return "", err
	}
	return brName[:len(brName)-LenOfBridgeSuffix], nil
}

// check if the bridge name is valid
func CheckBridgeName(brName string) error {
	_, err := IsBridgeNameValid(brName)
	return err
}

// get the bridge name exclude the suffix
func GetBridgeNamePrefix(brName string) (string, error) {
	return GetClusterNetworkFromBridgeName(brName)
}

func generateName(prefix, suffix string, lenOfSufix int) string {
	maxPrefixLen := MaxDeviceNameLen - lenOfSufix
	if len(prefix) > maxPrefixLen {
		return prefix[:maxPrefixLen] + suffix
	}
	return prefix + suffix
}

func GenerateBridgeName(prefix string) string {
	return generateName(prefix, BridgeSuffix, LenOfBridgeSuffix)
}

func GenerateBondName(prefix string) string {
	return generateName(prefix, BondSuffix, LenOfBondSuffix)
}

func IsHostNetworkIntfNameValid(cn string, vlanid uint16) error {
	vlanIntfName := GetClusterNetworkVlanDevice(cn, vlanid)

	if len(vlanIntfName) > MaxDeviceNameLen {
		return fmt.Errorf("host network interface name %v length %d is more than %d", vlanIntfName, len(vlanIntfName), MaxDeviceNameLen)
	}

	return nil
}
