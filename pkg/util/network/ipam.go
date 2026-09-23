package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"strings"

	whereaboutsv1alpha1 "github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
)

// AssignHostIPs binds each node to an address in hostIPRange. Existing bindings in
// current are kept while they stay inside the range, and every other node gets the
// lowest free address, in node name order. Nodes that cannot get an address because
// the range is exhausted are returned as unassigned.
func AssignHostIPs(hostIPRange string, current map[string]string, nodes []string) (map[string]string, []string, error) {
	prefix, err := netip.ParsePrefix(hostIPRange)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid host IP range %q: %w", hostIPRange, err)
	}
	prefix = prefix.Masked()

	sorted := slices.Clone(nodes)
	sort.Strings(sorted)

	assigned := make(map[string]string, len(sorted))
	used := make(map[netip.Addr]struct{}, len(sorted))
	var pending []string
	for _, node := range sorted {
		addr, err := netip.ParseAddr(current[node])
		if err != nil || !prefix.Contains(addr) {
			pending = append(pending, node)
			continue
		}
		if _, ok := used[addr]; ok {
			pending = append(pending, node)
			continue
		}
		used[addr] = struct{}{}
		assigned[node] = addr.String()
	}

	var unassigned []string
	next := prefix.Addr()
	for _, node := range pending {
		for prefix.Contains(next) {
			if _, ok := used[next]; !ok {
				break
			}
			next = next.Next()
		}
		if !prefix.Contains(next) {
			unassigned = append(unassigned, node)
			continue
		}
		used[next] = struct{}{}
		assigned[node] = next.String()
	}

	return assigned, unassigned, nil
}

// SetManagedExcludes rewrites the Whereabouts exclude list of the IPv4 range in a NAD
// config: entries in previous are removed, entries in desired are added, and anything
// else, including fields Harvester does not know about, is left untouched. The config
// is returned as is when the exclude list does not change.
func SetManagedExcludes(config string, previous, desired []string) (string, bool, error) {
	var conf map[string]any
	dec := json.NewDecoder(strings.NewReader(config))
	dec.UseNumber()
	if err := dec.Decode(&conf); err != nil {
		return "", false, fmt.Errorf("failed to decode NAD config: %w", err)
	}

	ipam, ok := conf["ipam"].(map[string]any)
	if !ok {
		return "", false, fmt.Errorf("NAD config has no ipam section")
	}

	target, err := ipv4RangeEntry(ipam)
	if err != nil {
		return "", false, err
	}

	var current []string
	if raw, ok := target["exclude"].([]any); ok {
		for _, e := range raw {
			s, ok := e.(string)
			if !ok {
				return "", false, fmt.Errorf("unexpected exclude entry %v", e)
			}
			current = append(current, s)
		}
	}

	updated := make([]string, 0, len(current)+len(desired))
	for _, e := range current {
		if !slices.Contains(previous, e) || slices.Contains(desired, e) {
			updated = append(updated, e)
		}
	}
	for _, e := range desired {
		if !slices.Contains(updated, e) {
			updated = append(updated, e)
		}
	}

	if slices.Equal(current, updated) {
		return config, false, nil
	}

	if len(updated) == 0 {
		delete(target, "exclude")
	} else {
		target["exclude"] = updated
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(conf); err != nil {
		return "", false, fmt.Errorf("failed to encode NAD config: %w", err)
	}
	return strings.TrimSuffix(buf.String(), "\n"), true, nil
}

// ipv4RangeEntry returns the object holding the IPv4 range and its exclude list,
// which is the ipam section itself for single-stack, or one of its ipRanges entries
// for dual-stack.
func ipv4RangeEntry(ipam map[string]any) (map[string]any, error) {
	ranges, ok := ipam["ipRanges"].([]any)
	if !ok {
		return ipam, nil
	}
	for _, r := range ranges {
		entry, ok := r.(map[string]any)
		if !ok {
			continue
		}
		cidr, _ := entry["range"].(string)
		if prefix, err := netip.ParsePrefix(cidr); err == nil && prefix.Addr().Is4() {
			return entry, nil
		}
	}
	return nil, fmt.Errorf("NAD config has no IPv4 range")
}

// BridgeNAD is the part of a Harvester bridge NAD config that locates its network.
type BridgeNAD struct {
	ClusterNetwork string
	Vlan           uint16
	Range          string
}

// ParseBridgeNADConfig reads the cluster network, VLAN and IPv4 range of a bridge NAD.
func ParseBridgeNADConfig(config string) (BridgeNAD, error) {
	var conf struct {
		Type   string         `json:"type"`
		Bridge string         `json:"bridge"`
		Vlan   uint16         `json:"vlan"`
		IPAM   map[string]any `json:"ipam"`
	}
	if err := json.Unmarshal([]byte(config), &conf); err != nil {
		return BridgeNAD{}, fmt.Errorf("failed to decode NAD config: %w", err)
	}
	if conf.Type != DefaultCNI || !strings.HasSuffix(conf.Bridge, BridgeSuffix) {
		return BridgeNAD{}, fmt.Errorf("NAD config is not a Harvester bridge network")
	}

	entry, err := ipv4RangeEntry(conf.IPAM)
	if err != nil {
		return BridgeNAD{}, err
	}
	cidr, _ := entry["range"].(string)
	if prefix, err := netip.ParsePrefix(cidr); err != nil || !prefix.Addr().Is4() {
		return BridgeNAD{}, fmt.Errorf("NAD config has no IPv4 range")
	}

	return BridgeNAD{
		ClusterNetwork: strings.TrimSuffix(conf.Bridge, BridgeSuffix),
		Vlan:           conf.Vlan,
		Range:          cidr,
	}, nil
}

// IPPoolAllocatedAddrs returns the addresses allocated in a Whereabouts IPPool. The
// allocation keys are offsets from the pool's network address.
func IPPoolAllocatedAddrs(pool *whereaboutsv1alpha1.IPPool) ([]netip.Addr, error) {
	prefix, err := netip.ParsePrefix(pool.Spec.Range)
	if err != nil {
		return nil, fmt.Errorf("invalid IPPool range %q: %w", pool.Spec.Range, err)
	}
	base := new(big.Int).SetBytes(prefix.Masked().Addr().AsSlice())
	size := len(prefix.Addr().AsSlice())

	addrs := make([]netip.Addr, 0, len(pool.Spec.Allocations))
	for key := range pool.Spec.Allocations {
		offset, err := strconv.ParseUint(key, 10, 64)
		if err != nil {
			continue
		}
		sum := new(big.Int).Add(base, new(big.Int).SetUint64(offset))
		if sum.BitLen() > size*8 {
			continue
		}
		addr, _ := netip.AddrFromSlice(sum.FillBytes(make([]byte, size)))
		addrs = append(addrs, addr)
	}
	return addrs, nil
}
