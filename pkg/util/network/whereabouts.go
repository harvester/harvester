package network

import (
	"fmt"
	"net"

	whereaboutsk8s "github.com/k8snetworkplumbingwg/whereabouts/pkg/storage/kubernetes"
)

// WhereaboutsIPPoolName returns the name of the Whereabouts IPPool for a CIDR, as
// Whereabouts derives it for a network without a network name.
func WhereaboutsIPPoolName(cidr string) (string, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	return whereaboutsk8s.IPPoolName(whereaboutsk8s.PoolIdentifier{IpRange: network.String()}), nil
}
