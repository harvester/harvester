package setting

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	// Although we don't use following drivers directly, we need to import them to register drivers.
	// NFS Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/nfs/nfs.go#L47-L51
	// S3 Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/s3/s3.go#L33-L37
	"github.com/harvester/go-common/ds"
	"github.com/longhorn/backupstore"
	_ "github.com/longhorn/backupstore/nfs" //nolint
	_ "github.com/longhorn/backupstore/s3"  //nolint
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/rancher/lasso/pkg/log"
	mgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/slice"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/http/httpproxy"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	mapset "github.com/deckarep/golang-set/v2"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/containerd"
	nodectl "github.com/harvester/harvester/pkg/controller/master/node"
	settingctl "github.com/harvester/harvester/pkg/controller/master/setting"
	storagenetworkctl "github.com/harvester/harvester/pkg/controller/master/storagenetwork"
	ctlv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllhv1b2 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlnetworkv1 "github.com/harvester/harvester/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	tlsutil "github.com/harvester/harvester/pkg/util/tls"
	vmUtil "github.com/harvester/harvester/pkg/util/virtualmachine"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	mcmFeature                        = "multi-cluster-management"
	labelAppNameKey                   = "app.kubernetes.io/name"
	labelAppNameValuePrometheus       = "prometheus"
	labelAppNameValueAlertManager     = "alertmanager"
	labelAppNameValueGrafana          = "grafana"
	labelAppNameValueImportController = "harvester-vm-import-controller"
	maxTTLDurationMinutes             = 52560000 //specifies max duration allowed for kubeconfig TTL setting, and corresponds to 100 years
)

var certs = getSystemCerts()

// See supported TLS protocols of ingress-nginx and Nginx.
// - https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/configmap/#ssl-protocols
// - http://nginx.org/en/docs/http/ngx_http_ssl_module.html#ssl_protocols
var supportedSSLProtocols = []string{"SSLv2", "SSLv3", "TLSv1", "TLSv1.1", "TLSv1.2", "TLSv1.3"}

// Borrow from github.com/cilium/cilium
var FQDNMatchNameRegexString = `^([-a-zA-Z0-9_]+[.]?)+$`

var FQDNMatchPatternRegexString = `^([-a-zA-Z0-9_*]+[.]?)+$`

type validateSettingFunc func(setting *v1beta1.Setting) error

var validateSettingFuncs = map[string]validateSettingFunc{
	settings.VMForceResetPolicySettingName:                     validateVMForceResetPolicy,
	settings.SupportBundleImageName:                            validateSupportBundleImage,
	settings.SupportBundleTimeoutSettingName:                   validateSupportBundleTimeout,
	settings.SupportBundleExpirationSettingName:                validateSupportBundleExpiration,
	settings.SupportBundleNodeCollectionTimeoutName:            validateSupportBundleNodeCollectionTimeout,
	settings.OvercommitConfigSettingName:                       validateOvercommitConfig,
	settings.VipPoolsConfigSettingName:                         validateVipPoolsConfig,
	settings.SSLCertificatesSettingName:                        validateSSLCertificates,
	settings.SSLParametersName:                                 validateSSLParameters,
	settings.ContainerdRegistrySettingName:                     validateContainerdRegistry,
	settings.DefaultVMTerminationGracePeriodSecondsSettingName: validateDefaultVMTerminationGracePeriodSeconds,
	settings.NTPServersSettingName:                             validateNTPServers,
	settings.AutoRotateRKE2CertsSettingName:                    validateAutoRotateRKE2Certs,
	settings.KubeconfigDefaultTokenTTLMinutesSettingName:       validateKubeConfigTTLSetting,
	settings.AdditionalGuestMemoryOverheadRatioName:            validateAdditionalGuestMemoryOverheadRatio,
}

type validateSettingUpdateFunc func(oldSetting *v1beta1.Setting, newSetting *v1beta1.Setting) error

var validateSettingUpdateFuncs = map[string]validateSettingUpdateFunc{
	settings.VMForceResetPolicySettingName:                     validateUpdateVMForceResetPolicy,
	settings.SupportBundleImageName:                            validateUpdateSupportBundleImage,
	settings.SupportBundleTimeoutSettingName:                   validateUpdateSupportBundleTimeout,
	settings.SupportBundleExpirationSettingName:                validateUpdateSupportBundle,
	settings.SupportBundleNodeCollectionTimeoutName:            validateUpdateSupportBundleNodeCollectionTimeout,
	settings.OvercommitConfigSettingName:                       validateUpdateOvercommitConfig,
	settings.VipPoolsConfigSettingName:                         validateUpdateVipPoolsConfig,
	settings.SSLCertificatesSettingName:                        validateUpdateSSLCertificates,
	settings.SSLParametersName:                                 validateUpdateSSLParameters,
	settings.ContainerdRegistrySettingName:                     validateUpdateContainerdRegistry,
	settings.DefaultVMTerminationGracePeriodSecondsSettingName: validateUpdateDefaultVMTerminationGracePeriodSeconds,
	settings.NTPServersSettingName:                             validateUpdateNTPServers,
	settings.AutoRotateRKE2CertsSettingName:                    validateUpdateAutoRotateRKE2Certs,
	settings.KubeconfigDefaultTokenTTLMinutesSettingName:       validateUpdateKubeConfigTTLSetting,
	settings.AdditionalGuestMemoryOverheadRatioName:            validateUpdateAdditionalGuestMemoryOverheadRatio,
}

type validateSettingDeleteFunc func(setting *v1beta1.Setting) error

var validateSettingDeleteFuncs = make(map[string]validateSettingDeleteFunc)

