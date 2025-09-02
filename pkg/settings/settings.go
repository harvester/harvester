package settings

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	supportBundleUtil "github.com/harvester/harvester/pkg/util/supportbundle"
)

var (
	releasePattern = regexp.MustCompile("^v[0-9]")
	settings       = map[string]Setting{}
	provider       Provider
	InjectDefaults string

	AdditionalCA                           = NewSetting(AdditionalCASettingName, "")
	APIUIVersion                           = NewSetting(APIUIVersionSettingName, "1.1.9") // Please update the HARVESTER_API_UI_VERSION in package/Dockerfile when updating the version here.
	ClusterRegistrationURL                 = NewSetting(ClusterRegistrationURLSettingName, "")
	ServerVersion                          = NewSetting(ServerVersionSettingName, "dev")
	UIIndex                                = NewSetting(UIIndexSettingName, DefaultDashboardUIURL)
	UIPath                                 = NewSetting(UIPathSettingName, "/usr/share/harvester/harvester")
	UISource                               = NewSetting(UISourceSettingName, "auto") // Options are 'auto', 'external' or 'bundled'
	UIPluginIndex                          = NewSetting(UIPluginIndexSettingName, DefaultUIPluginURL)
	VolumeSnapshotClass                    = NewSetting(VolumeSnapshotClassSettingName, "longhorn")
	BackupTargetSet                        = NewSetting(BackupTargetSettingName, "")
	UpgradableVersions                     = NewSetting(UpgradableVersionsSettingName, "")
	UpgradeCheckerEnabled                  = NewSetting(UpgradeCheckerEnabledSettingName, "true")
	UpgradeCheckerURL                      = NewSetting(UpgradeCheckerURLSettingName, "https://harvester-upgrade-responder.rancher.io/v1/checkupgrade")
	ReleaseDownloadURL                     = NewSetting(ReleaseDownloadURLSettingName, "https://releases.rancher.com/harvester")
	LogLevel                               = NewSetting(LogLevelSettingName, "info") // options are info, debug and trace
	SSLCertificates                        = NewSetting(SSLCertificatesSettingName, "{}")
	SSLParameters                          = NewSetting(SSLParametersName, "{}")
	SupportBundleImage                     = NewSetting(SupportBundleImageName, "{}")
	SupportBundleNamespaces                = NewSetting(SupportBundleNamespacesSettingName, "")
	SupportBundleTimeout                   = NewSetting(SupportBundleTimeoutSettingName, supportBundleUtil.SupportBundleTimeoutDefaultStr)                      // Unit is minute. 0 means disable timeout.
	SupportBundleExpiration                = NewSetting(SupportBundleExpirationSettingName, supportBundleUtil.SupportBundleExpirationDefaultStr)                // Unit is minute.
	SupportBundleNodeCollectionTimeout     = NewSetting(SupportBundleNodeCollectionTimeoutName, supportBundleUtil.SupportBundleNodeCollectionTimeoutDefaultStr) // Unit is minute.
	DefaultStorageClass                    = NewSetting(DefaultStorageClassSettingName, "longhorn")
	HTTPProxy                              = NewSetting(HTTPProxySettingName, "{}")
	VMForceResetPolicySet                  = NewSetting(VMForceResetPolicySettingName, InitVMForceResetPolicy())
	OvercommitConfig                       = NewSetting(OvercommitConfigSettingName, `{"cpu":1600,"memory":150,"storage":200}`)
	VipPools                               = NewSetting(VipPoolsConfigSettingName, "")
	AutoDiskProvisionPaths                 = NewSetting(AutoDiskProvisionPathsSettingName, "")
	CSIDriverConfig                        = NewSetting(CSIDriverConfigSettingName, `{"driver.longhorn.io":{"volumeSnapshotClassName":"longhorn-snapshot","backupVolumeSnapshotClassName":"longhorn"}}`)
	CSIOnlineExpandValidation              = NewSetting(CSIOnlineExpandValidationSettingName, `{"driver.longhorn.io":true}`)
	ContainerdRegistry                     = NewSetting(ContainerdRegistrySettingName, "")
	StorageNetwork                         = NewSetting(StorageNetworkName, "")
	DefaultVMTerminationGracePeriodSeconds = NewSetting(DefaultVMTerminationGracePeriodSecondsSettingName, "120")
	AutoRotateRKE2CertsSet                 = NewSetting(AutoRotateRKE2CertsSettingName, InitAutoRotateRKE2Certs())
	KubeconfigTTL                          = NewSetting(KubeconfigDefaultTokenTTLMinutesSettingName, "0") // "0" is default value to ensure token does not expire
	LonghornV2DataEngineEnabled            = NewSetting(LonghornV2DataEngineSettingName, "false")
	AdditionalGuestMemoryOverheadRatio     = NewSetting(AdditionalGuestMemoryOverheadRatioName, AdditionalGuestMemoryOverheadRatioDefault)
	RancherCluster                         = NewSetting(RancherClusterSettingName, "{}")
	// HarvesterCSICCMVersion this is the chart version from https://github.com/harvester/charts instead of image versions
	HarvesterCSICCMVersion = NewSetting(HarvesterCSICCMSettingName, `{"harvester-cloud-provider":">=0.0.1 <0.3.0","harvester-csi-provider":">=0.0.1 <0.3.0"}`)
	NTPServers             = NewSetting(NTPServersSettingName, "")
	WhiteListedSettings    = []string{ServerVersionSettingName, DefaultStorageClassSettingName, HarvesterCSICCMSettingName, DefaultVMTerminationGracePeriodSecondsSettingName}
	UpgradeConfigSet       = NewSetting(UpgradeConfigSettingName, `{"imagePreloadOption":{"strategy":{"type":"sequential"}}, "restoreVM": false}`)
	MaxHotplugRatio        = NewSetting(MaxHotplugRatioSettingName, "4")
	VMMigrationNetwork     = NewSetting(VMMigrationNetworkSettingName, "")
)

