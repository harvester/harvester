package settings

import (
	"encoding/json"
	"fmt"
	"reflect"
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
	APIUIVersion                           = NewSetting("api-ui-version", "1.1.9") // Please update the HARVESTER_API_UI_VERSION in package/Dockerfile when updating the version here.
	ClusterRegistrationURL                 = NewSetting("cluster-registration-url", "")
	ServerVersion                          = NewSetting("server-version", "dev")
	UIIndex                                = NewSetting(UIIndexSettingName, DefaultDashboardUIURL)
	UIPath                                 = NewSetting(UIPathSettingName, "/usr/share/harvester/harvester")
	UISource                               = NewSetting(UISourceSettingName, "auto") // Options are 'auto', 'external' or 'bundled'
	UIPluginIndex                          = NewSetting(UIPluginIndexSettingName, DefaultUIPluginURL)
	VolumeSnapshotClass                    = NewSetting(VolumeSnapshotClassSettingName, "longhorn")
	BackupTargetSet                        = NewSetting(BackupTargetSettingName, "")
	UpgradableVersions                     = NewSetting("upgradable-versions", "")
	UpgradeCheckerEnabled                  = NewSetting("upgrade-checker-enabled", "true")
	UpgradeCheckerURL                      = NewSetting("upgrade-checker-url", "https://harvester-upgrade-responder.rancher.io/v1/checkupgrade")
	ReleaseDownloadURL                     = NewSetting("release-download-url", "https://releases.rancher.com/harvester")
	LogLevel                               = NewSetting("log-level", "info") // options are info, debug and trace
	SSLCertificates                        = NewSetting(SSLCertificatesSettingName, "{}")
	SSLParameters                          = NewSetting(SSLParametersName, "{}")
	SupportBundleImage                     = NewSetting(SupportBundleImageName, "{}")
	SupportBundleNamespaces                = NewSetting("support-bundle-namespaces", "")
	SupportBundleTimeout                   = NewSetting(SupportBundleTimeoutSettingName, "10")                                                                  // Unit is minute. 0 means disable timeout.
	SupportBundleExpiration                = NewSetting(SupportBundleExpirationSettingName, supportBundleUtil.SupportBundleExpirationDefaultStr)                // Unit is minute.
	SupportBundleNodeCollectionTimeout     = NewSetting(SupportBundleNodeCollectionTimeoutName, supportBundleUtil.SupportBundleNodeCollectionTimeoutDefaultStr) // Unit is minute.
	DefaultStorageClass                    = NewSetting("default-storage-class", "longhorn")
	HTTPProxy                              = NewSetting(HTTPProxySettingName, "{}")
	VMForceResetPolicySet                  = NewSetting(VMForceResetPolicySettingName, InitVMForceResetPolicy())
	OvercommitConfig                       = NewSetting(OvercommitConfigSettingName, `{"cpu":1600,"memory":150,"storage":200}`)
	VipPools                               = NewSetting(VipPoolsConfigSettingName, "")
	AutoDiskProvisionPaths                 = NewSetting("auto-disk-provision-paths", "")
	CSIDriverConfig                        = NewSetting(CSIDriverConfigSettingName, `{"driver.longhorn.io":{"volumeSnapshotClassName":"longhorn-snapshot","backupVolumeSnapshotClassName":"longhorn"}}`)
	ContainerdRegistry                     = NewSetting(ContainerdRegistrySettingName, "")
	StorageNetwork                         = NewSetting(StorageNetworkName, "")
	DefaultVMTerminationGracePeriodSeconds = NewSetting(DefaultVMTerminationGracePeriodSecondsSettingName, "120")
	AutoRotateRKE2CertsSet                 = NewSetting(AutoRotateRKE2CertsSettingName, InitAutoRotateRKE2Certs())
	KubeconfigTTL                          = NewSetting(KubeconfigDefaultTokenTTLMinutesSettingName, "0") // "0" is default value to ensure token does not expire
	LonghornV2DataEngineEnabled            = NewSetting(LonghornV2DataEngineSettingName, "false")
	// HarvesterCSICCMVersion this is the chart version from https://github.com/harvester/charts instead of image versions
	HarvesterCSICCMVersion = NewSetting(HarvesterCSICCMSettingName, `{"harvester-cloud-provider":">=0.0.1 <0.3.0","harvester-csi-provider":">=0.0.1 <0.3.0"}`)
	NTPServers             = NewSetting(NTPServersSettingName, "")
	WhiteListedSettings    = []string{"server-version", "default-storage-class", "harvester-csi-ccm-versions", "default-vm-termination-grace-period-seconds"}
	UpgradeConfigSet       = NewSetting(UpgradeConfigSettingName, `{"imagePreloadOption":{"strategy":{"type":"sequential"}}}`)
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
	DefaultDashboardUIURL                             = "https://releases.rancher.com/harvester-ui/dashboard/release-harvester-v1.4/index.html"
	SupportBundleImageName                            = "support-bundle-image"
	CSIDriverConfigSettingName                        = "csi-driver-config"
	UIIndexSettingName                                = "ui-index"
	UIPathSettingName                                 = "ui-path"
	UISourceSettingName                               = "ui-source"
	UIPluginIndexSettingName                          = "ui-plugin-index"
	UIPluginBundledVersionSettingName                 = "ui-plugin-bundled-version"
	DefaultUIPluginURL                                = "https://releases.rancher.com/harvester-ui/plugin/harvester-release-harvester-v1.4/harvester-release-harvester-v1.4.umd.min.js"
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
	return "HARVESTER_" + strings.ToUpper(strings.Replace(key, "-", "_", -1))
}

