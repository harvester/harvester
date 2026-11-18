package iphelpers

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
)

// CompareIPs reports whether out of 2 given IPs, ipX and ipY, ipY is smaller (-1), the same (0) or larger (1).
// It does so by comparing each of the 16 bytes individually.
// IPs are stored in 16 byte representation regardless of the IP address family.
// For example:
// 192.168.0.3 - [0 0 0 0 0 0 0 0 0 0 255 255 192 168 0 3]
// 2000::3     - [32 0 0 0 0 0 0 0 0 0 0 0 0 0 0 3]
func CompareIPs(ipX net.IP, ipY net.IP) int {
	x := []byte(ipX.To16())
	y := []byte(ipY.To16())
	for i := 0; i < len(x); i++ {
		if x[i] < y[i] {
			return -1
		} else if x[i] > y[i] {
			return 1
		}
	}
	return 0
}

// DivideRangeBySize takes an ipRange i.e. 11.0.0.0/8 and a sliceSize i.e. /24
// and returns a list of IPNets that divide the input range into sizes
func DivideRangeBySize(inputNetwork string, sliceSizeString string) ([]string, error) {
	// Remove "/" from the start of the sliceSize
	sliceSizeString = strings.TrimPrefix(sliceSizeString, "/")

	sliceSize, err := strconv.Atoi(sliceSizeString)
	if err != nil {
		fmt.Println("Error:", err)
		return nil, nil
	}
	ip, ipNet, err := net.ParseCIDR(inputNetwork)
	if err != nil {
		return nil, fmt.Errorf("error parsing CIDR %s: %v", inputNetwork, err)
	}
	if !ip.Equal(ipNet.IP) {
		return nil, errors.New("netCIDR is not a valid network address")
	}
	netMaskSize, _ := ipNet.Mask.Size()
	if netMaskSize > int(sliceSize) {
		return nil, errors.New("subnetMaskSize must be greater or equal than netMaskSize")
	}

	totalSubnetsInNetwork := math.Pow(2, float64(sliceSize)-float64(netMaskSize))
	totalHostsInSubnet := math.Pow(2, 32-float64(sliceSize))
	subnetIntAddresses := make([]uint32, int(totalSubnetsInNetwork))
	// first subnet address is same as the network address
	subnetIntAddresses[0] = ip2int(ip.To4())
	for i := 1; i < int(totalSubnetsInNetwork); i++ {
		subnetIntAddresses[i] = subnetIntAddresses[i-1] + uint32(totalHostsInSubnet)
	}

	subnetCIDRs := make([]string, 0)
	for _, sia := range subnetIntAddresses {
		subnetCIDRs = append(
			subnetCIDRs,
			int2ip(sia).String()+"/"+strconv.Itoa(int(sliceSize)),
		)
	}
	return subnetCIDRs, nil
}

func ip2int(ip net.IP) uint32 {
	if len(ip) == 16 {
		panic("cannot convert IPv6 into uint32")
	}
	return binary.BigEndian.Uint32(ip)
}
func int2ip(nn uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, nn)
	return ip
}

// IsIPInRange returns true if a given IP is within the continuous range of start and end IP (inclusively).
func IsIPInRange(in net.IP, start net.IP, end net.IP) (bool, error) {
	if in == nil || start == nil || end == nil {
		return false, fmt.Errorf("cannot determine if IP is in range, either of the values is '<nil>', "+
			"in: %v, start: %v, end: %v", in, start, end)
	}
	return CompareIPs(in, start) >= 0 && CompareIPs(in, end) <= 0, nil
}

// NetworkIP returns the network IP of the subnet.
func NetworkIP(ipnet net.IPNet) net.IP {
	byteIP := []byte(ipnet.IP)             // []byte representation of IP.
	byteMask := []byte(ipnet.Mask)         // []byte representation of mask.
	networkIP := make([]byte, len(byteIP)) // []byte holding target IP.
	for k := range byteIP {
		networkIP[k] = byteIP[k] & byteMask[k]
	}
	return net.IP(networkIP)
}

