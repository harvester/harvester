// This is lease support for nclient4

package nclient4

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// Lease contains a DHCPv4 lease after DORA.
// note: Lease doesn't include binding interface name
type Lease struct {
	Offer        *dhcpv4.DHCPv4
	ACK          *dhcpv4.DHCPv4
	CreationTime time.Time
}

// Release send DHCPv4 release messsage to server, based on specified lease.
// release is sent as unicast per RFC2131, section 4.4.4.
// Note: some DHCP server requries of using assigned IP address as source IP,
// use nclient4.WithUnicast to create client for such case.
func (c *Client) Release(lease *Lease, modifiers ...dhcpv4.Modifier) error {
	if lease == nil {
		return fmt.Errorf("lease is nil")
	}
	req, err := dhcpv4.NewReleaseFromACK(lease.ACK, modifiers...)
	if err != nil {
		return fmt.Errorf("fail to create release request,%w", err)
	}
	_, err = c.conn.WriteTo(req.ToBytes(), &net.UDPAddr{IP: lease.ACK.Options.Get(dhcpv4.OptionServerIdentifier), Port: ServerPort})
	if err == nil {
		c.logger.PrintMessage("sent message:", req)
	}
	return err
}

// Renew sends a DHCPv4 request to the server to renew the given lease. The renewal information is
// sourced from the initial offer in the lease, and the ACK of the lease is updated to the ACK of
// the latest renewal. This avoids issues with DHCP servers that omit information needed to build a
// completely new lease from their renewal ACK (such as the Windows DHCP Server).
func (c *Client) Renew(ctx context.Context, lease *Lease, modifiers ...dhcpv4.Modifier) (*Lease, error) {
	if lease == nil {
		return nil, fmt.Errorf("lease is nil")
	}

	request, err := dhcpv4.NewRenewFromAck(lease.ACK, dhcpv4.PrependModifiers(modifiers,
		dhcpv4.WithOption(dhcpv4.OptMaxMessageSize(MaxMessageSize)))...)
	if err != nil {
		return nil, fmt.Errorf("unable to create a request: %w", err)
	}

	// Servers are supposed to only respond to Requests containing their server identifier,
	// but sometimes non-compliant servers respond anyway.
	// Clients are not required to validate this field, but servers are required to
	// include the server identifier in their Offer per RFC 2131 Section 4.3.1 Table 3.
	response, err := c.SendAndRead(ctx, c.serverAddr, request, IsAll(
		IsCorrectServer(lease.Offer.ServerIdentifier()),
		IsMessageType(dhcpv4.MessageTypeAck, dhcpv4.MessageTypeNak)))
	if err != nil {
		return nil, fmt.Errorf("got an error while processing the request: %w", err)
	}
	if response.MessageType() == dhcpv4.MessageTypeNak {
		return nil, &ErrNak{
			Offer: lease.Offer,
			Nak:   response,
		}
	}

	// Return a new lease with the latest ACK and updated creation time
	return &Lease{
		Offer:        lease.Offer,
		ACK:          response,
		CreationTime: time.Now(),
	}, nil
}
