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
)