// SubnetBroadcastIP returns the broadcast IP for a given net.IPNet.
// Mask will give us all fixed bits of the subnet (for the given byte)
// Inverted mask will give us all moving bits of the subnet (for the given byte)
// BroadcastIP = networkIP added to the inverted mask
func SubnetBroadcastIP(ipnet net.IPNet) net.IP {
	byteIP := []byte(ipnet.IP)               // []byte representation of IP.
	byteMask := []byte(ipnet.Mask)           // []byte representation of mask.
	broadcastIP := make([]byte, len(byteIP)) // []byte holding target IP.
	for k := range byteIP {
		invertedMask := byteMask[k] ^ 0xff                    // Inverted mask byte.
		broadcastIP[k] = byteIP[k]&byteMask[k] | invertedMask // Take network part and add the inverted mask to it.
	}
	return net.IP(broadcastIP)
}

// FirstUsableIP returns the first usable IP (not the network IP) in a given net.IPNet.
// This does not work for IPv4 /31 to /32 or IPv6 /127 to /128 netmasks.
func FirstUsableIP(ipnet net.IPNet) (net.IP, error) {
	if !HasUsableIPs(ipnet) {
		return nil, fmt.Errorf("net mask is too short, subnet %s has no usable IP addresses, it is too small", ipnet)
	}
	return IncIP(NetworkIP(ipnet)), nil
}

// LastUsableIP returns the last usable IP (not the broadcast IP in a given net.IPNet).
// This does not work for IPv4 /31 to /32 or IPv6 /127 to /128 netmasks.
func LastUsableIP(ipnet net.IPNet) (net.IP, error) {
	if !HasUsableIPs(ipnet) {
		return nil, fmt.Errorf("net mask is too short, subnet %s has no usable IP addresses, it is too small", ipnet)
	}
	return DecIP(SubnetBroadcastIP(ipnet)), nil
}

// HasUsableIPs returns true if this subnet has usable IPs (i.e. not the network nor the broadcast IP).
func HasUsableIPs(ipnet net.IPNet) bool {
	ones, totalBits := ipnet.Mask.Size()
	return totalBits-ones > 1
}

// IncIP increases the given IP address by one. IncIP will overflow for all 0xf adresses.
func IncIP(ip net.IP) net.IP {
	// Allocate a new IP.
	newIP := make(net.IP, len(ip))
	copy(newIP, ip)
	byteIP := []byte(newIP)
	// Get the end index (needed for IPv4 in 16 byte notation).
	endIndex := 0
	if ipv4 := newIP.To4(); ipv4 != nil {
		endIndex = len(byteIP) - len(ipv4)
	}

	// Start with the rightmost index first, increment it. If the index is < 256, then no overflow happened and we
	// increment and break else, continue to the next field in the byte.
	for i := len(byteIP) - 1; i >= endIndex; i-- {
		if byteIP[i] < 0xff {
			byteIP[i]++
			break
		} else {
			byteIP[i] = 0
		}
	}
	return net.IP(byteIP)
}

// DecIP decreases the given IP address by one. DecIP will overlow for all 0 addresses.
func DecIP(ip net.IP) net.IP {
	// allocate a new IP
	newIP := make(net.IP, len(ip))
	copy(newIP, ip)
	byteIP := []byte(newIP)
	// Get the end index (needed for IPv4 in 16 byte notation).
	endIndex := 0
	if ipv4 := newIP.To4(); ipv4 != nil {
		endIndex = len(byteIP) - len(ipv4)
	}

	// Start with the rightmost index first, decrement it. If the value != 0, then no overflow happened and we
	// decrement and break. Else, continue to the next field in the byte.
	for i := len(byteIP) - 1; i >= endIndex; i-- {
		if byteIP[i] != 0 {
			byteIP[i]--
			break
		} else {
			byteIP[i] = 0xff
		}
	}
	return net.IP(byteIP)
}

// IPGetOffset gets the absolute offset between ip1 and ip2, meaning that this offset will always be a positive number.
func IPGetOffset(ip1, ip2 net.IP) (uint64, error) {
	if ip1.To4() != nil && ip2.To4() == nil {
		return 0, fmt.Errorf("cannot calculate offset between IPv4 (%s) and IPv6 address (%s)", ip1, ip2)
	}
	if ip1.To4() == nil && ip2.To4() != nil {
		return 0, fmt.Errorf("cannot calculate offset between IPv6 (%s) and IPv4 address (%s)", ip1, ip2)
	}

	var ipOffset []byte
	var err error
	if CompareIPs(ip1, ip2) < 0 {
		ipOffset, err = byteSliceSub([]byte(ip2.To16()), []byte(ip1.To16()))
	} else {
		ipOffset, err = byteSliceSub([]byte(ip1.To16()), []byte(ip2.To16()))
	}
	if err != nil {
		return 0, err
	}
	return ipAddrToUint64(ipOffset), nil
}

