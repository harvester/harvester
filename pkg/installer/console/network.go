package console

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/dell/goiscsi"

	"github.com/harvester/harvester/pkg/installer/config"
)

func isDefaultRoute(r netlink.Route) bool {
	// In netlink, a nil Dst is a common way to represent 'match everything'
	if r.Dst == nil {
		return true
	}

	// Ensure the mask is present before calling Size() to avoid panics
	if r.Dst.Mask != nil {
		ones, _ := r.Dst.Mask.Size()
		if ones == 0 {
			return true
		}
	}

	return false
}

func checkDefaultRoute() (bool, error) {
	routes, err := netlink.RouteList(nil, syscall.AF_INET)
	if err != nil {
		logrus.Errorf("Failed to list routes: %s", err.Error())
		return false, err
	}

	for i, r := range routes {
		// Log summaries instead of dumping the whole struct to keep logs clean
		logrus.WithFields(logrus.Fields{
			"index": i,
			"dst":   r.Dst,
			"gw":    r.Gw,
			"iface": r.LinkIndex,
		}).Debug("Processing route")

		if isDefaultRoute(r) {
			logrus.WithField("route", r).Info("Default route detected")
			return true, nil
		}
	}

	return false, nil
}

func applyNetworks(network config.Network, hostname string, ipv6Enabled bool) error {
	if err := config.RestoreOriginalNetworkConfig(); err != nil {
		return err
	}
	if err := config.SaveOriginalNetworkConfig(); err != nil {
		return err
	}

	// If called without a hostname set, we enable setting hostname via the
	// DHCP server, in case the DHCP server is configured to give us a
	// hostname we can use by default.
	//
	// If we move the network interface page of the installer so it's before
	// the hostname page, this function will activate once the management
	// NIC is configured, and if the DHCP server is configured correctly,
	// the system hostname will be set to the one provided by the server.
	// Later, on the hostname page, we can default the hostname field to
	// the current system hostname.
	//
	// In order for this to work with NetworkManager, we first have to clear
	// the currently set hostname (which will be "rancher" in the installer,
	// after clearing it will be "localhost").  If we don't do this,
	// NetworkManager will _not_ apply the hostname that comes from the DHCP
	// server.

	getHostname := func() (string, error) {
		output, err := exec.Command("hostnamectl", "hostname").CombinedOutput()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(output)), nil
	}

	setHostname := func(hostname string) error {
		_, err := exec.Command("hostnamectl", "hostname", hostname).CombinedOutput()
		return err
	}

	previousHostname, err := getHostname()
	if err != nil {
		logrus.Warnf("Failed to get hostname: %v", err)
	}
	if hostname == "" {
		err = setHostname("")
		if err != nil {
			logrus.Warnf("Failed to clear hostname: %v", err)
		}
	} else {
		err = setHostname(hostname)
		if err != nil {
			logrus.Errorf("Failed to set hostname: %v", err)
			return err
		}
	}

	if err := applyKernelIPv6(ipv6Enabled); err != nil {
		return err
	}

	err = config.UpdateManagementInterfaceConfig(network, []string{}, config.NMConnectionPath, true, ipv6Enabled)
	if err != nil {
		return err
	}

	// Restore Down NIC to up
	if err := upAllLinks(); err != nil {
		logrus.Errorf("failed to bring all link up: %s", err.Error())
	}

	if hostname == "" && previousHostname != "" {
		currentHostname, err := getHostname()
		if err != nil {
			logrus.Errorf("Failed to get hostname: %v", err)
		} else if currentHostname == "localhost" {
			// Restore the system hostname to what it was before, provided
			// we haven't got a new hostname from the DHCP server.
			if err = setHostname(previousHostname); err != nil {
				logrus.Errorf("Failed to restore hostname %s: %v", previousHostname, err)
			}
		}
	}

	return err
}

func upAllLinks() error {
	nics, err := getNICs()
	if err != nil {
		return err
	}

	for _, nic := range nics {
		if err := netlink.LinkSetUp(nic); err != nil {
			return err
		}
	}
	return nil
}

func getNICs() ([]netlink.Link, error) {
	var nics, vlanNics []netlink.Link

	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}

	for _, l := range links {
		if l.Type() == "device" && l.Attrs().EncapType != "loopback" {
			nics = append(nics, l)
		}
		if l.Type() == "vlan" {
			vlanNics = append(vlanNics, l)
		}
	}

	iscsi := goiscsi.NewLinuxISCSI(nil)
	sessions, err := iscsi.GetSessions()
	if err != nil {
		return nil, fmt.Errorf("error querying iscsi sessions: %v", err)
	}

	// no iscsi sessions detected so no additional filtering based on usage for iscsi device
	// access is needed and we can break here
	if len(sessions) == 0 {
		return nics, nil
	}

	return filterISCSIInterfaces(nics, vlanNics, sessions)
}

func getNICState(name string) int {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return NICStateNotFound
	}
	up := link.Attrs().RawFlags&unix.IFF_UP != 0
	lowerUp := link.Attrs().RawFlags&unix.IFF_LOWER_UP != 0
	if !up {
		return NICStateDown
	}
	if !lowerUp {
		return NICStateLowerDown
	}
	return NICStateUP
}

