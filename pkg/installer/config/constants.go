package config

const (
	ModeCreate  = "create"
	ModeJoin    = "join"
	ModeUpgrade = "upgrade"
	ModeInstall = "install"

	RoleDefault = "default"
	RoleWitness = "witness"
	RoleMgmt    = "management"
	RoleWorker  = "worker"

	NetworkMethodDHCP   = "dhcp"
	NetworkMethodStatic = "static"
	NetworkMethodNone   = "none"

	MgmtInterfaceName     = "mgmt-br"
	MgmtBondInterfaceName = "mgmt-bo"

	RancherdConfigFile = "/etc/rancher/rancherd/config.yaml"

	DefaultCosOemSizeMiB      = 50
	DefaultCosStateSizeMiB    = 15360
	DefaultCosRecoverySizeMiB = 8192

	DefaultPersistentPercentageNum = 0.3
	PersistentSizeMinGiB           = 150

	SysctlDisableIPv6All     = "net.ipv6.conf.all.disable_ipv6"
	SysctlDisableIPv6Default = "net.ipv6.conf.default.disable_ipv6"
	SysctlDisableIPv6Lo      = "net.ipv6.conf.lo.disable_ipv6"

	// Default cluster network values used when the user leaves fields blank.
	// Dual-stack variants append the matching Harvester ULA IPv6 prefix.
	DefaultPodCIDR              = "10.52.0.0/16"
	DefaultServiceCIDR          = "10.53.0.0/16"
	DefaultClusterDNS           = "10.53.0.10"
	DefaultDualStackPodCIDR     = "10.52.0.0/16,fd52::/56"
	DefaultDualStackServiceCIDR = "10.53.0.0/16,fd53::/112"
	// DefaultDualStackClusterDNS pairs the IPv4 DNS address with its IPv6
	// equivalent in the fd53::/112 service prefix (host 0xa == 10).
	DefaultDualStackClusterDNS = "10.53.0.10,fd53::a"

	// IPFamilyIPv4 and IPFamilyIPv6 are the individual IP family identifiers,
	// matching the Kubernetes ipFamilies convention.
	// IPFamilies on the Install struct holds a slice of these values, e.g.
	// ["IPv4"] for single-stack or ["IPv4","IPv6"] for dual-stack IPv4-first.
	IPFamilyIPv4 = "IPv4"
	IPFamilyIPv6 = "IPv6"
)
