package utils

import (
	"fmt"
	"strconv"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
)

func IsValidMTU(MTU int) bool {
	return MTU == 0 || (MTU >= MinMTU && MTU <= MaxMTU)
}

func IsDefaultMTU(MTU int) bool {
	return MTU == DefaultMTU
}

func AreEqualMTUs(MTU1, MTU2 int) bool {
	return (MTU1 == MTU2) || (MTU1 == 0 && MTU2 == DefaultMTU) || (MTU1 == DefaultMTU && MTU2 == 0)
}

func GetMTUFromString(s string) (int, error) {
	MTU, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("value %v is not an integer: %w", s, err)
	}
	if !IsValidMTU(MTU) {
		return 0, fmt.Errorf("value %v is not in range [0, %v..%v]", s, MinMTU, MaxMTU)
	}
	return MTU, nil
}

// GetMTUFromVlanConfig returns the MTU value from the given VlanConfig.
// It returns 0 if no proper MTU value is found. Note, the caller handlers
// the return value of 0 which is normally treated as the default MTU.
func GetMTUFromVlanConfig(vc *networkv1.VlanConfig) int {
	if vc == nil || vc.Spec.Uplink.LinkAttrs == nil {
		return 0
	}
	return vc.Spec.Uplink.LinkAttrs.MTU
}

// MTUDefaultTo returns the default MTU if the input MTU is 0, otherwise returns the input MTU.
func MTUDefaultTo(MTU int) int {
	if MTU == 0 {
		return DefaultMTU
	}
	return MTU
}
