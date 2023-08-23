package iptables

import (
	"fmt"
	"strings"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/vishvananda/netlink"
	iputils "k8s.io/kubernetes/pkg/util/iptables"
	"k8s.io/utils/exec"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

const (
	networkInterfaceNamePrefix = "k6t-net"
)

// each launcher pod has the k8s-net* interface which corresponds to the bridge interface
// being injected by multus. we attempt to apply forwarding rules to these interfaces
// to ensure only whitelisted traffic is allowed to table
func ApplyRules(sg *harvesterv1beta1.SecurityGroup, vm *kubevirtv1.VirtualMachine) error {
	exec := exec.New()
	iptIface := iputils.New(exec, iputils.ProtocolIPv4)
	ok, err := iptIface.EnsureChain(iputils.TableFilter, iputils.ChainForward)
	if err != nil {
		return fmt.Errorf("error querying chain exists")
	}

	// if chain already exists, flush and recreate to ensure latest rules are applied
	if ok {
		iptIface.FlushChain(iputils.TableFilter, iputils.ChainForward)
	}

	links, err := identifyInterfaces()
	if err != nil {
		return err
	}

	if len(links) == 0 {
		return fmt.Errorf("internal error: expected to find atleast one k6t-net interface for a bridge interface")
	}

	macSourceAddresses := querySourceAddress(vm)
	rules := generateRules(sg, links, macSourceAddresses)
	for _, v := range rules {
		_, err := iptIface.EnsureRule(iputils.Append, iputils.TableFilter, iputils.ChainForward, v...)
		if err != nil {
			return fmt.Errorf("error during application of rule: %v", err)
		}
	}
	return nil
}

// generateRules will generate the correct iptables chain to ensure only whitelisted traffic is allowed to VM
func generateRules(sg *harvesterv1beta1.SecurityGroup, links []string, macSourceAddresses []string) [][]string {
	var rules [][]string
	for _, rule := range sg.Spec {
		for _, link := range links {
			// if no port is specified allow all traffic from source address //
			iptableRule := []string{"ALLOW", "-p", rule.IpProtocol, "-s", rule.SourceAddress, "-i", link, "-j", "ACCEPT"}
			if len(rule.SourcePortRange) != 0 {
				iptableRule = append(iptableRule, "-m", "multiport", "--dport", generatePortString(rule.SourcePortRange))
			}
			rules = append(rules, iptableRule)
		}
	}
	// allow all traffic matching macAddresses in VM from source. results in
	// iptables -A FORWARD -m mac --mac-source 46:8c:01:13:38:13 -j ACCEPT -m state --state NEW,ESTABLISHED,RELATED
	for _, v := range macSourceAddresses {
		iptableRule := []string{"-m", "mac", "--mac-source", v, "-j", "ACCEPT", "-m", "state", "--state", "NEW", "ESTABLISHED", "RELATED"}
		rules = append(rules, iptableRule)
	}

	// final rule in the FORWARD chain is to drop any traffic not matching the rules
	// as a result a security group with an empty []IngressRules list will
	// drop all incoming traffic. results in rule
	// iptables -A FORWARD  -i k6t-net1 -p icmp -j DROP -m state --state NEW
	for _, link := range links {
		rules = append(rules, []string{"-j", "DROP", "-i", link, "-m", "state", "--state", "NEW"})
	}
	return rules
}

// need to list all k8s-net interfaces as this needs to be added
// to the rules
func identifyInterfaces() ([]string, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("error querying network links: %v", err)
	}

	var matchingLinks []string
	for _, v := range links {
		if strings.Contains(v.Attrs().Name, networkInterfaceNamePrefix) {
			matchingLinks = append(matchingLinks, v.Attrs().Name)
		}
	}

	return matchingLinks, nil
}

func generatePortString(ports []uint32) string {
	var stringPort string
	for i, v := range ports {
		if i == 0 {
			stringPort = fmt.Sprintf("%d", v)
		} else {
			stringPort = fmt.Sprintf("%s,%d", stringPort, v)
		}
	}

	return stringPort
}

func querySourceAddress(vm *kubevirtv1.VirtualMachine) []string {
	var macAddresses []string
	for _, v := range vm.Spec.Template.Spec.Domain.Devices.Interfaces {
		if v.Bridge != nil {
			macAddresses = append(macAddresses, v.MacAddress)
		}
	}
	return macAddresses
}