// IPAddOffset show IP address plus given offset
func IPAddOffset(ip net.IP, offset uint64) net.IP {
	// Check IPv4 and its offset range
	if ip.To4() != nil && offset >= math.MaxUint32 {
		return nil
	}

	// make pseudo IP variable for offset
	idxIP := ipAddrFromUint64(offset)

	b, _ := byteSliceAdd([]byte(ip.To16()), []byte(idxIP))
	return net.IP(b)
}

// IsIPv4 checks if an IP is v4.
func IsIPv4(checkip net.IP) bool {
	return checkip.To4() != nil
}

// GetIPRange returns the first and last IP in a range.
// If either rangeStart or rangeEnd are inside the range of first usable IP to last usable IP, then use them. Otherwise,
// they will be silently ignored and the first usable IP and/or last usable IP will be used. A valid rangeEnd cannot
// be smaller than a valid rangeStart, otherwise it will be silently ignored.
// We do this also for backwards compatibility to avoid throwing unexpected errors in existing environments.
func GetIPRange(ipnet net.IPNet, rangeStart net.IP, rangeEnd net.IP) (net.IP, net.IP, error) {
	firstUsableIP, err := FirstUsableIP(ipnet)
	if err != nil {
		return nil, nil, err
	}
	lastUsableIP, err := LastUsableIP(ipnet)
	if err != nil {
		return nil, nil, err
	}
	if rangeStart != nil {
		rangeStartInRange, err := IsIPInRange(rangeStart, firstUsableIP, lastUsableIP)
		if err != nil {
			return nil, nil, err
		}
		if rangeStartInRange {
			firstUsableIP = rangeStart
		}
	}
	if rangeEnd != nil {
		rangeEndInRange, err := IsIPInRange(rangeEnd, firstUsableIP, lastUsableIP)
		if err != nil {
			return nil, nil, err
		}
		if rangeEndInRange {
			lastUsableIP = rangeEnd
		}
	}
	return firstUsableIP, lastUsableIP, nil
}

// byteSliceAdd adds ar1 to ar2
// note: ar1/ar2 should be 16-length array
func byteSliceAdd(ar1, ar2 []byte) ([]byte, error) {
	if len(ar1) != len(ar2) {
		return nil, fmt.Errorf("byteSliceAdd: bytes array mismatch: %v != %v", len(ar1), len(ar2))
	}
	carry := uint(0)

	sumByte := make([]byte, 16)
	for n := range ar1 {
		sum := uint(ar1[15-n]) + uint(ar2[15-n]) + carry
		carry = 0
		if sum > 255 {
			carry = 1
		}
		sumByte[15-n] = uint8(sum)
	}

	return sumByte, nil
}

// byteSliceSub subtracts ar2 from ar1. This function assumes that ar1 > ar2
// note: ar1/ar2 should be 16-length array
func byteSliceSub(ar1, ar2 []byte) ([]byte, error) {
	if len(ar1) != len(ar2) {
		return nil, fmt.Errorf("byteSliceSub: bytes array mismatch")
	}
	carry := int(0)

	sumByte := make([]byte, 16)
	for n := range ar1 {
		var sum int
		sum = int(ar1[15-n]) - int(ar2[15-n]) - carry
		if sum < 0 {
			sum = 0x100 - int(ar1[15-n]) - int(ar2[15-n]) - carry
			carry = 1
		} else {
			carry = 0
		}
		sumByte[15-n] = uint8(sum)
	}

	return sumByte, nil
}

func ipAddrToUint64(ip net.IP) uint64 {
	num := uint64(0)
	ipArray := []byte(ip)
	for n := range ipArray {
		num = num << 8
		num = uint64(ipArray[n]) + num
	}
	return num
}

func ipAddrFromUint64(num uint64) net.IP {
	idxByte := make([]byte, 16)
	i := num
	for n := range idxByte {
		idxByte[15-n] = byte(0xff & i)
		i = i >> 8
	}
	return net.IP(idxByte)
}
