package console

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/rancher/wharfie/pkg/registries"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/harvester/harvester/pkg/installer/config"
	"github.com/harvester/harvester/pkg/installer/util"
)

const validTokenChars = "[a-zA-Z0-9 !\"#$%&'()*+,-./:;<=>?@^_`{|}~[\\]\\\\]"

var (
	persistentStateDirBlackList = []string{
		"/tmp",
	}
)

var (
	ErrMsgModeCreateContainsServerURL   = fmt.Sprintf("ServerURL need to be empty in %s mode", config.ModeCreate)
	ErrMsgModeJoinServerURLNotSpecified = fmt.Sprintf("ServerURL can't empty in %s mode", config.ModeJoin)
	ErrMsgModeUnknown                   = "unknown mode"
	ErrMsgTokenNotSpecified             = "token not specified"
	ErrMsgISOURLNotSpecified            = "iso_url is required in automatic installation"

	ErrMsgMgmtInterfaceNotSpecified    = "no management interface specified"
	ErrMsgMgmtInterfaceInvalidMethod   = "management network must configure with either static or DHCP method"
	ErrMsgMgmtInterfaceStaticNoDNS     = "DNS servers are required for static IP address"
	ErrMsgInterfaceNotSpecified        = "no interface specified"
	ErrMsgInterfaceNotSpecifiedForMgmt = "no interface specified for management network"
	ErrMsgInterfaceNotFound            = "interface not found"
	ErrMsgInterfaceIsLoop              = "interface is a loopback interface"
	ErrMsgDeviceNotSpecified           = "no device specified"
	ErrMsgDeviceNotFound               = "device not found"
	ErrMsgDeviceTooSmall               = fmt.Sprintf("device size too small. At least %dG is required", config.SingleDiskMinSizeGiB)
	ErrMsgNoCredentials                = "no SSH authorized keys or passwords are set"
	ErrMsgForceMBROnLargeDisk          = "disk size too large for MBR partitioning table"
	ErrMsgForceMBROnUEFI               = "cannot force MBR on UEFI system"

	ErrMsgNetworkMethodUnknown = "unknown network method"
	ErrMsgVipModeUnknown       = "unknown vip mode"

	ErrMsgSystemSettingsUnknown = "unknown system settings: %s"

	ErrMsgManagementInterfaceNotFound = "networks is deprecated, please use management_interface for new config and refer https://docs.harvesterhci.io/v1.1/install/harvester-configuration/#installmanagement_interface"
	ErrMsgUnsupportedSchemeVersion    = "Unsupported Harvester Scheme Version %d, please use new config and refer https://docs.harvesterhci.io/v1.1/install/harvester-configuration/"

	ErrContainerdRegistrySettingNotValidJSON = "could not parse containerd-registry as JSON"
)

type ValidatorInterface interface {
	Validate(cfg *config.HarvesterConfig) error
}

type ConfigValidator struct {
}

func prettyError(errMsg string, value string) error {
	return errors.Errorf("%s: %s", errMsg, value)
}

func checkInterface(iface config.NetworkInterface) error {

	err := iface.FindNetworkInterfaceName()
	if err != nil {
		return err
	}

	// For now we only accept interface name but not device specifier.
	if iface.Name == "" {
		return errors.New(ErrMsgInterfaceNotSpecified)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}

	checkFlags := func(flags net.Flags, name string) error {
		if flags&net.FlagLoopback != 0 {
			return prettyError(ErrMsgInterfaceIsLoop, name)
		}
		return nil
	}

	for _, i := range ifaces {
		if i.Name == iface.Name {
			return checkFlags(i.Flags, iface.Name)
		}
	}
	return prettyError(ErrMsgInterfaceNotFound, iface.Name)
}

func checkDevice(cfg *config.HarvesterConfig) error {
	installDisk := cfg.Install.Device
	dataDisk := cfg.Install.DataDisk

	if installDisk == "" {
		return errors.New(ErrMsgDeviceNotSpecified)
	}

	fileInfo, err := os.Lstat(installDisk)
	if err != nil {
		return err
	}

	targetDevice := installDisk
	// Support using path like `/dev/disks/by-id/xxx`
	if fileInfo.Mode()&fs.ModeSymlink != 0 {
		targetDevice, err = filepath.EvalSymlinks(installDisk)
		if err != nil {
			return err
		}
	}

	// Check against the latest refreshed disk options cache
	if err := diskOptionsCache.refresh(); err != nil {
		return err
	}
	options := diskOptionsCache.getAllValidDiskOptions()

	deviceFound := false
	for _, option := range options {
		if targetDevice == option.Value {
			deviceFound = true
			break
		}
	}
	if !deviceFound {
		return prettyError(ErrMsgDeviceNotFound, installDisk)
	}

	if cfg.SkipChecks {
		return nil
	}

	if cfg.Install.Role == config.RoleWitness {
		if err := validateDiskSize(installDisk, false); err != nil {
			return prettyError(ErrMsgDeviceTooSmall, installDisk)
		}
	} else if dataDisk == installDisk || dataDisk == "" {
		if err := validateDiskSize(installDisk, true); err != nil {
			return prettyError(ErrMsgDeviceTooSmall, installDisk)
		}
	} else {
		if err := validateDiskSize(installDisk, false); err != nil {
			return prettyError(ErrMsgDeviceTooSmall, installDisk)
		}
		if err := validateDataDiskSize(dataDisk); err != nil {
			return prettyError(ErrMsgDeviceTooSmall, dataDisk)
		}
	}

	return nil
}

