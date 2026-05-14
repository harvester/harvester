// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// This file contains code taken from gVisor.

//go:build go1.12
// +build go1.12

package nclient4

import (
	"encoding/binary"
	"net"

	"github.com/u-root/uio/uio"
)

const (
	versIHL     = 0
	tos         = 1
	totalLen    = 2
	id          = 4
	flagsFO     = 6
	ttl         = 8
	protocol    = 9
	checksumOff = 10
	srcAddr     = 12
	dstAddr     = 16
)

// transportProtocolNumber is the number of a transport protocol.
type transportProtocolNumber uint32

// ipv4Fields contains the fields of an IPv4 packet. It is used to describe the
// fields of a packet that needs to be encoded.
type ipv4Fields struct {
	// IHL is the "internet header length" field of an IPv4 packet.
	IHL uint8

	// TOS is the "type of service" field of an IPv4 packet.
	TOS uint8

	// TotalLength is the "total length" field of an IPv4 packet.
	TotalLength uint16

	// ID is the "identification" field of an IPv4 packet.
	ID uint16

	// Flags is the "flags" field of an IPv4 packet.
	Flags uint8

	// FragmentOffset is the "fragment offset" field of an IPv4 packet.
	FragmentOffset uint16

	// TTL is the "time to live" field of an IPv4 packet.
	TTL uint8

	// Protocol is the "protocol" field of an IPv4 packet.
	Protocol uint8

	// checksum is the "checksum" field of an IPv4 packet.
	checksum uint16

	// SrcAddr is the "source ip address" of an IPv4 packet.
	SrcAddr net.IP

	// DstAddr is the "destination ip address" of an IPv4 packet.
	DstAddr net.IP
}

// ipv4 represents an ipv4 header stored in a byte array.
// Most of the methods of IPv4 access to the underlying slice without
// checking the boundaries and could panic because of 'index out of range'.
// Always call IsValid() to validate an instance of IPv4 before using other methods.
type ipv4 []byte

const (
	// ipv4MinimumSize is the minimum size of a valid IPv4 packet.
	ipv4MinimumSize = 20

	// ipv4MaximumHeaderSize is the maximum size of an IPv4 header. Given
	// that there are only 4 bits to represents the header length in 32-bit
	// units, the header cannot exceed 15*4 = 60 bytes.
	ipv4MaximumHeaderSize = 60

	// ipv4AddressSize is the size, in bytes, of an IPv4 address.
	ipv4AddressSize = 4

	// IPv4Version is the version of the IPv4 protocol.
	ipv4Version = 4
)

// ipVersion returns the version of IP used in the given packet. It returns -1
// if the packet is not large enough to contain the version field.
func ipVersion(b []byte) int {
	// Length must be at least offset+length of version field.
	if len(b) < versIHL+1 {
		return -1
	}
	return int(b[versIHL] >> ipVersionShift)
}

const (
	ipVersionShift = 4
)

// headerLength returns the value of the "header length" field of the ipv4
// header.
func (b ipv4) headerLength() uint8 {
	return (b[versIHL] & 0xf) * 4
}

// protocol returns the value of the protocol field of the ipv4 header.
func (b ipv4) protocol() uint8 {
	return b[protocol]
}

// sourceAddress returns the "source address" field of the ipv4 header.
func (b ipv4) sourceAddress() net.IP {
	return net.IP(b[srcAddr : srcAddr+ipv4AddressSize])
}

// destinationAddress returns the "destination address" field of the ipv4
// header.
func (b ipv4) destinationAddress() net.IP {
	return net.IP(b[dstAddr : dstAddr+ipv4AddressSize])
}

// transportProtocol implements Network.transportProtocol.
func (b ipv4) transportProtocol() transportProtocolNumber {
	return transportProtocolNumber(b.protocol())
}

// payloadLength returns the length of the payload portion of the ipv4 packet.
func (b ipv4) payloadLength() uint16 {
	return b.totalLength() - uint16(b.headerLength())
}

