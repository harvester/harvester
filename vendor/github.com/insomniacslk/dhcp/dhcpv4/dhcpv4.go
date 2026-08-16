//go:build !tinygo

// Package dhcpv4 provides encoding and decoding of DHCPv4 packets and options.
//
// Example Usage:
//
//	p, err := dhcpv4.New(
//	  dhcpv4.WithClientIP(net.IP{192, 168, 0, 1}),
//	  dhcpv4.WithMessageType(dhcpv4.MessageTypeInform),
//	)
//	p.UpdateOption(dhcpv4.OptServerIdentifier(net.IP{192, 110, 110, 110}))
//
//	// Retrieve the DHCP Message Type option.
//	m := p.MessageType()
//
//	bytesOnTheWire := p.ToBytes()
//	longSummary := p.Summary()
package dhcpv4

import (
	"errors"
	"fmt"
	"net"
)

// IPv4AddrsForInterface obtains the currently-configured, non-loopback IPv4
// addresses for iface.
func IPv4AddrsForInterface(iface *net.Interface) ([]net.IP, error) {
	if iface == nil {
		return nil, errors.New("IPv4AddrsForInterface: iface cannot be nil")
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	return GetExternalIPv4Addrs(addrs)
}

// NewDiscoveryForInterface builds a new DHCPv4 Discovery message, with a default
// Ethernet HW type and the hardware address obtained from the specified
// interface.
func NewDiscoveryForInterface(ifname string, modifiers ...Modifier) (*DHCPv4, error) {
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return nil, err
	}
	return NewDiscovery(iface.HardwareAddr, modifiers...)
}

// NewInformForInterface builds a new DHCPv4 Informational message with default
// Ethernet HW type and the hardware address obtained from the specified
// interface.
func NewInformForInterface(ifname string, needsBroadcast bool) (*DHCPv4, error) {
	// get hw addr
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return nil, err
	}

	// Set Client IP as iface's currently-configured IP.
	localIPs, err := IPv4AddrsForInterface(iface)
	if err != nil || len(localIPs) == 0 {
		return nil, fmt.Errorf("could not get local IPs for iface %s", ifname)
	}
	pkt, err := NewInform(iface.HardwareAddr, localIPs[0])
	if err != nil {
		return nil, err
	}

	if needsBroadcast {
		pkt.SetBroadcast()
	} else {
		pkt.SetUnicast()
	}
	return pkt, nil
}
