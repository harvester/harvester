package types

import (
	"fmt"
	"net"

	netutils "k8s.io/utils/net"
)

func sanitizeIP(address string) (net.IP, error) {
	sanitizedAddress := netutils.ParseIPSloppy(address)
	if sanitizedAddress == nil {
		return nil, fmt.Errorf("%s is not a valid IP address", address)
	}

	return sanitizedAddress, nil
}