func NewValidator(
	settingCache ctlv1beta1.SettingCache,
	nodeCache ctlcorev1.NodeCache,
	vmBackupCache ctlv1beta1.VirtualMachineBackupCache,
	snapshotClassCache ctlsnapshotv1.VolumeSnapshotClassCache,
	vmRestoreCache ctlv1beta1.VirtualMachineRestoreCache,
	vmCache ctlkubevirtv1.VirtualMachineCache,
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache,
	featureCache mgmtv3.FeatureCache,
	lhVolumeCache ctllhv1b2.VolumeCache,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	cnCache ctlnetworkv1.ClusterNetworkCache,
	vcCache ctlnetworkv1.VlanConfigCache,
	vsCache ctlnetworkv1.VlanStatusCache,
) types.Validator {
	validator := &settingValidator{
		settingCache:       settingCache,
		nodeCache:          nodeCache,
		vmBackupCache:      vmBackupCache,
		snapshotClassCache: snapshotClassCache,
		vmRestoreCache:     vmRestoreCache,
		vmCache:            vmCache,
		vmiCache:           vmiCache,
		featureCache:       featureCache,
		lhVolumeCache:      lhVolumeCache,
		pvcCache:           pvcCache,
		cnCache:            cnCache,
		vcCache:            vcCache,
		vsCache:            vsCache,
	}

	validateSettingFuncs[settings.BackupTargetSettingName] = validator.validateBackupTarget
	validateSettingFuncs[settings.VolumeSnapshotClassSettingName] = validator.validateVolumeSnapshotClass
	validateSettingUpdateFuncs[settings.BackupTargetSettingName] = validator.validateUpdateBackupTarget
	validateSettingUpdateFuncs[settings.VolumeSnapshotClassSettingName] = validator.validateUpdateVolumeSnapshotClass

	validateSettingFuncs[settings.StorageNetworkName] = validator.validateStorageNetwork
	validateSettingUpdateFuncs[settings.StorageNetworkName] = validator.validateUpdateStorageNetwork
	validateSettingDeleteFuncs[settings.StorageNetworkName] = validator.validateDeleteStorageNetwork

	validateSettingFuncs[settings.HTTPProxySettingName] = validator.validateHTTPProxy
	validateSettingUpdateFuncs[settings.HTTPProxySettingName] = validator.validateUpdateHTTPProxy

	validateSettingFuncs[settings.UpgradeConfigSettingName] = validator.validateUpgradeConfig
	validateSettingUpdateFuncs[settings.UpgradeConfigSettingName] = validator.validateUpdateUpgradeConfig

	return validator
}

type settingValidator struct {
	types.DefaultValidator

	settingCache       ctlv1beta1.SettingCache
	nodeCache          ctlcorev1.NodeCache
	vmBackupCache      ctlv1beta1.VirtualMachineBackupCache
	snapshotClassCache ctlsnapshotv1.VolumeSnapshotClassCache
	vmRestoreCache     ctlv1beta1.VirtualMachineRestoreCache
	vmCache            ctlkubevirtv1.VirtualMachineCache
	vmiCache           ctlkubevirtv1.VirtualMachineInstanceCache
	featureCache       mgmtv3.FeatureCache
	lhVolumeCache      ctllhv1b2.VolumeCache
	pvcCache           ctlcorev1.PersistentVolumeClaimCache
	cnCache            ctlnetworkv1.ClusterNetworkCache
	vcCache            ctlnetworkv1.VlanConfigCache
	vsCache            ctlnetworkv1.VlanStatusCache
}

func (v *settingValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.SettingResourceName},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.Setting{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}

func (v *settingValidator) Create(_ *types.Request, newObj runtime.Object) error {
	return validateSetting(newObj)
}

func (v *settingValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	return validateUpdateSetting(oldObj, newObj)
}

func (v *settingValidator) Delete(_ *types.Request, oldObj runtime.Object) error {
	return validateDeleteSetting(oldObj)
}

func validateSetting(newObj runtime.Object) error {
	setting := newObj.(*v1beta1.Setting)

	if validateFunc, ok := validateSettingFuncs[setting.Name]; ok {
		return validateFunc(setting)
	}

	return nil
}

func validateUpdateSetting(oldObj runtime.Object, newObj runtime.Object) error {
	oldSetting := oldObj.(*v1beta1.Setting)
	newSetting := newObj.(*v1beta1.Setting)

	if validateUpdateFunc, ok := validateSettingUpdateFuncs[newSetting.Name]; ok {
		return validateUpdateFunc(oldSetting, newSetting)
	}

	return nil
}

func validateDeleteSetting(oldObj runtime.Object) error {
	setting := oldObj.(*v1beta1.Setting)

	if validateDeleteFunc, ok := validateSettingDeleteFuncs[setting.Name]; ok {
		return validateDeleteFunc(setting)
	}

	return nil
}