// totalLength returns the "total length" field of the ipv4 header.
func (b ipv4) totalLength() uint16 {
	return binary.BigEndian.Uint16(b[totalLen:])
}

// setTotalLength sets the "total length" field of the ipv4 header.
func (b ipv4) setTotalLength(totalLength uint16) {
	binary.BigEndian.PutUint16(b[totalLen:], totalLength)
}

// setChecksum sets the checksum field of the ipv4 header.
func (b ipv4) setChecksum(v uint16) {
	binary.BigEndian.PutUint16(b[checksumOff:], v)
}

// setFlagsFragmentOffset sets the "flags" and "fragment offset" fields of the
// ipv4 header.
func (b ipv4) setFlagsFragmentOffset(flags uint8, offset uint16) {
	v := (uint16(flags) << 13) | (offset >> 3)
	binary.BigEndian.PutUint16(b[flagsFO:], v)
}

// calculateChecksum calculates the checksum of the ipv4 header.
func (b ipv4) calculateChecksum() uint16 {
	return checksum(b[:b.headerLength()], 0)
}

// encode encodes all the fields of the ipv4 header.
func (b ipv4) encode(i *ipv4Fields) {
	b[versIHL] = (4 << 4) | ((i.IHL / 4) & 0xf)
	b[tos] = i.TOS
	b.setTotalLength(i.TotalLength)
	binary.BigEndian.PutUint16(b[id:], i.ID)
	b.setFlagsFragmentOffset(i.Flags, i.FragmentOffset)
	b[ttl] = i.TTL
	b[protocol] = i.Protocol
	b.setChecksum(i.checksum)
	copy(b[srcAddr:srcAddr+ipv4AddressSize], i.SrcAddr)
	copy(b[dstAddr:dstAddr+ipv4AddressSize], i.DstAddr)
}

// isValid performs basic validation on the packet.
func (b ipv4) isValid(pktSize int) bool {
	if len(b) < ipv4MinimumSize {
		return false
	}

	hlen := int(b.headerLength())
	tlen := int(b.totalLength())
	if hlen < ipv4MinimumSize || hlen > tlen || tlen > pktSize {
		return false
	}

	if ipVersion(b) != ipv4Version {
		return false
	}

	return true
}

const (
	udpSrcPort  = 0
	udpDstPort  = 2
	udpLength   = 4
	udpchecksum = 6
)

// udpFields contains the fields of a udp packet. It is used to describe the
// fields of a packet that needs to be encoded.
type udpFields struct {
	// SrcPort is the "source port" field of a udp packet.
	SrcPort uint16

	// DstPort is the "destination port" field of a UDP packet.
	DstPort uint16

	// Length is the "length" field of a UDP packet.
	Length uint16

	// checksum is the "checksum" field of a UDP packet.
	checksum uint16
}

// udp represents a udp header stored in a byte array.
type udp []byte

const (
	// udpMinimumSize is the minimum size of a valid udp packet.
	udpMinimumSize = 8

	// udpProtocolNumber is udp's transport protocol number.
	udpProtocolNumber transportProtocolNumber = 17
)

// sourcePort returns the "source port" field of the udp header.
func (b udp) sourcePort() uint16 {
	return binary.BigEndian.Uint16(b[udpSrcPort:])
}

// DestinationPort returns the "destination port" field of the udp header.
func (b udp) destinationPort() uint16 {
	return binary.BigEndian.Uint16(b[udpDstPort:])
}

// Length returns the "length" field of the udp header.
func (b udp) length() uint16 {
	return binary.BigEndian.Uint16(b[udpLength:])
}

// setChecksum sets the "checksum" field of the udp header.
func (b udp) setChecksum(checksum uint16) {
	binary.BigEndian.PutUint16(b[udpchecksum:], checksum)
}

