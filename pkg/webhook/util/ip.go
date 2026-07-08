package util

import (
	"math"
	"net"
	"net/netip"
)

// incrementIP increments the IP address by 1.
// To16 allocates a new slice, so the original is not mutated.
func incrementIP(ip net.IP) net.IP {
	ip = ip.To16()
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
	return ip
}

func GetUsableIPAddresses(includeRange string, excludeRange []string) (map[string]struct{}, error) {
	includeRangeList := []string{includeRange}

	includeIPAddrMap, err := getIPAddressesFromSubnet(includeRangeList, true)
	if err != nil {
		return includeIPAddrMap, err
	}

	excludeIPAddrMap, err := getIPAddressesFromSubnet(excludeRange, false)
	if err != nil {
		return nil, err
	}

	for includeIP := range includeIPAddrMap {
		if _, exists := excludeIPAddrMap[includeIP]; exists {
			delete(includeIPAddrMap, includeIP)
		}
	}

	return includeIPAddrMap, nil
}

// GetUsableIPAddressesCount returns the number of usable IP addresses in
// includeRange after subtracting addresses covered by excludeRange CIDRs.
// It uses arithmetic rather than enumeration, making it safe for any prefix
// length including large IPv6 subnets up to integer limit.
func GetUsableIPAddressesCount(includeRange string, excludeRange []string) (int, error) {
	netPrefix, err := netip.ParsePrefix(includeRange)
	if err != nil {
		return 0, err
	}
	netPrefix = netPrefix.Masked()

	maxBits := 32
	if netPrefix.Addr().Is6() {
		maxBits = 128
	}
	hostBits := maxBits - netPrefix.Bits()

	// 1<<63 (2^63) overflows int64 so directly return MaxInt
	// which trivially has enough IPs for any cluster
	if hostBits >= 63 {
		return math.MaxInt, nil
	}

	total := int(1 << uint(hostBits))

	if netPrefix.Addr().Is4() {
		// subtract network address and broadcast required by IPv4
		if total >= 2 {
			total -= 2
		} else {
			total = 0
		}
	} else {
		// subtract network address only there is no broadcast address for IPv6
		if total >= 1 {
			total--
		}
	}

	for _, exStr := range excludeRange {
		exPrefix, err := netip.ParsePrefix(exStr)
		if err != nil {
			return 0, err
		}
		exPrefix = exPrefix.Masked()

		// only subtract if the exclude range is fully contained within the include range
		if netPrefix.Bits() <= exPrefix.Bits() && netPrefix.Contains(exPrefix.Addr()) {
			exHostBits := maxBits - exPrefix.Bits()
			total -= int(1 << uint(exHostBits))
		}
	}

	if total < 0 {
		total = 0
	}

	return total, nil
}

func getIPAddressesFromSubnet(ipNetSubnets []string, include bool) (ipAddrList map[string]struct{}, err error) {
	ipAddrList = make(map[string]struct{})

	for _, ipNetSubnet := range ipNetSubnets {
		ip, network, err := net.ParseCIDR(ipNetSubnet)
		if err != nil {
			return ipAddrList, err
		}

		lastAddr := getLastAddress(network)
		isIPv4 := network.IP.To4() != nil

		for ; network.Contains(ip); ip = incrementIP(ip) {
			if include && ip.Equal(network.IP) {
				continue // skip network address for both families
			}
			if include && isIPv4 && ip.Equal(lastAddr) {
				continue // skip broadcast address for IPv4 only
			}
			ipAddrList[ip.String()] = struct{}{}
		}
	}

	return ipAddrList, nil
}

// getLastAddress returns the last address in the subnet (broadcast for IPv4;
// last unicast address for IPv6 — but callers must not exclude it for IPv6).
// net.ParseCIDR guarantees len(ipNet.IP) == len(ipNet.Mask), so no padding is needed.
func getLastAddress(ipNet *net.IPNet) net.IP {
	ip := ipNet.IP
	mask := ipNet.Mask
	last := make(net.IP, len(ip))
	for i := range ip {
		last[i] = ip[i] | (^mask[i])
	}
	return last
}