func validateHTTPProxyHelper(value string, nodes []*corev1.Node) error {
	if value == "" {
		return nil
	}

	httpProxyConfig := &util.HTTPProxyConfig{}
	if err := json.Unmarshal([]byte(value), httpProxyConfig); err != nil {
		return err
	}

	// Make sure the node's IP addresses are set in `NoProxy` if `HTTPProxy`
	// or `HTTPSProxy` are configured. These IP addresses can be specified
	// individually or via CIDR address.
	if httpProxyConfig.HTTPProxy != "" || httpProxyConfig.HTTPSProxy != "" {
		err := validateNoProxy(httpProxyConfig.NoProxy, nodes)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *settingValidator) validateHTTPProxy(setting *v1beta1.Setting) error {
	nodes, err := v.nodeCache.List(labels.Everything())
	if err != nil {
		return werror.NewInternalError(err.Error())
	}

	if setting.Default != "{}" {
		if err := validateHTTPProxyHelper(setting.Default, nodes); err != nil {
			return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
		}
	}

	// Validate the value only if it is not the default value.
	if setting.Value != setting.Default {
		if err := validateHTTPProxyHelper(setting.Value, nodes); err != nil {
			return werror.NewInvalidError(err.Error(), settings.KeywordValue)
		}
	}
	return nil
}

func (v *settingValidator) validateUpdateHTTPProxy(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return v.validateHTTPProxy(newSetting)
}

// validateNoProxy Make sure the node's IP addresses is set in 'noProxy'.
// These IP addresses can be specified individually or via CIDR address.
// The function returns a `AdmitError` error if the node's IP addresses
// are not set in 'noProxy', otherwise `nil`.
func validateNoProxy(noProxy string, nodes []*corev1.Node) error {
	foundMap := make(map[string]bool)
	// 1. Get the list of node IP addresses.
	var nodeIPs []net.IP
	for _, node := range nodes {
		for _, nodeAddress := range node.Status.Addresses {
			if nodeAddress.Type == corev1.NodeInternalIP {
				nodeIP := net.ParseIP(nodeAddress.Address)
				if nodeIP != nil {
					nodeIPs = append(nodeIPs, nodeIP)
					foundMap[nodeIP.String()] = false
				}
			}
		}
	}
	// 2. Check if the node's IP addresses are set in 'noProxy'.
	noProxyParts := ds.SliceMapFunc(strings.Split(noProxy, ","),
		func(v string, _ int) string { return strings.TrimSpace(v) })
	for _, part := range noProxyParts {
		_, ipNet, err := net.ParseCIDR(part)
		if ipNet != nil && err == nil {
			for _, nodeIP := range nodeIPs {
				if ipNet.Contains(nodeIP) {
					foundMap[nodeIP.String()] = true
				}
			}
		} else {
			ip := net.ParseIP(part)
			if ip != nil {
				if slices.ContainsFunc(nodeIPs, func(v net.IP) bool { return v.Equal(ip) }) {
					foundMap[ip.String()] = true
				}
			}
		}
	}
	if slices.Index(ds.MapValues(foundMap), false) != -1 {
		missedNodes := ds.MapFilterFunc(foundMap, func(v bool, _ string) bool { return v == false })
		missedIPs := ds.MapKeys(missedNodes)
		slices.Sort(missedIPs)
		msg := fmt.Sprintf("noProxy should contain the node's IP addresses or CIDR. The node(s) %s are not covered.", strings.Join(missedIPs, ", "))
		return werror.NewInvalidError(msg, settings.KeywordValue)
	}

	return nil
}

func validateOvercommitConfigHelper(field, value string) error {
	if value == "" {
		return nil
	}
	overcommit := &settings.Overcommit{}
	if err := json.Unmarshal([]byte(value), overcommit); err != nil {
		return werror.NewInvalidError(fmt.Sprintf("Invalid JSON: %s", value), field)
	}
	emit := func(percentage int, field string) error {
		msg := fmt.Sprintf("Cannot undercommit. Should be greater than or equal to 100 but got %d", percentage)
		return werror.NewInvalidError(msg, field)
	}
	if overcommit.CPU < 100 {
		return emit(overcommit.CPU, "cpu")
	}
	if overcommit.Memory < 100 {
		return emit(overcommit.Memory, "memory")
	}
	if overcommit.Storage < 100 {
		return emit(overcommit.Storage, "storage")
	}
	return nil
}

func validateOvercommitConfig(setting *v1beta1.Setting) error {
	if err := validateOvercommitConfigHelper(settings.KeywordDefault, setting.Default); err != nil {
		return err
	}

	return validateOvercommitConfigHelper(settings.KeywordValue, setting.Value)
}

func validateUpdateOvercommitConfig(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateOvercommitConfig(newSetting)
}

func validateVMForceResetPolicyHelper(value string) error {
	if value == "" {
		return nil
	}

	if _, err := settings.DecodeVMForceResetPolicy(value); err != nil {
		return err
	}

	return nil
}

func validateVMForceResetPolicy(setting *v1beta1.Setting) error {
	if err := validateVMForceResetPolicyHelper(setting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := validateVMForceResetPolicyHelper(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}

	return nil
}

func validateUpdateVMForceResetPolicy(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateVMForceResetPolicy(newSetting)
}

// chech if this backup target is updated again by controller to strip secret information
func (v *settingValidator) isUpdatedS3BackupTarget(target *settings.BackupTarget) bool {
	if target.Type != settings.S3BackupType || target.SecretAccessKey != "" || target.AccessKeyID != "" {
		return false
	}

	if savedSetting, err := v.settingCache.Get(settings.BackupTargetSettingName); err != nil {
		return false
	} else if savedTarget, err := settings.DecodeBackupTarget(savedSetting.Value); err != nil {
		return false
	} else if savedTarget.Type != target.Type || savedTarget.BucketName != target.BucketName || savedTarget.BucketRegion != target.BucketRegion || savedTarget.Endpoint != target.Endpoint || savedTarget.VirtualHostedStyle != target.VirtualHostedStyle {
		// when any of those fields is different, then it is from user input, not from internal re-config
		return false
	}

	return true
}

// for each type of backup target, a well defined field composition is checked here
func (v *settingValidator) validateBackupTargetFields(target *settings.BackupTarget) error {
	switch target.Type {
	case settings.S3BackupType:
		if target.SecretAccessKey == "" || target.AccessKeyID == "" {
			return werror.NewInvalidError("S3 backup target should have access key and access key id", settings.KeywordValue)
		}

		if target.BucketName == "" || target.BucketRegion == "" {
			return werror.NewInvalidError("S3 backup target should have bucket name and region ", settings.KeywordValue)
		}

	case settings.NFSBackupType:
		if target.Endpoint == "" {
			return werror.NewInvalidError("NFS backup target should have endpoint", settings.KeywordValue)
		}

		if target.SecretAccessKey != "" || target.AccessKeyID != "" {
			return werror.NewInvalidError("NFS backup target should not have access key or access key id", settings.KeywordValue)
		}

		if target.BucketName != "" || target.BucketRegion != "" {
			return werror.NewInvalidError("NFS backup target should not have bucket name or region", settings.KeywordValue)
		}

	default:
		// do not check target.IsDefaultBackupTarget again, direct return error
		return werror.NewInvalidError("Invalid backup target type", settings.KeywordValue)
	}

	return nil
}

func (v *settingValidator) validateUpdateBackupTarget(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return v.validateBackupTarget(newSetting)
}

func validateBackupTargetHelper(setting *v1beta1.Setting) (*settings.BackupTarget, error) {
	var target *settings.BackupTarget

	if setting.Default != "" {
		defaultTarget, err := settings.DecodeBackupTarget(setting.Default)
		if err != nil {
			return nil, werror.NewInvalidError(err.Error(), settings.KeywordDefault)
		}
		target = defaultTarget
	}

	if setting.Value != "" {
		valueTarget, err := settings.DecodeBackupTarget(setting.Value)
		if err != nil {
			return nil, werror.NewInvalidError(err.Error(), settings.KeywordValue)
		}
		target = valueTarget
	}
	return target, nil
}

func (v *settingValidator) validateBackupTarget(setting *v1beta1.Setting) error {
	target, err := validateBackupTargetHelper(setting)
	if err != nil {
		return err
	}
	if target == nil {
		return nil
	}

	// when target is from internal re-update, allow fast pass
	if v.isUpdatedS3BackupTarget(target) {
		return nil
	}

	logrus.Debugf("validate backup target:%s:%s", target.Type, target.Endpoint)

	vmBackups, err := v.vmBackupCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("Can't list VM backups, err: %+v", err.Error()))
	}
	if hasVMBackupInCreatingOrDeletingProgress(vmBackups) {
		return werror.NewBadRequest("There is VMBackup in creating or deleting progress")
	}

	vmRestores, err := v.vmRestoreCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("Failed to get the VM restore objects, err: %+v", err.Error()))
	}
	if hasVMRestoreInCreatingOrDeletingProgress(vmRestores) {
		return werror.NewBadRequest("Can't update the backup target. There is a VM either in the process of creating or deleting")
	}

	// It is allowed to reset the current backup target setting to the default value
	// when it is default, the validator will skip all remaining checks
	if target.IsDefaultBackupTarget() {
		return nil
	}

	if err = v.validateBackupTargetFields(target); err != nil {
		return err
	}

	if target.Type == settings.S3BackupType {
		// Set OS environment variables for S3
		os.Setenv(util.AWSAccessKey, target.AccessKeyID)
		os.Setenv(util.AWSSecretKey, target.SecretAccessKey)
		os.Setenv(util.AWSEndpoints, target.Endpoint)
		if err := v.customizeTransport(); err != nil {
			return err
		}
	}

	// GetBackupStoreDriver tests whether the driver can List objects, so we don't need to do it again here.
	// S3: https://github.com/longhorn/backupstore/blob/56ddc538b85950b02c37432e4854e74f2647ca61/s3/s3.go#L38-L87
	// NFS: https://github.com/longhorn/backupstore/blob/56ddc538b85950b02c37432e4854e74f2647ca61/nfs/nfs.go#L46-L81
	endpoint := util.ConstructEndpoint(target)
	if _, err := backupstore.GetBackupStoreDriver(endpoint); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