const (
	AdditionalCASettingName                           = "additional-ca"
	BackupTargetSettingName                           = "backup-target"
	VMForceResetPolicySettingName                     = "vm-force-reset-policy"
	SupportBundleTimeoutSettingName                   = "support-bundle-timeout"
	HTTPProxySettingName                              = "http-proxy"
	OvercommitConfigSettingName                       = "overcommit-config"
	SSLCertificatesSettingName                        = "ssl-certificates"
	SSLParametersName                                 = "ssl-parameters"
	VipPoolsConfigSettingName                         = "vip-pools"
	VolumeSnapshotClassSettingName                    = "volume-snapshot-class"
	DefaultDashboardUIURL                             = "https://releases.rancher.com/harvester-ui/dashboard/release-harvester-v1.6/index.html"
	SupportBundleImageName                            = "support-bundle-image"
	CSIDriverConfigSettingName                        = "csi-driver-config"
	CSIOnlineExpandValidationSettingName              = "csi-online-expand-validation"
	UIIndexSettingName                                = "ui-index"
	UIPathSettingName                                 = "ui-path"
	UISourceSettingName                               = "ui-source"
	UIPluginIndexSettingName                          = "ui-plugin-index"
	UIPluginBundledVersionSettingName                 = "ui-plugin-bundled-version"
	DefaultUIPluginURL                                = "https://releases.rancher.com/harvester-ui/plugin/harvester-release-harvester-v1.6/harvester-release-harvester-v1.6.umd.min.js"
	ContainerdRegistrySettingName                     = "containerd-registry"
	HarvesterCSICCMSettingName                        = "harvester-csi-ccm-versions"
	StorageNetworkName                                = "storage-network"
	DefaultVMTerminationGracePeriodSecondsSettingName = "default-vm-termination-grace-period-seconds"
	SupportBundleExpirationSettingName                = "support-bundle-expiration"
	NTPServersSettingName                             = "ntp-servers"
	AutoRotateRKE2CertsSettingName                    = "auto-rotate-rke2-certs"
	KubeconfigDefaultTokenTTLMinutesSettingName       = "kubeconfig-default-token-ttl-minutes"
	SupportBundleNodeCollectionTimeoutName            = "support-bundle-node-collection-timeout"
	UpgradeConfigSettingName                          = "upgrade-config"
	LonghornV2DataEngineSettingName                   = "longhorn-v2-data-engine-enabled"
	LogLevelSettingName                               = "log-level"
	AdditionalGuestMemoryOverheadRatioName            = "additional-guest-memory-overhead-ratio"
	ClusterRegistrationURLSettingName                 = "cluster-registration-url"
	AutoDiskProvisionPathsSettingName                 = "auto-disk-provision-paths"
	APIUIVersionSettingName                           = "api-ui-version"
	ServerVersionSettingName                          = "server-version"
	UpgradableVersionsSettingName                     = "upgradable-versions"
	UpgradeCheckerEnabledSettingName                  = "upgrade-checker-enabled"
	UpgradeCheckerURLSettingName                      = "upgrade-checker-url"
	ReleaseDownloadURLSettingName                     = "release-download-url"
	SupportBundleNamespacesSettingName                = "support-bundle-namespaces"
	DefaultStorageClassSettingName                    = "default-storage-class"
	MaxHotplugRatioSettingName                        = "max-hotplug-ratio"
	VMMigrationNetworkSettingName                     = "vm-migration-network"
	RancherClusterSettingName                         = "rancher-cluster"

	// settings have `default` and `value` string used in many places, replace them with const
	KeywordDefault = "default"
	KeywordValue   = "value"
)

