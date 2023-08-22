package iptables

import (
	"fmt"
	"strings"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/vishvananda/netlink"
	iputils "k8s.io/kubernetes/pkg/util/iptables"
	"k8s.io/utils/exec"
)

const (
	networkInterfaceNamePrefix = "k6t-net"
)

// each launcher pod has the k8s-net* interface which corresponds to the bridge interface
// being injected by multus. we attempt to apply forwarding rules to these interfaces
// to ensure only whitelisted traffic is allowed to table
func ApplyRules(sg *harvesterv1beta1.SecurityGroup) error {
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

	rules := generateRules(sg, links)
	for _, v := range rules {
		_, err := iptIface.EnsureRule(iputils.Append, iputils.TableFilter, iputils.ChainForward, v...)
		if err != nil {
			return fmt.Errorf("error during application of rule: %v", err)
		}
	}
	return nil
}

func generateRules(sg *harvesterv1beta1.SecurityGroup, links []string) [][]string {
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

	// final rule in the FORWARD chain is to drop any traffic not matching the rules
	// as a result a security group with an empty []IngressRules list will
	// drop all incoming traffic
	for _, link := range links {
		rules = append(rules, []string{"DROP", "-i", link})
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