// customizeTransport sets HTTP proxy and trusted CAs in the default HTTP transport.
// The transport used in the library is a one-time cache. Overwrite to update dynamically.
func (v *settingValidator) customizeTransport() error {
	httpProxySetting, err := v.settingCache.Get(settings.HTTPProxySettingName)
	if err != nil {
		return fmt.Errorf("failed to get HTTP proxy setting: %v", err)
	}
	var httpProxyConfig util.HTTPProxyConfig
	if httpProxySetting.Value != "" {
		if err := json.Unmarshal([]byte(httpProxySetting.Value), &httpProxyConfig); err != nil {
			return fmt.Errorf("failed to parse HTTP proxy config: %v", err)
		}
	}
	os.Setenv(util.HTTPProxyEnv, httpProxyConfig.HTTPProxy)
	os.Setenv(util.HTTPSProxyEnv, httpProxyConfig.HTTPSProxy)
	os.Setenv(util.NoProxyEnv, util.AddBuiltInNoProxy(httpProxyConfig.NoProxy))

	caSetting, err := v.settingCache.Get(settings.AdditionalCASettingName)
	if err != nil {
		return fmt.Errorf("failed to get additional CA setting: %v", err)
	}
	if caSetting.Value != "" {
		os.Setenv(util.AWSCERT, caSetting.Value)
		if ok := certs.AppendCertsFromPEM([]byte(caSetting.Value)); !ok {
			return fmt.Errorf("failed to append custom certificates: %v", caSetting.Value)
		}
	}

	customTransport, ok := http.DefaultTransport.(*http.Transport)
	if ok {
		customTransport.Proxy = func(request *http.Request) (*url.URL, error) {
			return httpproxy.FromEnvironment().ProxyFunc()(request.URL)
		}
		customTransport.TLSClientConfig = &tls.Config{
			RootCAs: certs,
		}
	}

	return nil
}

func validateNTPServersHelper(value string) error {
	if value == "" {
		return nil
	}

	logrus.Infof("Prepare to validate NTP servers: %s", value)
	ntpSettings := &util.NTPSettings{}
	if err := json.Unmarshal([]byte(value), ntpSettings); err != nil {
		return fmt.Errorf("failed to parse NTP settings: %v", err)
	}
	if ntpSettings.NTPServers == nil {
		return fmt.Errorf("NTP servers can't be nil")
	}
	logrus.Infof("ntpSettings: %+v", ntpSettings)

	fqdnNameValidator, err := regexp.Compile(FQDNMatchNameRegexString)
	if err != nil {
		return fmt.Errorf("Failed to compile fqdnName regexp: %v", err)
	}
	fqdnPatternValidator, err := regexp.Compile(FQDNMatchPatternRegexString)
	if err != nil {
		return fmt.Errorf("Failed to compile fqdnPattern regexp: %v", err)
	}
	startWithHTTP, err := regexp.Compile("^https?://.*")
	if err != nil {
		return fmt.Errorf("Failed to compile startWithHttp regexp: %v", err)
	}

	for _, server := range ntpSettings.NTPServers {
		if startWithHTTP.MatchString(server) {
			return fmt.Errorf("ntp server %s should not start with http:// or https://", server)
		}
		if !validateNTPServer(server, fqdnNameValidator, fqdnPatternValidator) {
			return fmt.Errorf("invalid NTP server: %s", server)
		}
	}

	duplicates := ds.SliceFindDuplicates(ntpSettings.NTPServers)
	if len(duplicates) > 0 {
		errMsg := fmt.Sprintf("duplicate NTP server: %v", duplicates)
		logrus.Errorf(errMsg)
		return fmt.Errorf(errMsg)
	}

	logrus.Infof("NTP servers validation passed")
	return nil
}