func init() {
	if InjectDefaults == "" {
		return
	}
	defaults := map[string]string{}
	if err := json.Unmarshal([]byte(InjectDefaults), &defaults); err != nil {
		return
	}
	for name, defaultValue := range defaults {
		value, ok := settings[name]
		if !ok {
			continue
		}
		value.Default = defaultValue
		settings[name] = value
	}
}

type Provider interface {
	Get(name string) string
	Set(name, value string) error
	SetIfUnset(name, value string) error
	SetAll(settings map[string]Setting) error
}

type Setting struct {
	Name     string
	Default  string
	ReadOnly bool
}

func (s Setting) SetIfUnset(value string) error {
	if provider == nil {
		return s.Set(value)
	}
	return provider.SetIfUnset(s.Name, value)
}

func (s Setting) Set(value string) error {
	if provider == nil {
		s, ok := settings[s.Name]
		if ok {
			s.Default = value
			settings[s.Name] = s
		}
	} else {
		return provider.Set(s.Name, value)
	}
	return nil
}

func (s Setting) GetDefault() string {
	return s.Default
}

func (s Setting) Get() string {
	if provider == nil {
		s := settings[s.Name]
		return s.Default
	}
	return provider.Get(s.Name)
}

func (s Setting) GetInt() int {
	var (
		i   int
		err error
		v   = s.Get()
	)

	if v != "" {
		if i, err = strconv.Atoi(v); err == nil {
			return i
		}
		logrus.Errorf("failed to parse setting %s=%s as int: %v", s.Name, v, err)
	}

	if i, err = strconv.Atoi(s.Default); err != nil {
		return 0
	}

	return i
}

func SetProvider(p Provider) error {
	if err := p.SetAll(settings); err != nil {
		return err
	}
	provider = p
	return nil
}

func NewSetting(name, def string) Setting {
	s := Setting{
		Name:    name,
		Default: def,
	}
	settings[s.Name] = s
	return s
}

func GetEnvKey(key string) string {
	return "HARVESTER_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
}

func GetEnvDefaultValueKey(key string) string {
	return "HARVESTER_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_")) + "_DEFAULT_VALUE"
}

func IsRelease() bool {
	return !strings.Contains(ServerVersion.Get(), "head") && releasePattern.MatchString(ServerVersion.Get())
}

// move specific setting related things to settings_helper.go
