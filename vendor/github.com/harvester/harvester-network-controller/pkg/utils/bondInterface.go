package utils

import (
	"fmt"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type MgmtBondInterfaceInfo struct {
	MTU        int
	BondMode   string
	SlaveNames []string
}

func GetMgmtBondInfo() (*MgmtBondInterfaceInfo, error) {
	// Find the mgmt-bo bond interface
	mgmtBo, err := netlink.LinkByName(ManagementClusterNetworkBondDevicePrefix)
	if err != nil {
		logrus.Errorf("failed to get %s interface: %v", ManagementClusterNetworkBondDevicePrefix, err)
		return nil, err
	}

	bond, ok := mgmtBo.(*netlink.Bond)
	if !ok {
		logrus.Errorf("failed to read bond mode for %s: %v", ManagementClusterNetworkBondDevicePrefix, err)
		return nil, fmt.Errorf("interface %s is not a bond device (got %T)", ManagementClusterNetworkBondDevicePrefix, mgmtBo)
	}

	// Get all interfaces and find slaves of mgmt-bo
	links, err := netlink.LinkList()
	if err != nil {
		logrus.Errorf("failed to list interfaces: %v", err)
		return nil, err
	}
	var slaveNames []string
	for _, l := range links {
		if l.Attrs().MasterIndex == mgmtBo.Attrs().Index {
			slaveNames = append(slaveNames, l.Attrs().Name)
		}
	}

	mgmtInfo := &MgmtBondInterfaceInfo{
		MTU:        mgmtBo.Attrs().MTU,
		BondMode:   bond.Mode.String(),
		SlaveNames: slaveNames,
	}

	return mgmtInfo, nil
}

func BuildUpdatedVlanConfig(vc *networkv1.VlanConfig, mgmtIntfInfo *MgmtBondInterfaceInfo) bool {
	if vc == nil {
		return false
	}

	changed := false

	if vc.Spec.Uplink.LinkAttrs == nil {
		vc.Spec.Uplink.LinkAttrs = &networkv1.LinkAttrs{TxQLen: -1}
	}
	if vc.Spec.Uplink.BondOptions == nil {
		vc.Spec.Uplink.BondOptions = &networkv1.BondOptions{Miimon: -1}
	}

	if vc.Spec.Uplink.LinkAttrs.MTU != mgmtIntfInfo.MTU {
		vc.Spec.Uplink.LinkAttrs.MTU = mgmtIntfInfo.MTU
		changed = true
	}

	if vc.Spec.Uplink.BondOptions.Mode != networkv1.BondMode(mgmtIntfInfo.BondMode) {
		vc.Spec.Uplink.BondOptions.Mode = networkv1.BondMode(mgmtIntfInfo.BondMode)
		changed = true
	}

	if !compareStringSlices(vc.Spec.Uplink.NICs, mgmtIntfInfo.SlaveNames) {
		vc.Spec.Uplink.NICs = mgmtIntfInfo.SlaveNames
		changed = true
	}

	return changed
}

// Helper to compare two string slices
func compareStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]struct{}, len(a))
	for _, v := range a {
		m[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := m[v]; !ok {
			return false
		}
	}
	return true
}
