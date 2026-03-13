package ippoolusage

import (
	"fmt"
	"net"
	"reflect"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

type Handler struct {
	ipPoolUsages ctlharvesterv1.IPPoolUsageClient
}

func (h *Handler) OnChange(_ string, pool *harvesterv1beta1.IPPoolUsage) (*harvesterv1beta1.IPPoolUsage, error) {
	if pool == nil || pool.DeletionTimestamp != nil {
		return pool, nil
	}

	reservedIPs, err := reservedIPsForPool(pool)
	if err != nil {
		return pool, err
	}
	if reflect.DeepEqual(pool.Status.ReservedIPs, reservedIPs) {
		return pool, nil
	}

	poolCopy := pool.DeepCopy()
	poolCopy.Status.ReservedIPs = reservedIPs
	return h.ipPoolUsages.UpdateStatus(poolCopy)
}

func reservedIPsForPool(pool *harvesterv1beta1.IPPoolUsage) ([]string, error) {
	_, networkCIDR, err := net.ParseCIDR(pool.Spec.CIDR)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pool CIDR %q: %w", pool.Spec.CIDR, err)
	}

	startIP, endIP, err := allocatableIPRange(networkCIDR)
	if err != nil {
		return nil, err
	}

	reservedCount := effectiveReservedIPCount(pool.Spec.ReservedIPCount)
	if reservedCount == 0 {
		return []string{}, nil
	}

	reservedIPs := make([]string, 0, reservedCount)
	for ip := append(net.IP(nil), startIP...); compareIPs(ip, endIP) <= 0 && len(reservedIPs) < reservedCount; ip = nextIP(ip) {
		reservedIPs = append(reservedIPs, ip.String())
	}

	return reservedIPs, nil
}

func effectiveReservedIPCount(count int) int {
	if count == 0 {
		return util.IPPoolUsageDefaultReservedIPCount
	}
	if count < 0 {
		return 0
	}
	return count
}

func allocatableIPRange(cidr *net.IPNet) (net.IP, net.IP, error) {
	networkIP := cidr.IP.Mask(cidr.Mask)
	if networkIP == nil {
		return nil, nil, fmt.Errorf("failed to mask CIDR %s", cidr.String())
	}

	maskSize, totalBits := cidr.Mask.Size()
	startIP := append(net.IP(nil), networkIP...)
	endIP := lastIPInSubnet(cidr)

	if startIP.To4() != nil && totalBits == 32 && maskSize < 31 {
		startIP = nextIP(startIP)
		endIP = previousIP(endIP)
	}
	if compareIPs(startIP, endIP) > 0 {
		return nil, nil, fmt.Errorf("CIDR %s has no allocatable IPs", cidr.String())
	}

	return startIP, endIP, nil
}

func lastIPInSubnet(cidr *net.IPNet) net.IP {
	endIP := append(net.IP(nil), cidr.IP.Mask(cidr.Mask)...)
	for i := range endIP {
		endIP[i] |= ^cidr.Mask[i]
	}
	return endIP
}

func nextIP(ip net.IP) net.IP {
	next := append(net.IP(nil), ip...)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	return next
}

func previousIP(ip net.IP) net.IP {
	prev := append(net.IP(nil), ip...)
	for i := len(prev) - 1; i >= 0; i-- {
		if prev[i] == 0 {
			prev[i] = 255
			continue
		}
		prev[i]--
		break
	}
	return prev
}

func compareIPs(left, right net.IP) int {
	left16 := left.To16()
	right16 := right.To16()
	for i := 0; i < len(left16) && i < len(right16); i++ {
		if left16[i] < right16[i] {
			return -1
		}
		if left16[i] > right16[i] {
			return 1
		}
	}
	return 0
}
