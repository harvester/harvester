package util

import (
	"fmt"
	"net"
)

// maxSubnetHostBits is the maximum number of host bits allowed in a subnet.
// It caps enumeration at 2^16 = 65536 addresses, preventing a large CIDR
const maxSubnetHostBits = 16

// subnetExceedsLimit reports whether the subnet contains more addresses than
// this package is willing to enumerate (more than 2^maxSubnetHostBits).
func subnetExceedsLimit(network *net.IPNet) bool {
	ones, bits := network.Mask.Size()
	return bits-ones > maxSubnetHostBits
}

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

func GetUsableIPAddressesCount(includeRange string, excludeRange []string) (int, error) {
	usableIPAddrMap, err := GetUsableIPAddresses(includeRange, excludeRange)
	if err != nil {
		return 0, err
	}
	return len(usableIPAddrMap), nil
}

// GetUsableIPAddressesCountDualStack returns the total usable IP count across
// an IPv4 range and an IPv6 range. Either range may be empty.
func GetUsableIPAddressesCountDualStack(v4Range string, v6Range string, v4Exclude []string, v6Exclude []string) (int, error) {
	total := 0
	if v4Range != "" {
		count, err := GetUsableIPAddressesCount(v4Range, v4Exclude)
		if err != nil {
			return 0, err
		}
		total += count
	}
	if v6Range != "" {
		count, err := GetUsableIPAddressesCount(v6Range, v6Exclude)
		if err != nil {
			return 0, err
		}
		total += count
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

		if subnetExceedsLimit(network) {
			return nil, fmt.Errorf("subnet %s is too large: at most %d addresses are supported", ipNetSubnet, 1<<maxSubnetHostBits)
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