func validateNTPServers(setting *v1beta1.Setting) error {
	if err := validateNTPServersHelper(setting.Default); err != nil {
		log.Errorf(err.Error())
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := validateNTPServersHelper(setting.Value); err != nil {
		log.Errorf(err.Error())
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

func validateNTPServer(server string, nameValidator, patternValidator *regexp.Regexp) bool {
	if server == "" {
		logrus.Warnf("NTP server should not be empty, just skip")
		return true
	}

	// check IPv4/IPv6 format, or FQDN format
	if validatedIP := net.ParseIP(server); validatedIP == nil {
		if !nameValidator.MatchString(server) || !patternValidator.MatchString(server) {
			return false
		}
	}

	return true
}

func validateUpdateNTPServers(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateNTPServers(newSetting)
}

func validateVipPoolsConfigHelper(value string) error {
	if value == "" {
		return nil
	}

	pools := map[string]string{}
	err := json.Unmarshal([]byte(value), &pools)
	if err != nil {
		return err
	}

	return settingctl.ValidateCIDRs(pools)
}

func validateVipPoolsConfig(setting *v1beta1.Setting) error {
	if err := validateVipPoolsConfigHelper(setting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := validateVipPoolsConfigHelper(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

func validateUpdateVipPoolsConfig(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateVipPoolsConfig(newSetting)
}

func validateSupportBundleTimeoutHelper(value string) error {
	if value == "" {
		return nil
	}

	i, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	if i < 0 {
		return fmt.Errorf("timeout can't be negative")
	}
	return nil
}

func validateSupportBundleTimeout(setting *v1beta1.Setting) error {
	if err := validateSupportBundleTimeoutHelper(setting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := validateSupportBundleTimeoutHelper(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

func validateUpdateSupportBundleTimeout(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateSupportBundleTimeout(newSetting)
}

func validateSupportBundleExpirationHelper(value string) error {
	if value == "" {
		return nil
	}

	i, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	if i < 0 {
		return fmt.Errorf("expiration can't be negative")
	}
	return nil
}

func validateSupportBundleExpiration(setting *v1beta1.Setting) error {
	if err := validateSupportBundleExpirationHelper(setting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := validateSupportBundleExpirationHelper(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

func validateUpdateSupportBundleNodeCollectionTimeout(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateSupportBundleNodeCollectionTimeout(newSetting)
}

func validateSupportBundleNodeCollectionTimeoutHelper(value string) error {
	if value == "" {
		return nil
	}

	i, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	if i < 0 {
		return fmt.Errorf("node collection timeout can't be negative")
	}
	return nil
}

func validateSupportBundleNodeCollectionTimeout(setting *v1beta1.Setting) error {
	if err := validateSupportBundleNodeCollectionTimeoutHelper(setting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := validateSupportBundleNodeCollectionTimeoutHelper(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

func validateUpdateSupportBundle(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateSupportBundleExpiration(newSetting)
}

func validateSSLCertificatesHelper(field, value string) error {
	if value == "" {
		return nil
	}

	sslCertificate := &settings.SSLCertificate{}
	if err := json.Unmarshal([]byte(value), sslCertificate); err != nil {
		return werror.NewInvalidError(err.Error(), field)
	}

	if sslCertificate.CA == "" && sslCertificate.PublicCertificate == "" && sslCertificate.PrivateKey == "" {
		return nil
	}

	if sslCertificate.CA != "" {
		if err := tlsutil.ValidateCABundle([]byte(sslCertificate.CA)); err != nil {
			return werror.NewInvalidError(err.Error(), "ca")
		}
	}

	if err := tlsutil.ValidateServingBundle([]byte(sslCertificate.PublicCertificate)); err != nil {
		return werror.NewInvalidError(err.Error(), "publicCertificate")
	}

	if err := tlsutil.ValidatePrivateKey([]byte(sslCertificate.PrivateKey)); err != nil {
		return werror.NewInvalidError(err.Error(), "privateKey")
	}

	return nil
}

func validateSSLCertificates(setting *v1beta1.Setting) error {
	if err := validateSSLCertificatesHelper(settings.KeywordDefault, setting.Default); err != nil {
		return err
	}
	return validateSSLCertificatesHelper(settings.KeywordValue, setting.Value)
}

func validateUpdateSSLCertificates(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateSSLCertificates(newSetting)
}

func validateSSLParametersHelper(field, value string) error {
	if value == "" {
		return nil
	}

	sslParameter := &settings.SSLParameter{}
	if err := json.Unmarshal([]byte(value), sslParameter); err != nil {
		return werror.NewInvalidError(err.Error(), field)
	}

	if sslParameter.Protocols == "" && sslParameter.Ciphers == "" {
		return nil
	}

	if err := validateSSLProtocols(sslParameter); err != nil {
		return werror.NewInvalidError(err.Error(), "protocols")
	}
	return nil
}

func validateSSLParameters(setting *v1beta1.Setting) error {
	if err := validateSSLParametersHelper(settings.KeywordDefault, setting.Default); err != nil {
		return err
	}

	if err := validateSSLParametersHelper(settings.KeywordValue, setting.Value); err != nil {
		return err
	}

	// TODO: Validate ciphers
	// Currently, there's no easy way to actually tell what Ciphers are supported by rke2-ingress-nginx,
	// we need to tell users where to look for ciphers in docs.
	return nil
}

func validateUpdateSSLParameters(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateSSLParameters(newSetting)
}

func validateSSLProtocols(param *settings.SSLParameter) error {
	if param.Protocols == "" {
		return nil
	}

	for _, given := range strings.Split(param.Protocols, " ") {
		// Deal with multiple spaces between words e.g. "TLSv1.1    TLSv1.2"
		// ingress-nginx supports this so we also support it
		if len(given) == 0 {
			continue
		}

		if !slice.ContainsString(supportedSSLProtocols, given) {
			return fmt.Errorf("unsupported SSL protocol: %s", given)
		}
	}

	return nil
}

func getSystemCerts() *x509.CertPool {
	certs, _ := x509.SystemCertPool()
	if certs == nil {
		certs = x509.NewCertPool()
	}
	return certs
}

func hasVMBackupInCreatingOrDeletingProgress(vmBackups []*v1beta1.VirtualMachineBackup) bool {
	for _, vmBackup := range vmBackups {
		if vmBackup.DeletionTimestamp != nil || vmBackup.Status == nil || !*vmBackup.Status.ReadyToUse {
			return true
		}
	}
	return false
}

func hasVMRestoreInCreatingOrDeletingProgress(vmRestores []*v1beta1.VirtualMachineRestore) bool {
	for _, vmRestore := range vmRestores {
		if vmRestore.DeletionTimestamp != nil || vmRestore.Status == nil || !*vmRestore.Status.Complete {
			return true
		}
	}
	return false
}

func validateSupportBundleImageHelper(value string) error {
	if value == "" {
		return nil
	}

	var image settings.Image
	err := json.Unmarshal([]byte(value), &image)
	if err != nil {
		return err
	}
	if image.Repository == "" || image.Tag == "" || image.ImagePullPolicy == "" {
		return fmt.Errorf("image field can't be blank")
	}
	return nil
}

func validateSupportBundleImage(setting *v1beta1.Setting) error {
	if setting.Default != "{}" {
		if err := validateSupportBundleImageHelper(setting.Default); err != nil {
			return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
		}
	}
	if err := validateSupportBundleImageHelper(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}

	return nil
}

func validateUpdateSupportBundleImage(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateSupportBundleImage(newSetting)
}

func (v *settingValidator) validateVolumeSnapshotClass(setting *v1beta1.Setting) error {
	if setting.Default != "" {
		_, err := v.snapshotClassCache.Get(setting.Default)
		if err != nil {
			return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
		}
	}
	if setting.Value != "" {
		_, err := v.snapshotClassCache.Get(setting.Value)
		if err != nil {
			return werror.NewInvalidError(err.Error(), settings.KeywordValue)
		}
	}
	return nil
}

func (v *settingValidator) validateUpdateVolumeSnapshotClass(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return v.validateVolumeSnapshotClass(newSetting)
}

func validateContainerdRegistryHelper(value string) error {
	if value == "" {
		return nil
	}

	registry := &containerd.Registry{}

	return json.Unmarshal([]byte(value), registry)
}

func validateContainerdRegistry(setting *v1beta1.Setting) error {
	if err := validateContainerdRegistryHelper(setting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := validateContainerdRegistryHelper(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

func validateUpdateContainerdRegistry(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateContainerdRegistry(newSetting)
}

func (v *settingValidator) validateStorageNetworkHelper(value string) error {
	if value == "" {
		// harvester will create a default setting with empty value
		// storage-network will be the same in Longhorn, just skip check, don't require all VMs are stopped.
		return nil
	}

	var config storagenetworkctl.Config
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return fmt.Errorf("failed to unmarshal the setting value, %v", err)
	}

	if err := v.checkStorageNetworkVlanValid(&config); err != nil {
		return err
	}

	if err := v.checkStorageNetworkRangeValid(&config); err != nil {
		return err
	}

	if err := v.checkVlanStatusReady(&config); err != nil {
		return err
	}

	if err := v.checkVCSpansAllNodes(&config); err != nil {
		return err
	}

	return nil
}

func (v *settingValidator) validateStorageNetwork(setting *v1beta1.Setting) error {
	if setting.Name != settings.StorageNetworkName {
		return nil
	}

	if err := v.validateStorageNetworkHelper(setting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := v.validateStorageNetworkHelper(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}

	return v.checkStorageNetworkValueVaild()
}

func (v *settingValidator) validateUpdateStorageNetwork(oldSetting *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	if newSetting.Name != settings.StorageNetworkName {
		return nil
	}

	if oldSetting.Default == newSetting.Default && oldSetting.Value == newSetting.Value {
		return nil
	}

	if err := v.validateStorageNetworkHelper(newSetting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := v.validateStorageNetworkHelper(newSetting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}

	return v.checkStorageNetworkValueVaild()
}

func (v *settingValidator) validateDeleteStorageNetwork(_ *v1beta1.Setting) error {
	return werror.NewMethodNotAllowed(fmt.Sprintf("Disallow delete setting name %s", settings.StorageNetworkName))
}

func (v *settingValidator) findVolumesByLabelAppName(namespace string, labelAppNameValue string) ([]string, error) {
	volumes := make([]string, 0)
	sets := labels.Set{
		labelAppNameKey: labelAppNameValue,
	}

	pvcs, err := v.pvcCache.List(namespace, sets.AsSelector())
	if err != nil {
		return nil, err
	}

	for _, pvc := range pvcs {
		if pvc.Spec.VolumeName == "" {
			continue
		}
		logrus.Debugf("Get system volume %v", pvc.Spec.VolumeName)
		volumes = append(volumes, pvc.Spec.VolumeName)
	}

	return volumes, nil
}

type getVolumesFunc func() ([]string, error)

func (v *settingValidator) getPrometheusVolumes() ([]string, error) {
	return v.findVolumesByLabelAppName(util.CattleMonitoringSystemNamespace, labelAppNameValuePrometheus)
}

func (v *settingValidator) getAlertManagerVolumes() ([]string, error) {
	return v.findVolumesByLabelAppName(util.CattleMonitoringSystemNamespace, labelAppNameValueAlertManager)
}

func (v *settingValidator) getGrafanaVolumes() ([]string, error) {
	return v.findVolumesByLabelAppName(util.CattleMonitoringSystemNamespace, labelAppNameValueGrafana)
}

func (v *settingValidator) getVMImportControllerVolumes() ([]string, error) {
	return v.findVolumesByLabelAppName(util.HarvesterSystemNamespaceName, labelAppNameValueImportController)
}

func (v *settingValidator) getSystemVolumes() (map[string]struct{}, error) {
	getVolumeFuncs := []getVolumesFunc{
		v.getPrometheusVolumes,
		v.getAlertManagerVolumes,
		v.getGrafanaVolumes,
		v.getVMImportControllerVolumes,
	}

	systemVolumes := make(map[string]struct{})
	for _, f := range getVolumeFuncs {
		volumes, err := f()
		if err != nil {
			return nil, err
		}

		for _, volume := range volumes {
			systemVolumes[volume] = struct{}{}
		}
	}

	return systemVolumes, nil
}

func (v *settingValidator) getVClusterVolumes() (map[string]struct{}, error) {
	sets := labels.Set{
		util.LablelVClusterAppNameKey: util.LablelVClusterAppNameValue,
	}

	pvcs, err := v.pvcCache.List(util.VClusterNamespace, sets.AsSelector())
	if err != nil {
		return nil, err
	}

	vClusterVolumes := make(map[string]struct{})
	for _, pvc := range pvcs {
		if pvc.Spec.VolumeName == "" {
			continue
		}
		logrus.Debugf("Get vCluster volume %v", pvc.Spec.VolumeName)
		vClusterVolumes[pvc.Spec.VolumeName] = struct{}{}
	}

	return vClusterVolumes, nil
}

func (v *settingValidator) checkOnlineVolume() error {
	systemVolumes, err := v.getSystemVolumes()
	if err != nil {
		logrus.Errorf("getSystemVolumes err %v", err)
		return err
	}

	vClusterVolumes, err := v.getVClusterVolumes()
	if err != nil {
		logrus.Errorf("getVClusterVolumes err %v", err)
		return err
	}

	volumes, err := v.lhVolumeCache.List(util.LonghornSystemNamespaceName, labels.Everything())
	if err != nil {
		logrus.Errorf("list longhorn volumes err %v", err)
		return err
	}

	for _, volume := range volumes {
		if _, found := systemVolumes[volume.Name]; found {
			continue
		}
		if volume.Status.State == lhv1beta2.VolumeStateDetached {
			continue
		}
		if _, found := vClusterVolumes[volume.Name]; found {
			return fmt.Errorf("please stop vcluster before configuring the storage-network setting")
		}
		logrus.Errorf("volume %v in state %v", volume.Name, volume.Status.State)
		return fmt.Errorf("please stop all workloads before configuring the storage-network setting")
	}

	return nil
}

func (v *settingValidator) checkStorageNetworkValueVaild() error {
	// check all VM are stopped, there is no VMI
	vms, err := v.vmCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return err
	}

	for _, vm := range vms {
		isStopped, err := vmUtil.IsVMStopped(vm, v.vmiCache)
		if err != nil {
			return werror.NewInvalidError(err.Error(), settings.KeywordValue)
		}

		if !isStopped {
			return werror.NewInvalidError("Please stop all VMs before configuring the storage-network setting", settings.KeywordValue)
		}
	}

	if err := v.checkOnlineVolume(); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}

	return nil
}

func (v *settingValidator) checkVlanStatusReady(config *storagenetworkctl.Config) error {
	_, err := v.cnCache.Get(config.ClusterNetwork)
	if err != nil {
		return fmt.Errorf("cluster network %s not found because %v", config.ClusterNetwork, err)
	}

	vsList, err := v.vsCache.List(labels.Set{
		utils.KeyClusterNetworkLabel: config.ClusterNetwork,
	}.AsSelector())
	if err != nil {
		return err
	}

	if len(vsList) == 0 {
		return fmt.Errorf("vlan status not present for cluster network %s", config.ClusterNetwork)
	}

	for _, vs := range vsList {
		if networkv1.Ready.IsFalse(vs.Status) {
			return fmt.Errorf("vs %s status is not Ready", vs.Name)
		}
	}

	return nil
}

func getMatchNodes(vc *networkv1.VlanConfig) ([]string, error) {
	if vc.Annotations == nil || vc.Annotations[utils.KeyMatchedNodes] == "" {
		return nil, fmt.Errorf("vlan config annotations is absent for matched nodes")
	}

	var matchedNodes []string
	if err := json.Unmarshal([]byte(vc.Annotations[utils.KeyMatchedNodes]), &matchedNodes); err != nil {
		return nil, err
	}

	return matchedNodes, nil
}

func (v *settingValidator) checkVCSpansAllNodes(config *storagenetworkctl.Config) error {
	matchedNodes := mapset.NewSet[string]()

	vcs, err := v.vcCache.List(labels.Set{
		utils.KeyClusterNetworkLabel: config.ClusterNetwork,
	}.AsSelector())
	if err != nil {
		return err
	}

	if len(vcs) == 0 {
		return fmt.Errorf("vlan config not present for cluster network %s", config.ClusterNetwork)
	}

	for _, vc := range vcs {
		vnodes, err := getMatchNodes(vc)
		if err != nil {
			return err
		}
		matchedNodes.Append(vnodes...)
	}

	nodes, err := v.nodeCache.List(labels.Everything())
	if err != nil {
		return werror.NewInternalError(err.Error())
	}

	//check if vlanconfig contains all the nodes in the cluster
	for _, node := range nodes {
		//skip witness nodes which do not run LH Pods
		isManagement := nodectl.IsManagementRole(node)
		if nodectl.IsWitnessNode(node, isManagement) {
			continue
		}

		if !matchedNodes.Contains(node.Name) {
			return fmt.Errorf("vlanconfig does not span %s", node.Name)
		}
	}

	return nil
}

func (v *settingValidator) checkStorageNetworkVlanValid(config *storagenetworkctl.Config) error {
	if config.Vlan < 1 || config.Vlan > 4094 {
		return fmt.Errorf("The valid value range for VLAN IDs is 1 to 4094")
	}
	return nil
}

func (v *settingValidator) checkStorageNetworkRangeValid(config *storagenetworkctl.Config) error {
	ip, network, err := net.ParseCIDR(config.Range)
	if err != nil {
		return err
	}
	if !network.IP.Equal(ip) {
		return fmt.Errorf("Range should be subnet CIDR %v", network)
	}
	return nil
}

func validateDefaultVMTerminationGracePeriodSecondsHelper(value string) error {
	if value == "" {
		return nil
	}

	num, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return err
	}

	if num < 0 {
		return fmt.Errorf("can't be negative")
	}

	return nil
}

func validateDefaultVMTerminationGracePeriodSeconds(setting *v1beta1.Setting) error {
	if err := validateDefaultVMTerminationGracePeriodSecondsHelper(setting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := validateDefaultVMTerminationGracePeriodSecondsHelper(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

func validateUpdateDefaultVMTerminationGracePeriodSeconds(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateDefaultVMTerminationGracePeriodSeconds(newSetting)
}

func validateAutoRotateRKE2CertsHelper(value string) error {
	if value == "" {
		return nil
	}

	autoRotateRKE2Certs := &settings.AutoRotateRKE2Certs{}
	if err := json.Unmarshal([]byte(value), autoRotateRKE2Certs); err != nil {
		return err
	}

	if autoRotateRKE2Certs.ExpiringInHours <= 0 {
		return fmt.Errorf("expiringInHours can't be negative or zero")
	}

	largestExpiringInHours := 24*365 - 1
	if autoRotateRKE2Certs.ExpiringInHours > largestExpiringInHours {
		return fmt.Errorf("expiringInHours can't be large than %d", largestExpiringInHours)
	}

	return nil
}

func validateAutoRotateRKE2Certs(setting *v1beta1.Setting) error {
	if err := validateAutoRotateRKE2CertsHelper(setting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := validateAutoRotateRKE2CertsHelper(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

func validateUpdateAutoRotateRKE2Certs(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateAutoRotateRKE2Certs(newSetting)
}

func validateKubeConfigTTLSettingHelper(value string) error {
	if value == "" {
		return nil
	}

	num, err := strconv.Atoi(value)
	if err != nil {
		return err
	}

	if num < 0 {
		return fmt.Errorf("kubeconfig-default-token-ttl-minutes can't be negative")
	}

	if num > maxTTLDurationMinutes {
		return fmt.Errorf("kubeconfig-default-token-ttl-minutes exceeds 100 years")
	}
	return nil
}

func validateKubeConfigTTLSetting(newSetting *v1beta1.Setting) error {
	if err := validateKubeConfigTTLSettingHelper(newSetting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := validateKubeConfigTTLSettingHelper(newSetting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

func validateUpdateKubeConfigTTLSetting(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateKubeConfigTTLSetting(newSetting)
}

func validateUpgradeConfigHelper(setting *v1beta1.Setting) (*settings.UpgradeConfig, error) {
	var config *settings.UpgradeConfig

	if setting.Default != "" {
		defaultConfig, err := settings.DecodeConfig[settings.UpgradeConfig](setting.Default)
		if err != nil {
			return nil, werror.NewInvalidError(err.Error(), "default")
		}
		config = defaultConfig
	}

	if setting.Value != "" {
		valueConfig, err := settings.DecodeConfig[settings.UpgradeConfig](setting.Value)
		if err != nil {
			return nil, werror.NewInvalidError(err.Error(), "value")
		}
		config = valueConfig
	}
	return config, nil
}

func validateUpgradeConfigFields(setting *v1beta1.Setting, isSingleNode bool) error {
	upgradeConfig, err := validateUpgradeConfigHelper(setting)
	if err != nil {
		return err
	}

	if upgradeConfig == nil {
		return nil
	}

	strategyType := upgradeConfig.PreloadOption.Strategy.Type

	// Validate the image preload strategy type field
	switch strategyType {
	case settings.SkipType, settings.SequentialType, settings.ParallelType:
	default:
		return fmt.Errorf("invalid image preload strategy type: %s", strategyType)
	}

	// Validate the image preload strategy concurrency field
	concurrency := upgradeConfig.PreloadOption.Strategy.Concurrency
	if concurrency < 0 {
		return fmt.Errorf("invalid image preload concurrency: %d", concurrency)
	}

	// Validate the restore VM field
	if upgradeConfig.RestoreVM && !isSingleNode {
		return fmt.Errorf("restoreVM is only supported in single node cluster")
	}

	return nil
}

func (v *settingValidator) validateUpgradeConfig(setting *v1beta1.Setting) error {
	isSingleNode, err := v.isSingleNode()
	if err != nil {
		return err
	}
	return validateUpgradeConfigFields(setting, isSingleNode)
}

func (v *settingValidator) validateUpdateUpgradeConfig(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return v.validateUpgradeConfig(newSetting)
}

func validateAdditionalGuestMemoryOverheadRatio(newSetting *v1beta1.Setting) error {
	if err := settings.ValidateAdditionalGuestMemoryOverheadRatioHelper(newSetting.Default); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}

	if err := settings.ValidateAdditionalGuestMemoryOverheadRatioHelper(newSetting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

func validateUpdateAdditionalGuestMemoryOverheadRatio(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateAdditionalGuestMemoryOverheadRatio(newSetting)
}

func (v *settingValidator) isSingleNode() (bool, error) {
	nodes, err := v.nodeCache.List(labels.Everything())
	if err != nil {
		return false, err
	}
	return len(nodes) == 1, nil
}
