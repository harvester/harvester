package network

import (
	"fmt"
	"net"
	"strings"
)

// WhereaboutsIPPoolName derives the Whereabouts IPPool Kubernetes object name from a
// CIDR string. The name is the masked network address with the prefix length
// appended via a dash. Colons in IPv6 addresses are replaced with dashes so the
// result is a valid Kubernetes object name (e.g. "fd00::/64" -> "fd00---64").
func WhereaboutsIPPoolName(cidr string) (string, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	parts := strings.SplitN(network.String(), "/", 2)
	addr := strings.ReplaceAll(parts[0], ":", "-")
	return addr + "-" + parts[1], nil
}