func IsRelease() bool {
	return !strings.Contains(ServerVersion.Get(), "head") && releasePattern.MatchString(ServerVersion.Get())
}

type TargetType string

const (
	S3BackupType  TargetType = "s3"
	NFSBackupType TargetType = "nfs"
)

type BackupTarget struct {
	Type               TargetType `json:"type"`
	Endpoint           string     `json:"endpoint"`
	AccessKeyID        string     `json:"accessKeyId"`
	SecretAccessKey    string     `json:"secretAccessKey"`
	BucketName         string     `json:"bucketName"`
	BucketRegion       string     `json:"bucketRegion"`
	Cert               string     `json:"cert"`
	VirtualHostedStyle bool       `json:"virtualHostedStyle"`
}

type VMForceResetPolicy struct {
	Enable bool `json:"enable"`
	// Period means how many seconds to wait for a node get back.
	Period int64 `json:"period"`
}

func InitBackupTargetToString() string {
	target := &BackupTarget{}
	targetStr, err := json.Marshal(target)
	if err != nil {
		logrus.Errorf("failed to init %s, error: %s", BackupTargetSettingName, err.Error())
	}
	return string(targetStr)
}

func DecodeBackupTarget(value string) (*BackupTarget, error) {
	target := &BackupTarget{}

	if value != "" {
		if err := json.Unmarshal([]byte(value), target); err != nil {
			return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, value)
		}
	}

	return target, nil
}

func (target *BackupTarget) IsDefaultBackupTarget() bool {
	if target == nil || target.Type != "" {
		return false
	}

	defaultTarget := &BackupTarget{}
	return reflect.DeepEqual(target, defaultTarget)
}

func InitVMForceResetPolicy() string {
	policy := &VMForceResetPolicy{
		Enable: true,
		Period: 5 * 60, // 5 minutes
	}
	policyStr, err := json.Marshal(policy)
	if err != nil {
		logrus.Errorf("failed to init %s, error: %s", VMForceResetPolicySettingName, err.Error())
	}
	return string(policyStr)
}

func DecodeVMForceResetPolicy(value string) (*VMForceResetPolicy, error) {
	policy := &VMForceResetPolicy{}
	if err := json.Unmarshal([]byte(value), policy); err != nil {
		return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, value)
	}

	if policy.Period <= 0 {
		return nil, fmt.Errorf("period value should be greater than 0, value: %d", policy.Period)
	}

	return policy, nil
}

type Overcommit struct {
	CPU     int `json:"cpu"`
	Memory  int `json:"memory"`
	Storage int `json:"storage"`
}

type SSLCertificate struct {
	CA                string `json:"ca"`
	PublicCertificate string `json:"publicCertificate"`
	PrivateKey        string `json:"privateKey"`
}

type SSLParameter struct {
	Protocols string `json:"protocols"`
	Ciphers   string `json:"ciphers"`
}

type CSIDriverInfo struct {
	VolumeSnapshotClassName       string `json:"volumeSnapshotClassName"`
	BackupVolumeSnapshotClassName string `json:"backupVolumeSnapshotClassName"`
}

type AutoRotateRKE2Certs struct {
	Enable          bool `json:"enable"`
	ExpiringInHours int  `json:"expiringInHours"`
}

func InitAutoRotateRKE2Certs() string {
	autoRotateRKE2Certs := &AutoRotateRKE2Certs{
		Enable:          false,
		ExpiringInHours: 240, // 7 days
	}
	autoRotateRKE2CertsStr, err := json.Marshal(autoRotateRKE2Certs)
	if err != nil {
		logrus.WithField("name", AutoRotateRKE2CertsSettingName).WithError(err).Error("failed to init setting")
	}
	return string(autoRotateRKE2CertsStr)
}

func GetCSIDriverInfo(provisioner string) (*CSIDriverInfo, error) {
	csiDriverConfig := make(map[string]*CSIDriverInfo)
	if err := json.Unmarshal([]byte(CSIDriverConfig.Get()), &csiDriverConfig); err != nil {
		return nil, err
	}
	csiDriverInfo, ok := csiDriverConfig[provisioner]
	if !ok {
		return nil, fmt.Errorf("can not find csi driver info for %s", provisioner)
	}
	return csiDriverInfo, nil
}

type StrategyType string

const (
	// Do no preloading
	SkipType StrategyType = "skip"

	// Preloading one node at a time
	SequentialType StrategyType = "sequential"

	// Preloading multiple nodes starts at the same time
	ParallelType StrategyType = "parallel"
)

type PreloadStrategy struct {
	Type StrategyType `json:"type,omitempty"`

	// Concurrency only takes effect when ParallelType is specified. Default to
	// 0, which means "full scale." Any value higher than the number of the
	// cluster nodes will be treated as 0; values lower than 0 will be rejected
	// by the validator.
	Concurrency int `json:"concurrency,omitempty"`
}

type ImagePreloadOption struct {
	// PreloadStrategy tweaks the way images are preloaded.
	Strategy PreloadStrategy `json:"strategy,omitempty"`
}

type UpgradeConfig struct {
	// Options for the Image Preload phase of Harvester Upgrade
	PreloadOption ImagePreloadOption `json:"imagePreloadOption,omitempty"`
}

func DecodeConfig[T any](value string) (*T, error) {
	target := new(T)

	if value != "" {
		if err := json.Unmarshal([]byte(value), target); err != nil {
			return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, value)
		}
	}

	return target, nil
}
