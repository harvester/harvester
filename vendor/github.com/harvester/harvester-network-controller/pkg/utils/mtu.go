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

func GetMTUFromAnnotation(annotation string) (int, error) {
	MTU, err := strconv.Atoi(annotation)
	if err != nil {
		return 0, fmt.Errorf("annotation %v value is not an integer, error %w", annotation, err)
	}
	if !IsValidMTU(MTU) {
		return 0, fmt.Errorf("annotation %v value is not in range [0, %v..%v]", annotation, MinMTU, MaxMTU)
	}
	return MTU, nil
}

// caller handlers the return value of 0 which is normally treated as the default MTU
func GetMTUFromVlanConfig(vc *networkv1.VlanConfig) int {
	if vc == nil || vc.Spec.Uplink.LinkAttrs == nil {
		return 0
	}

	return vc.Spec.Uplink.LinkAttrs.MTU
}
