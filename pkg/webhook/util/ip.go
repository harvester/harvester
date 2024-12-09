package util

import (
	"net"
)

// incrementIP increments the IP address by 1
func incrementIP(ip net.IP) net.IP {
	// Convert the IP to a byte slice and increment the last byte
	ip = ip.To4()
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
	return ip
}

func GetUsableIPAddressesCount(includeRange string, excludeRange []string) (count int, err error) {
	var includeRangeList []string
	includeRangeList = append(includeRangeList, includeRange)

	includeIPAddrList, err := getIPAddressesFromSubnet(includeRangeList, true)
	if err != nil {
		return count, err
	}

	excludeIPAddrList, err := getIPAddressesFromSubnet(excludeRange, false)
	if err != nil {
		return count, err
	}

	for includeIP := range includeIPAddrList {
		if _, exists := excludeIPAddrList[includeIP]; !exists {
			count++
		}
	}

	return count, nil
}

func getIPAddressesFromSubnet(ipNetSubnets []string, include bool) (ipAddrList map[string]struct{}, err error) {
	ipAddrList = make(map[string]struct{})

	for _, ipNetSubnet := range ipNetSubnets {
		ip, network, err := net.ParseCIDR(ipNetSubnet)
		if err != nil {
			return ipAddrList, err
		}

		// Get broadcast address (last address in the subnet)
		broadcast := getBroadcastAddress(network)

		// Iterate through all the IP addresses in the subnet
		for ; network.Contains(ip); incrementIP(ip) {
			if include && (ip.Equal(network.IP) || ip.Equal(broadcast)) {
				continue
			}
			ipAddrList[ip.String()] = struct{}{}
		}
	}

	return ipAddrList, nil
}

// getBroadcastAddress calculates the broadcast address of a subnet
func getBroadcastAddress(ipNet *net.IPNet) net.IP {
	// Use the mask to calculate the broadcast address
	ip := ipNet.IP.To4()
	mask := ipNet.Mask
	broadcast := make(net.IP, len(ip))
	for i := 0; i < len(ip); i++ {
		broadcast[i] = ip[i] | (^mask[i])
	}
	return broadcast
}
