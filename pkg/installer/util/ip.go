package util

import (
	"fmt"
	"net/netip"
	"strings"
)

// ValidateCIDR checks that cidr is a valid CIDR string (or empty).
// At most two comma-separated CIDRs are allowed (dual-stack).
// A single CIDR must be IPv4; the first of two must be IPv4 and the second IPv6
// (IPv4-first order -- the only supported dual-stack variant).
func ValidateCIDR(cidr string) error {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return nil
	}
	parts := strings.Split(cidr, ",")
	if len(parts) > 2 {
		return fmt.Errorf("at most two CIDRs (IPv4,IPv6) are allowed, got %d", len(parts))
	}
	prefixes := make([]netip.Prefix, 0, len(parts))
	for _, p := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(p))
		if err != nil {
			return fmt.Errorf("%q is not a valid CIDR (expected e.g. 10.52.0.0/16 or fd52::/56)", strings.TrimSpace(p))
		}
		prefixes = append(prefixes, prefix)
	}
	switch len(prefixes) {
	case 1:
		if !prefixes[0].Addr().Is4() {
			return fmt.Errorf("a single CIDR must be IPv4 (e.g. 10.52.0.0/16); for dual-stack provide IPv4,IPv6")
		}
	case 2:
		if !prefixes[0].Addr().Is4() || prefixes[1].Addr().Is4() {
			return fmt.Errorf("dual-stack CIDRs must be in IPv4-first order (e.g. 10.52.0.0/16,fd52::/56)")
		}
	}
	return nil
}

// ValidateCIDRConsistency checks that podCIDR and serviceCIDR are compatible:
// both must have the same number of entries (both single-stack or both dual-stack)
// and their ranges must not overlap.
func ValidateCIDRConsistency(podCIDR, serviceCIDR string) error {
	podCIDR = strings.TrimSpace(podCIDR)
	serviceCIDR = strings.TrimSpace(serviceCIDR)
	if podCIDR == "" || serviceCIDR == "" {
		return nil
	}
	podParts := strings.Split(podCIDR, ",")
	svcParts := strings.Split(serviceCIDR, ",")
	if len(podParts) != len(svcParts) {
		if len(podParts) > len(svcParts) {
			return fmt.Errorf("pod CIDR is dual-stack but service CIDR is single-stack; both must use the same number of CIDRs")
		}
		return fmt.Errorf("service CIDR is dual-stack but pod CIDR is single-stack; both must use the same number of CIDRs")
	}
	podPrefixes := make([]netip.Prefix, 0, len(podParts))
	for _, p := range podParts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(p))
		if err != nil {
			return err
		}
		podPrefixes = append(podPrefixes, prefix.Masked())
	}
	svcPrefixes := make([]netip.Prefix, 0, len(svcParts))
	for _, p := range svcParts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(p))
		if err != nil {
			return err
		}
		svcPrefixes = append(svcPrefixes, prefix.Masked())
	}
	for _, pod := range podPrefixes {
		for _, svc := range svcPrefixes {
			if pod.Overlaps(svc) {
				return fmt.Errorf("pod CIDR %s and service CIDR %s must not overlap", pod, svc)
			}
		}
	}
	return nil
}

// ValidateDNSIP checks that ip (comma-separated DNS addresses) are valid and
// each falls within the matching-family service CIDR.
// An empty ip with a non-empty serviceCIDR is allowed (defaults will be used).
func ValidateDNSIP(ip, serviceCIDR string) error {
	ip = strings.TrimSpace(ip)
	serviceCIDR = strings.TrimSpace(serviceCIDR)
	if ip == "" && serviceCIDR == "" {
		return nil
	}

	// Build a map from Is4() bool -> service prefix so each DNS address
	// can be matched to the CIDR of its own address family.
	svcParts := strings.Split(serviceCIDR, ",")
	svcNets := make(map[bool]netip.Prefix, len(svcParts))
	for _, part := range svcParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		prefix, parseErr := netip.ParsePrefix(part)
		if parseErr != nil {
			return fmt.Errorf("to override the cluster DNS IP, the service CIDR must be valid: %w", parseErr)
		}
		svcNets[prefix.Addr().Is4()] = prefix
	}

	if ip == "" {
		return nil
	}

	// A single DNS IP must be IPv4; IPv6-only is not a supported mode.
	if !strings.Contains(ip, ",") {
		singleAddr, parseErr := netip.ParseAddr(ip)
		if parseErr == nil && !singleAddr.Is4() {
			return fmt.Errorf("a single DNS IP must be IPv4 (e.g. 10.53.0.10); for dual-stack provide IPv4,IPv6 (e.g. 10.53.0.10,fd53::a)")
		}
	}

	dnsParts := strings.Split(ip, ",")
	if len(dnsParts) > 2 {
		return fmt.Errorf("at most two DNS IPs (IPv4,IPv6) are allowed, got %d", len(dnsParts))
	}
	parsed := make([]netip.Addr, 0, len(dnsParts))
	for _, part := range dnsParts {
		part = strings.TrimSpace(part)
		ipAddr, parseErr := netip.ParseAddr(part)
		if parseErr != nil {
			return fmt.Errorf("invalid cluster DNS IP: %w", parseErr)
		}
		svcNet, ok := svcNets[ipAddr.Is4()]
		if !ok {
			return fmt.Errorf("invalid cluster DNS IP: %s has no matching service CIDR for its address family", part)
		}
		if !svcNet.Contains(ipAddr) {
			return fmt.Errorf("invalid cluster DNS IP: %s is not in the service CIDR %s", part, svcNet)
		}
		if ipAddr == svcNet.Masked().Addr() {
			return fmt.Errorf("invalid cluster DNS IP: %s is the network address of %s and cannot be used as a host address", part, svcNet)
		}
		parsed = append(parsed, ipAddr)
	}
	if len(parsed) == 2 {
		if !parsed[0].Is4() || parsed[1].Is4() {
			return fmt.Errorf("dual-stack DNS IPs must be in IPv4-first order (e.g. 10.53.0.10,fd53::a)")
		}
	}
	return nil
}

// ValidateCIDRMatchesIPFamilies enforces that the CIDR stack mode matches the
// configured IP families: IPv4-only mode requires a single IPv4 CIDR; dual-stack
// mode requires exactly two CIDRs in IPv4,IPv6 order.
// An empty cidr is always allowed (defaults are applied later).
func ValidateCIDRMatchesIPFamilies(cidr string, ipv6Enabled bool) error {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return nil
	}
	isDual := strings.Contains(cidr, ",")
	if ipv6Enabled && !isDual {
		return fmt.Errorf("dual-stack mode (IPv4,IPv6) requires both an IPv4 and an IPv6 CIDR (e.g. 10.52.0.0/16,fd52::/56)")
	}
	if !ipv6Enabled && isDual {
		return fmt.Errorf("IPv4-only mode requires a single IPv4 CIDR")
	}
	return nil
}

// ValidateDNSMatchesIPFamilies enforces DNS IP constraints based on the
// configured IP families: IPv4-only mode requires a single IPv4 DNS address.
// Dual-stack mode allows a single IPv4 address or IPv4+IPv6 pair.
// An empty dns string is always allowed.
func ValidateDNSMatchesIPFamilies(dns string, ipv6Enabled bool) error {
	dns = strings.TrimSpace(dns)
	if dns == "" {
		return nil
	}
	isDual := strings.Contains(dns, ",")
	if !ipv6Enabled && isDual {
		return fmt.Errorf("IPv4-only mode requires a single IPv4 DNS address")
	}
	return nil
}