func checkStaticRequiredString(field, value string) error {
	if len(value) == 0 {
		return fmt.Errorf("must specify %s in static method", field)
	}
	return nil
}

func checkDomain(domain string) error {
	if errs := validation.IsDNS1123Subdomain(domain); len(errs) > 0 {
		return fmt.Errorf("%s is not a valid domain", domain)
	}
	return nil
}

func checkIP(addr string) error {
	if ip := net.ParseIP(addr); ip == nil || ip.To4() == nil {
		return fmt.Errorf("%s is not a valid IP address", addr)
	}
	return nil
}

func checkMTU(mtu int) error {
	// Treat 0 as default value
	if mtu == 0 {
		return nil
	}
	// RFC 791
	if mtu < 576 || mtu > 9000 {
		return fmt.Errorf("%d is not a valid MTU value", mtu)
	}
	return nil
}

func checkHwAddr(hwAddr string) error {
	if _, err := net.ParseMAC(hwAddr); err != nil {
		return fmt.Errorf("%s is an invalid hardware address, error: %w", hwAddr, err)
	}

	return nil
}

func checkIPList(ipList []string) error {
	for _, ip := range ipList {
		if err := checkIP(ip); err != nil {
			return err
		}
	}
	return nil
}

func checkNetworks(network config.Network, dnsServers []string) error {
	if len(network.Interfaces) == 0 {
		return errors.New(ErrMsgInterfaceNotSpecifiedForMgmt)
	}
	method := network.Method
	if method != config.NetworkMethodDHCP && method != config.NetworkMethodStatic {
		return errors.New(ErrMsgMgmtInterfaceInvalidMethod)
	}
	if method == config.NetworkMethodStatic && len(dnsServers) == 0 {
		return errors.New(ErrMsgMgmtInterfaceStaticNoDNS)
	}

	for _, iface := range network.Interfaces {
		if err := checkInterface(iface); err != nil {
			return err
		}
	}

	// check VLAN ID in 0-4094 (0 is unset)
	if network.VlanID < 0 || network.VlanID > 4094 {
		return errors.New(ErrMsgVLANShouldBeANumberInRange)
	}

	switch network.Method {
	case config.NetworkMethodDHCP, config.NetworkMethodNone, "":
		return nil
	case config.NetworkMethodStatic:
		if err := checkStaticRequiredString("ip", network.IP); err != nil {
			return err
		}
		if err := checkIP(network.IP); err != nil {
			return err
		}
		if err := checkStaticRequiredString("subnetMask", network.SubnetMask); err != nil {
			return err
		}
		if err := checkIP(network.SubnetMask); err != nil {
			return err
		}
		if err := checkStaticRequiredString("gateway", network.Gateway); err != nil {
			return err
		}
		if err := checkIP(network.Gateway); err != nil {
			return err
		}
		if err := checkMTU(network.MTU); err != nil {
			return err
		}
	default:
		return prettyError(ErrMsgNetworkMethodUnknown, network.Method)
	}

	return nil
}

func checkVip(vip, vipHwAddr, vipMode string) error {
	if err := checkIP(vip); err != nil {
		return err
	}

	switch vipMode {
	case config.NetworkMethodDHCP:
		if err := checkHwAddr(vipHwAddr); err != nil {
			return err
		}
	case config.NetworkMethodStatic, config.NetworkMethodNone:
		return nil
	default:
		return prettyError(ErrMsgVipModeUnknown, vipMode)
	}

	return nil
}

func checkForceMBR(device string) error {
	diskTooLargeForMBR, err := diskExceedsMBRLimit(device)
	if err != nil {
		return err
	}
	if diskTooLargeForMBR {
		return prettyError(ErrMsgForceMBROnLargeDisk, device)
	}

	if !util.SystemIsBIOS() {
		return prettyError(ErrMsgForceMBROnUEFI, "UEFI")
	}

	return nil
}

func checkToken(token string) error {
	pattern := regexp.MustCompile("^" + validTokenChars + "+$")
	if !pattern.MatchString(token) {
		return errors.Errorf(
			"Invalid token. Must be alphanumeric and OWASP special password characters. Regexp: %v",
			pattern,
		)
	}

	return nil
}