func getManagementInterfaceName(mgmtInterface config.Network) string {
	mgmtName := config.MgmtInterfaceName
	vlanID := mgmtInterface.VlanID
	if vlanID >= 2 && vlanID <= 4094 {
		mgmtName = fmt.Sprintf("%s.%d", mgmtName, vlanID)
	}
	return mgmtName
}

// filterISCSIInterfaces will query the host to identify iscsi sessions, and skip interfaces
// used by the existing iscsi session.
func filterISCSIInterfaces(nics, vlanNics []netlink.Link, sessions []goiscsi.ISCSISession) ([]netlink.Link, error) {
	hwDeviceMap := make(map[string]netlink.Link)
	for _, v := range nics {
		hwDeviceMap[v.Attrs().HardwareAddr.String()] = v
	}

	// temporary sessionMap to make it easy to correlate interface addressses with session ip address
	// should speed up identification of interfaces in use with iscsi sessions
	sessionMap := make(map[string]string)
	for _, session := range sessions {
		sessionMap[session.IfaceIPaddress] = ""
	}

	if err := filterNICSBySession(hwDeviceMap, nics, sessionMap); err != nil {
		return nil, err
	}

	if err := filterNICSBySession(hwDeviceMap, vlanNics, sessionMap); err != nil {
		return nil, err
	}
	logrus.Debugf("identified following iscsi sessions: %v", sessionMap)
	// we need to filter the filteredNics to also isolate parent nics if a vlan if is in use
	returnedNics := make([]netlink.Link, 0, len(hwDeviceMap))
	for _, v := range hwDeviceMap {
		returnedNics = append(returnedNics, v)
	}
	return returnedNics, nil
}

func filterNICSBySession(hwDeviceMap map[string]netlink.Link, links []netlink.Link, sessionMap map[string]string) error {
	for _, link := range links {
		logrus.Debugf("checking if link %s is in use", link.Attrs().Name)
		if getNICState(link.Attrs().Name) == NICStateUP {
			iface, err := net.InterfaceByName(link.Attrs().Name)
			if err != nil {
				return fmt.Errorf("error fetching interface details: %v", err)
			}

			addresses, err := iface.Addrs()
			if err != nil {
				return fmt.Errorf("error fetching addresses from interface: %v", err)
			}

			for _, address := range addresses {
				// interface addresses are in cidr format, and need to be converted before comparison
				// since iscsi session contains just the ip address
				ipAddress, _, err := net.ParseCIDR(address.String())
				if err != nil {
					return fmt.Errorf("error parsing ip address: %v", err)
				}
				if _, ok := sessionMap[ipAddress.String()]; ok {
					logrus.Debugf("filtering interface %s", link.Attrs().Name)
					delete(hwDeviceMap, link.Attrs().HardwareAddr.String())
					break //device is already removed, no point checking for other addresses
				}
			}
		}
	}
	return nil
}

// applyKernelIPv6 enables (enabled=true) or disables (enabled=false) IPv6 in the
// running kernel by writing the disable_ipv6 sysctls. Called at the cluster-CIDR
// confirm step so the live installer session matches the chosen networking mode.
// For dual-stack (enabled=true) any sysctl failure is returned as a hard error.
// For IPv4-only (enabled=false) failures are logged as warnings only.
func applyKernelIPv6(enabled bool) error {
	value := "0"
	if !enabled {
		value = "1"
	}
	for _, param := range []string{
		config.SysctlDisableIPv6All,
		config.SysctlDisableIPv6Default,
		config.SysctlDisableIPv6Lo,
	} {
		//nolint:gosec // G204: param and value are controlled internal strings
		if out, execErr := exec.Command("sysctl", "-w", fmt.Sprintf("%s=%s", param, value)).CombinedOutput(); execErr != nil {
			if enabled {
				return fmt.Errorf("failed to enable IPv6 (%s): %v (%s)", param, execErr, string(out))
			}
			logrus.Warnf("Failed to disable IPv6 sysctl %s: %v (%s)", param, execErr, string(out))
		}
	}
	return nil
}

// checkIPv6Address verifies that ifaceName has at least one non-link-local,
// non-loopback IPv6 unicast address assigned. It polls for up to maxWait,
// sleeping pollInterval between attempts, to allow time for SLAAC assignment.
// Returns an error if no qualifying address is found within the deadline.
func checkIPv6Address(ifaceName string, maxWait, pollInterval time.Duration) error {
	deadline := time.Now().Add(maxWait)
	for {
		iface, err := net.InterfaceByName(ifaceName)
		if err != nil {
			return fmt.Errorf("interface %s not found: %w", ifaceName, err)
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return fmt.Errorf("failed to list addresses on %s: %w", ifaceName, err)
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() != nil {
				continue // skip non-IPv6
			}
			if ip.IsLinkLocalUnicast() || ip.IsLoopback() {
				continue // skip link-local and loopback
			}
			logrus.Infof("IPv6 address found on %s: %s", ifaceName, ip)
			return nil
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("no non-link-local IPv6 address found on interface %s after %s; "+
		"ensure the network provides IPv6 via SLAAC or DHCPv6 before selecting dual-stack mode", ifaceName, maxWait)
}
