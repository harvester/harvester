package util

import (
	"net"
	"regexp"
)

var (
	HarvesterNodeRoleLabelPrefix = "node-role.harvesterhci.io/"
	HarvesterWitnessNodeLabelKey = HarvesterNodeRoleLabelPrefix + "witness"
	HarvesterMgmtNodeLabelKey    = HarvesterNodeRoleLabelPrefix + "management"
	HarvesterWorkerNodeLabelKey  = HarvesterNodeRoleLabelPrefix + "worker"

	sizeRegexp = regexp.MustCompile(`^(\d+)(Mi|Gi)$`)
)

func StringSliceContains(sSlice []string, s string) bool {
	for _, target := range sSlice {
		if target == s {
			return true
		}
	}
	return false
}

func DupStrings(src []string) []string {
	if src == nil {
		return nil
	}
	s := make([]string, len(src))
	copy(s, src)
	return s
}

func ByteToGi(b uint64) uint64 {
	return b >> 30
}

func ByteToMi(b uint64) uint64 {
	return b >> 20
}

func GiToByte(gi uint64) uint64 {
	return gi << 30
}

func MiToByte(mi uint64) uint64 {
	return mi << 20
}

func IsMACAddress(s string) (bool, error) {
	_, err := net.ParseMAC(s)
	if err != nil {
		return false, err
	}
	return true, nil
}
