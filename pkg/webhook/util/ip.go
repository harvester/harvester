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

func GetUsableIPAddressesCount(includeRange string, excludeRange []string) (int, error) {
	usableIPAddrMap, err := GetUsableIPAddresses(includeRange, excludeRange)
	if err != nil {
		return 0, err
	}
	return len(usableIPAddrMap), nil
}

// GetUsableIPAddressesCountDualStack returns the total usable IP count across
// an IPv4 range and an IPv6 range. Either range may be empty.
// IPv4 uses the existing map-enumeration path (unchanged).
// IPv6 uses arithmetic counting to avoid OOM on large ranges.
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
		count, err := getUsableIPAddressesCountArithmetic(v6Range, v6Exclude)
		if err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

// getUsableIPAddressesCountArithmetic counts usable IPs using prefix arithmetic,
// avoiding enumeration. Safe for large IPv6 ranges.
func getUsableIPAddressesCountArithmetic(includeRange string, excludeRange []string) (int, error) {
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

	// Guard: hostBits >= 63 would overflow int on 64-bit systems.
	// Return MaxInt unless a single exclude covers the entire range.
	if hostBits >= 63 {
		for _, exStr := range excludeRange {
			exPrefix, parseErr := netip.ParsePrefix(exStr)
			if parseErr != nil {
				continue
			}
			if exPrefix.Masked() == netPrefix {
				return 0, nil
			}
		}
		return math.MaxInt, nil
	}

	total := 1 << uint(hostBits)

	// Remove reserved addresses.
	reservedNet := netPrefix.Addr()
	hasBroadcast := netPrefix.Addr().Is4()
	var reservedBroadcast netip.Addr
	if hasBroadcast {
		reservedBroadcast = lastAddrInPrefixNetip(netPrefix)
		total -= 2 // network + broadcast
	} else {
		total-- // network address only; IPv6 has no broadcast
	}

	if total < 0 {
		total = 0
	}

	// Subtract each exclude range that is fully contained within the include range,
	// adjusting for reserved addresses already subtracted above.
	for _, exStr := range excludeRange {
		exPrefix, parseErr := netip.ParsePrefix(exStr)
		if parseErr != nil {
			return 0, parseErr
		}
		exPrefix = exPrefix.Masked()
		if netPrefix.Bits() <= exPrefix.Bits() && netPrefix.Contains(exPrefix.Addr()) {
			exHostBits := maxBits - exPrefix.Bits()
			exCount := 1 << uint(exHostBits)
			// Avoid double-counting the reserved addresses already removed above.
			if exPrefix.Contains(reservedNet) {
				exCount--
			}
			if hasBroadcast && exPrefix.Contains(reservedBroadcast) {
				exCount--
			}
			if exCount > 0 {
				total -= exCount
			}
		}
	}

	if total < 0 {
		total = 0
	}
	return total, nil
}

// lastAddrInPrefixNetip returns the last address in a prefix
// (broadcast for IPv4; last unicast for IPv6).
func lastAddrInPrefixNetip(p netip.Prefix) netip.Addr {
	if p.Addr().Is4() {
		b := p.Addr().As4()
		hostBits := 32 - p.Bits()
		for i := 0; i < hostBits; i++ {
			b[3-i/8] |= 1 << uint(i%8)
		}
		return netip.AddrFrom4(b)
	}
	b := p.Addr().As16()
	hostBits := 128 - p.Bits()
	for i := 0; i < hostBits; i++ {
		b[15-i/8] |= 1 << uint(i%8)
	}
	return netip.AddrFrom16(b)
}

// rightHalfPrefix returns the "right half" of a prefix by setting
// the first free bit to 1 and increasing the prefix length by 1.
// Example: 10.0.0.0/24 → 10.0.0.128/25.
func rightHalfPrefix(p netip.Prefix) netip.Prefix {
	bits := p.Bits()
	if p.Addr().Is4() {
		b := p.Addr().As4()
		b[bits/8] |= 1 << uint(7-(bits%8))
		return netip.PrefixFrom(netip.AddrFrom4(b), bits+1)
	}
	b := p.Addr().As16()
	b[bits/8] |= 1 << uint(7-(bits%8))
	return netip.PrefixFrom(netip.AddrFrom16(b), bits+1)
}

// subtractPrefix removes the part of target that is covered by ex,
// returning the remaining uncovered sub-prefixes.
func subtractPrefix(target netip.Prefix, ex netip.Prefix) []netip.Prefix {
	if !target.Overlaps(ex) {
		return []netip.Prefix{target}
	}
	// ex is at least as general as target: it fully covers target.
	if ex.Bits() <= target.Bits() && ex.Contains(target.Addr()) {
		return nil
	}
	// Bisect target into left and right halves, then recurse.
	left := netip.PrefixFrom(target.Addr(), target.Bits()+1).Masked()
	right := rightHalfPrefix(target)
	result := subtractPrefix(left, ex)
	result = append(result, subtractPrefix(right, ex)...)
	return result
}

// IsCoveredByPrefixes returns true if the union of the given CIDR exclude strings
// fully covers target, meaning no usable addresses remain outside the excludes.
func IsCoveredByPrefixes(target netip.Prefix, excludes []string) bool {
	remaining := []netip.Prefix{target}
	for _, exStr := range excludes {
		exPrefix, err := netip.ParsePrefix(exStr)
		if err != nil {
			continue
		}
		exPrefix = exPrefix.Masked()
		var next []netip.Prefix
		for _, r := range remaining {
			next = append(next, subtractPrefix(r, exPrefix)...)
		}
		remaining = next
		if len(remaining) == 0 {
			return true
		}
	}
	return len(remaining) == 0
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