func checkPersistentStatePath(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("Invalid path. PersistentStatePath must be an absolute path")
	}

	for _, d := range persistentStateDirBlackList {
		if path == d || strings.HasPrefix(path, d+string(os.PathSeparator)) {
			return errors.Errorf("Invalid path. Parent dir of PersistentStatePath cannot be %s", d)
		}
	}

	return nil
}

func checkPersistentStatePaths(persistentStatePaths []string) error {
	for _, path := range persistentStatePaths {
		if err := checkPersistentStatePath(path); err != nil {
			return err
		}
	}
	return nil
}

func checkSystemSettings(systemSettings map[string]string) error {
	if systemSettings == nil {
		return nil
	}

	allowList := config.GetSystemSettingsAllowList()
	for systemSetting, value := range systemSettings {
		isValid := false
		for _, allowSystemSetting := range allowList {
			if systemSetting == allowSystemSetting {
				isValid = true
				break
			}
		}

		if systemSetting == "containerd-registry" {
			var r registries.Registry
			if err := json.NewDecoder(strings.NewReader(value)).Decode(&r); err != nil {
				return errors.New(ErrContainerdRegistrySettingNotValidJSON)
			}
		}

		if !isValid {
			return errors.Errorf(ErrMsgSystemSettingsUnknown, systemSetting)
		}
	}
	return nil
}

func (v ConfigValidator) Validate(cfg *config.HarvesterConfig) error {
	if cfg.SchemeVersion != config.SchemeVersion {
		return fmt.Errorf(ErrMsgUnsupportedSchemeVersion, cfg.SchemeVersion)
	}

	// check hostname
	// ref: https://github.com/kubernetes/kubernetes/blob/b15f788d29df34337fedc4d75efe5580c191cbf3/pkg/apis/core/validation/validation.go#L242-L245
	if errs := validation.IsDNS1123Subdomain(cfg.OS.Hostname); len(errs) > 0 {
		// TODO: show regexp for validation to users
		return errors.Errorf("invalid hostname. A lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.'")
	}

	if err := diskChecks(cfg); err != nil {
		return err
	}

	if cfg.Install.Mode != config.ModeInstall {
		if len(cfg.Install.ManagementInterface.Interfaces) == 0 {
			return errors.Errorf("%s", ErrMsgManagementInterfaceNotFound)
		}

		if err := checkNetworks(cfg.Install.ManagementInterface, cfg.OS.DNSNameservers); err != nil {
			return err
		}

		if err := checkToken(cfg.Token); err != nil {
			return err
		}
	}

	if cfg.Install.Mode == config.ModeCreate {
		if err := checkVip(cfg.Vip, cfg.VipHwAddr, cfg.VipMode); err != nil {
			return err
		}
		if err := checkSystemSettings(cfg.SystemSettings); err != nil {
			return err
		}
	}

	if _, err := cfg.GetKubeletArgs(); err != nil {
		return err
	}

	return nil
}

func commonCheck(cfg *config.HarvesterConfig) error {
	// modes
	switch mode := cfg.Install.Mode; mode {
	case config.ModeUpgrade, config.ModeInstall:
		return nil
	case config.ModeCreate:
		if cfg.ServerURL != "" {
			return errors.New(ErrMsgModeCreateContainsServerURL)
		}
	case config.ModeJoin:
		if cfg.ServerURL == "" {
			return errors.New(ErrMsgModeJoinServerURLNotSpecified)
		}
	default:
		return prettyError(ErrMsgModeUnknown, mode)
	}

	if !alreadyInstalled && cfg.Install.Mode != config.ModeInstall && cfg.Install.Automatic && cfg.Install.ISOURL == "" {
		return errors.New(ErrMsgISOURLNotSpecified)
	}

	if cfg.Install.Mode != config.ModeInstall && cfg.Token == "" {
		return errors.New(ErrMsgTokenNotSpecified)
	}

	if len(cfg.SSHAuthorizedKeys) == 0 && cfg.Password == "" {
		return errors.New(ErrMsgNoCredentials)
	}

	return checkPersistentStatePaths(cfg.OS.PersistentStatePaths)
}

func validateConfig(v ValidatorInterface, cfg *config.HarvesterConfig) error {
	logrus.Debug("Validating config: ", cfg)
	if err := commonCheck(cfg); err != nil {
		return err
	}
	return v.Validate(cfg)
}

func diskChecks(cfg *config.HarvesterConfig) error {
	if err := checkDevice(cfg); err != nil {
		return err
	}

	if cfg.ForceMBR {
		if err := checkForceMBR(cfg.Install.Device); err != nil {
			return err
		}
	}

	return nil
}