// calculateChecksum calculates the checksum of the udp packet, given the total
// length of the packet and the checksum of the network-layer pseudo-header
// (excluding the total length) and the checksum of the payload.
func (b udp) calculateChecksum(partialchecksum uint16, totalLen uint16) uint16 {
	// Add the length portion of the checksum to the pseudo-checksum.
	tmp := make([]byte, 2)
	binary.BigEndian.PutUint16(tmp, totalLen)
	xsum := checksum(tmp, partialchecksum)

	// Calculate the rest of the checksum.
	return checksum(b[:udpMinimumSize], xsum)
}

// encode encodes all the fields of the udp header.
func (b udp) encode(u *udpFields) {
	binary.BigEndian.PutUint16(b[udpSrcPort:], u.SrcPort)
	binary.BigEndian.PutUint16(b[udpDstPort:], u.DstPort)
	binary.BigEndian.PutUint16(b[udpLength:], u.Length)
	binary.BigEndian.PutUint16(b[udpchecksum:], u.checksum)
}

func calculateChecksum(buf []byte, initial uint32) uint16 {
	v := initial

	l := len(buf)
	if l&1 != 0 {
		l--
		v += uint32(buf[l]) << 8
	}

	for i := 0; i < l; i += 2 {
		v += (uint32(buf[i]) << 8) + uint32(buf[i+1])
	}

	return checksumCombine(uint16(v), uint16(v>>16))
}

// checksum calculates the checksum (as defined in RFC 1071) of the bytes in the
// given byte array.
//
// The initial checksum must have been computed on an even number of bytes.
func checksum(buf []byte, initial uint16) uint16 {
	return calculateChecksum(buf, uint32(initial))
}

// checksumCombine combines the two uint16 to form their checksum. This is done
// by adding them and the carry.
//
// Note that checksum a must have been computed on an even number of bytes.
func checksumCombine(a, b uint16) uint16 {
	v := uint32(a) + uint32(b)
	return uint16(v + v>>16)
}

// pseudoHeaderchecksum calculates the pseudo-header checksum for the
// given destination protocol and network address, ignoring the length
// field. pseudo-headers are needed by transport layers when calculating
// their own checksum.
func pseudoHeaderchecksum(protocol transportProtocolNumber, srcAddr net.IP, dstAddr net.IP) uint16 {
	xsum := checksum([]byte(srcAddr), 0)
	xsum = checksum([]byte(dstAddr), xsum)
	return checksum([]byte{0, uint8(protocol)}, xsum)
}

func udp4pkt(packet []byte, dest *net.UDPAddr, src *net.UDPAddr) []byte {
	ipLen := ipv4MinimumSize
	udpLen := udpMinimumSize

	h := make([]byte, 0, ipLen+udpLen+len(packet))
	hdr := uio.NewBigEndianBuffer(h)

	ipv4fields := &ipv4Fields{
		IHL:         ipv4MinimumSize,
		TotalLength: uint16(ipLen + udpLen + len(packet)),
		TTL:         64, // Per RFC 1700's recommendation for IP time to live
		Protocol:    uint8(udpProtocolNumber),
		SrcAddr:     src.IP.To4(),
		DstAddr:     dest.IP.To4(),
	}
	ipv4hdr := ipv4(hdr.WriteN(ipLen))
	ipv4hdr.encode(ipv4fields)
	ipv4hdr.setChecksum(^ipv4hdr.calculateChecksum())

	udphdr := udp(hdr.WriteN(udpLen))
	udphdr.encode(&udpFields{
		SrcPort: uint16(src.Port),
		DstPort: uint16(dest.Port),
		Length:  uint16(udpLen + len(packet)),
	})

	xsum := checksum(packet, pseudoHeaderchecksum(
		ipv4hdr.transportProtocol(), ipv4fields.SrcAddr, ipv4fields.DstAddr))
	udphdr.setChecksum(^udphdr.calculateChecksum(xsum, udphdr.length()))

	hdr.WriteBytes(packet)
	return hdr.Data()
}
