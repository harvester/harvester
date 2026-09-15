package util

import (
	"fmt"
	"math"
	"math/big"
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
// an IPv4 range and an IPv6 start/end window. Either may be empty.
// IPv4 uses the existing map-enumeration path (unchanged).
// IPv6 is a named, explicit, contiguous window (RangeStart/RangeEnd), so its
// count is exact arithmetic (end - start + 1) rather than CIDR+exclude
// enumeration/bisection - see the discussion on why IPv6 doesn't need the
// IPv4-style exclude-list carving.
func GetUsableIPAddressesCountDualStack(v4Range string, v6Start string, v6End string, v4Exclude []string) (int, error) {
	total := 0
	if v4Range != "" {
		count, err := GetUsableIPAddressesCount(v4Range, v4Exclude)
		if err != nil {
			return 0, err
		}
		total += count
	}
	if v6Start != "" && v6End != "" {
		count, err := ipv6RangeCount(v6Start, v6End)
		if err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

// ipv6RangeCount returns the number of addresses in the inclusive range
// [start, end], computed as 128-bit arithmetic to avoid overflow on large
// windows. Callers are expected to have already validated start <= end.
func ipv6RangeCount(startStr, endStr string) (int, error) {
	start, err := netip.ParseAddr(startStr)
	if err != nil {
		return 0, fmt.Errorf("invalid rangeV6Start: %w", err)
	}
	end, err := netip.ParseAddr(endStr)
	if err != nil {
		return 0, fmt.Errorf("invalid rangeV6End: %w", err)
	}
	if start.Compare(end) > 0 {
		return 0, fmt.Errorf("rangeV6Start %s must not be after rangeV6End %s", startStr, endStr)
	}

	startBytes := start.As16()
	endBytes := end.As16()
	diff := new(big.Int).Sub(new(big.Int).SetBytes(endBytes[:]), new(big.Int).SetBytes(startBytes[:]))
	diff.Add(diff, big.NewInt(1))
	if !diff.IsInt64() || diff.Int64() > math.MaxInt {
		return math.MaxInt, nil
	}
	return int(diff.Int64()), nil
}

// rightHalfPrefix returns the "right half" of a prefix by setting
// the first free bit to 1 and increasing the prefix length by 1.
// Example: 10.0.0.0/24 -> 10.0.0.128/25.
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
		next := make([]netip.Prefix, 0, len(remaining))
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
// last unicast address for IPv6 - but callers must not exclude it for IPv6).
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
