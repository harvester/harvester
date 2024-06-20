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

	gocommon "github.com/harvester/go-common"
	"github.com/longhorn/backupstore"
	corev1 "k8s.io/api/core/v1"

	// Although we don't use following drivers directly, we need to import them to register drivers.
	// NFS Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/nfs/nfs.go#L47-L51
	// S3 Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/s3/s3.go#L33-L37
	_ "github.com/longhorn/backupstore/nfs" //nolint
	_ "github.com/longhorn/backupstore/s3"  //nolint
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	mgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/slice"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/http/httpproxy"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/containerd"
	"github.com/harvester/harvester/pkg/controller/master/backup"
	settingctl "github.com/harvester/harvester/pkg/controller/master/setting"
	storagenetworkctl "github.com/harvester/harvester/pkg/controller/master/storagenetwork"
	ctlv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllhv1b2 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
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
	settings.DefaultSnapshotMaxCountSettingName:                validateDefaultSnapshotMaxCount,
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
	settings.DefaultSnapshotMaxCountSettingName:                validateUpdateDefaultSnapshotMaxCount,
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

func (v *settingValidator) validateHTTPProxy(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	httpProxyConfig := &util.HTTPProxyConfig{}
	if err := json.Unmarshal([]byte(setting.Value), httpProxyConfig); err != nil {
		message := fmt.Sprintf("failed to unmarshal the setting value, %v", err)
		return werror.NewInvalidError(message, "value")
	}

	// Make sure the node's IP addresses is set in 'noProxy'. These IP
	// addresses can be specified individually or via CIDR address.
	nodes, err := v.nodeCache.List(labels.Everything())
	if err != nil {
		return werror.NewInvalidError(err.Error(), "")
	}
	err = validateNoProxy(httpProxyConfig.NoProxy, nodes)
	if err != nil {
		return err
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
	noProxyParts := gocommon.SliceMapFunc(strings.Split(noProxy, ","),
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
	if slices.Index(gocommon.MapValues(foundMap), false) != -1 {
		missedNodes := gocommon.MapFilterFunc(foundMap, func(v bool, _ string) bool { return v == false })
		missedIPs := gocommon.MapKeys(missedNodes)
		slices.Sort(missedIPs)
		msg := fmt.Sprintf("noProxy should contain the node's IP addresses or CIDR. The node(s) %s are not covered.", strings.Join(missedIPs, ", "))
		return werror.NewInvalidError(msg, "value")
	}

	return nil
}

func validateOvercommitConfig(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}
	overcommit := &settings.Overcommit{}
	if err := json.Unmarshal([]byte(setting.Value), overcommit); err != nil {
		return werror.NewInvalidError(fmt.Sprintf("Invalid JSON: %s", setting.Value), "Value")
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

func validateUpdateOvercommitConfig(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateOvercommitConfig(newSetting)
}

func validateVMForceResetPolicy(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	if _, err := settings.DecodeVMForceResetPolicy(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), "value")
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
			return werror.NewInvalidError("S3 backup target should have access key and access key id", "value")
		}

		if target.BucketName == "" || target.BucketRegion == "" {
			return werror.NewInvalidError("S3 backup target should have bucket name and region ", "value")
		}

	case settings.NFSBackupType:
		if target.Endpoint == "" {
			return werror.NewInvalidError("NFS backup target should have endpoint", "value")
		}

		if target.SecretAccessKey != "" || target.AccessKeyID != "" {
			return werror.NewInvalidError("NFS backup target should not have access key or access key id", "value")
		}

		if target.BucketName != "" || target.BucketRegion != "" {
			return werror.NewInvalidError("NFS backup target should not have bucket name or region", "value")
		}

	default:
		// do not check target.IsDefaultBackupTarget again, direct return error
		return werror.NewInvalidError("Invalid backup target type", "value")
	}

	return nil
}

func (v *settingValidator) validateUpdateBackupTarget(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return v.validateBackupTarget(newSetting)
}

func (v *settingValidator) validateBackupTarget(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	target, err := settings.DecodeBackupTarget(setting.Value)
	if err != nil {
		return werror.NewInvalidError(err.Error(), "value")
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
		os.Setenv(backup.AWSAccessKey, target.AccessKeyID)
		os.Setenv(backup.AWSSecretKey, target.SecretAccessKey)
		os.Setenv(backup.AWSEndpoints, target.Endpoint)
		if err := v.customizeTransport(); err != nil {
			return err
		}
	}

	// GetBackupStoreDriver tests whether the driver can List objects, so we don't need to do it again here.
	// S3: https://github.com/longhorn/backupstore/blob/56ddc538b85950b02c37432e4854e74f2647ca61/s3/s3.go#L38-L87
	// NFS: https://github.com/longhorn/backupstore/blob/56ddc538b85950b02c37432e4854e74f2647ca61/nfs/nfs.go#L46-L81
	endpoint := backup.ConstructEndpoint(target)
	if _, err := backupstore.GetBackupStoreDriver(endpoint); err != nil {
		return werror.NewInvalidError(err.Error(), "value")
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
		os.Setenv(backup.AWSCERT, caSetting.Value)
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

func validateNTPServers(setting *v1beta1.Setting) error {
	logrus.Infof("Prepare to validate NTP servers: %s", setting.Value)
	if setting.Value == "" {
		return nil
	}

	ntpSettings := &util.NTPSettings{}
	if err := json.Unmarshal([]byte(setting.Value), ntpSettings); err != nil {
		return fmt.Errorf("failed to parse NTP settings: %v", err)
	}
	if ntpSettings.NTPServers == nil {
		return fmt.Errorf("NTP servers can't be empty")
	}
	logrus.Infof("ntpSettings: %+v", ntpSettings)

	fqdnNameValidator, err := regexp.Compile(FQDNMatchNameRegexString)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to compile fqdnName regexp: %v", err)
		logrus.Errorf(errMsg)
		return fmt.Errorf(errMsg)
	}
	fqdnPatternValidator, err := regexp.Compile(FQDNMatchPatternRegexString)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to compile fqdnPattern regexp: %v", err)
		logrus.Errorf(errMsg)
		return fmt.Errorf(errMsg)
	}
	startWithHTTP, err := regexp.Compile("^http?://.*")
	if err != nil {
		errMsg := fmt.Sprintf("Failed to compile startWithHttp regexp: %v", err)
		logrus.Errorf(errMsg)
		return fmt.Errorf(errMsg)
	}

	for _, server := range ntpSettings.NTPServers {
		if startWithHTTP.MatchString(server) {
			errMsg := fmt.Sprintf("ntp server %s should not start with http:// or https:// .", server)
			logrus.Errorf(errMsg)
			return fmt.Errorf(errMsg)
		}
		if !validateNTPServer(server, fqdnNameValidator, fqdnPatternValidator) {
			errMsg := fmt.Sprintf("invalid NTP server: %s", server)
			logrus.Errorf(errMsg)
			return fmt.Errorf(errMsg)
		}
	}
	logrus.Infof("NTP servers validation passed")

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

func validateVipPoolsConfig(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	pools := map[string]string{}
	err := json.Unmarshal([]byte(setting.Value), &pools)
	if err != nil {
		return err
	}

	if err := settingctl.ValidateCIDRs(pools); err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	return nil
}

func validateUpdateVipPoolsConfig(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateVipPoolsConfig(newSetting)
}

func validateSupportBundleTimeout(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	i, err := strconv.Atoi(setting.Value)
	if err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}
	if i < 0 {
		return werror.NewInvalidError("timeout can't be negative", "value")
	}
	return nil
}

func validateUpdateSupportBundleTimeout(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateSupportBundleTimeout(newSetting)
}

func validateSupportBundleExpiration(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	i, err := strconv.Atoi(setting.Value)
	if err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}
	if i < 0 {
		return werror.NewInvalidError("expiration can't be negative", "value")
	}
	return nil
}

func validateUpdateSupportBundleNodeCollectionTimeout(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateSupportBundleNodeCollectionTimeout(newSetting)
}

func validateSupportBundleNodeCollectionTimeout(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	i, err := strconv.Atoi(setting.Value)
	if err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}
	if i < 0 {
		return werror.NewInvalidError("node collection timeout can't be negative", "value")
	}
	return nil
}

func validateUpdateSupportBundle(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateSupportBundleExpiration(newSetting)
}

func validateSSLCertificates(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	sslCertificate := &settings.SSLCertificate{}
	if err := json.Unmarshal([]byte(setting.Value), sslCertificate); err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	if sslCertificate.CA == "" && sslCertificate.PublicCertificate == "" && sslCertificate.PrivateKey == "" {
		return nil
	} else if sslCertificate.CA != "" {
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

func validateUpdateSSLCertificates(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateSSLCertificates(newSetting)
}

func validateSSLParameters(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	sslParameter := &settings.SSLParameter{}
	if err := json.Unmarshal([]byte(setting.Value), sslParameter); err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	if sslParameter.Protocols == "" && sslParameter.Ciphers == "" {
		return nil
	}

	if err := validateSSLProtocols(sslParameter); err != nil {
		return werror.NewInvalidError(err.Error(), "protocols")
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

func validateSupportBundleImage(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	var image settings.Image
	err := json.Unmarshal([]byte(setting.Value), &image)
	if err != nil {
		return err
	}
	if image.Repository == "" || image.Tag == "" || image.ImagePullPolicy == "" {
		return fmt.Errorf("image field can't be blank")
	}
	return nil
}

func validateUpdateSupportBundleImage(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateSupportBundleImage(newSetting)
}

func (v *settingValidator) validateVolumeSnapshotClass(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}
	_, err := v.snapshotClassCache.Get(setting.Value)
	return err
}

func (v *settingValidator) validateUpdateVolumeSnapshotClass(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return v.validateVolumeSnapshotClass(newSetting)
}

func validateContainerdRegistry(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	registry := &containerd.Registry{}
	if err := json.Unmarshal([]byte(setting.Value), registry); err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	return nil
}

func validateUpdateContainerdRegistry(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateContainerdRegistry(newSetting)
}

func (v *settingValidator) validateStorageNetwork(setting *v1beta1.Setting) error {
	if setting.Name != settings.StorageNetworkName {
		return nil
	}

	if setting.Value == "" {
		// harvester will create a default setting with empty value
		// storage-network will be the same in Longhorn, just skip check, don't require all VMs are stopped.
		return nil
	}

	return v.checkStorageNetworkValueVaild(setting)
}

func (v *settingValidator) validateUpdateStorageNetwork(oldSetting *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	if newSetting.Name != settings.StorageNetworkName {
		return nil
	}

	if oldSetting.Value == newSetting.Value {
		return nil
	}

	return v.checkStorageNetworkValueVaild(newSetting)
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

func (v *settingValidator) checkOnlineVolume() error {
	systemVolumes, err := v.getSystemVolumes()
	if err != nil {
		logrus.Errorf("getSystemVolumes err %v", err)
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

		if volume.Status.State != lhv1beta2.VolumeStateDetached {
			logrus.Errorf("volume %v in state %v", volume.Name, volume.Status.State)
			return fmt.Errorf("please stop all workloads before configuring the storage-network setting")
		}
	}

	return nil
}

func (v *settingValidator) checkStorageNetworkValueVaild(setting *v1beta1.Setting) error {
	// check JSON is valid
	if setting.Value != "" {
		var config storagenetworkctl.Config
		if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
			return werror.NewInvalidError(fmt.Sprintf("failed to unmarshal the setting value, %v", err), "value")
		}

		if err := v.checkStorageNetworkVlanValid(&config); err != nil {
			return werror.NewInvalidError(err.Error(), "value")
		}

		if err := v.checkStorageNetworkRangeValid(&config); err != nil {
			return werror.NewInvalidError(err.Error(), "value")
		}
	}

	// check all VM are stopped, there is no VMI
	vms, err := v.vmCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return err
	}

	for _, vm := range vms {
		isStopped, err := vmUtil.IsVMStopped(vm, v.vmiCache)
		if err != nil {
			return werror.NewInvalidError(err.Error(), "value")
		}

		if !isStopped {
			return werror.NewInvalidError("Please stop all VMs before configuring the storage-network setting", "value")
		}
	}

	if err := v.checkOnlineVolume(); err != nil {
		return werror.NewInvalidError(err.Error(), "value")
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

func validateDefaultVMTerminationGracePeriodSeconds(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	num, err := strconv.ParseInt(setting.Value, 10, 64)
	if err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	if num < 0 {
		return werror.NewInvalidError("can't be negative", "value")
	}

	return nil
}

func validateUpdateDefaultVMTerminationGracePeriodSeconds(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateDefaultVMTerminationGracePeriodSeconds(newSetting)
}

func validateAutoRotateRKE2Certs(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	autoRotateRKE2Certs := &settings.AutoRotateRKE2Certs{}
	if err := json.Unmarshal([]byte(setting.Value), autoRotateRKE2Certs); err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	if autoRotateRKE2Certs.ExpiringInHours <= 0 {
		return werror.NewInvalidError("expiringInHours can't be negative or zero", "value")
	}

	largestExpiringInHours := 24*365 - 1
	if autoRotateRKE2Certs.ExpiringInHours > largestExpiringInHours {
		return werror.NewInvalidError(fmt.Sprintf("expiringInHours can't be large than %d", largestExpiringInHours), "value")
	}

	return nil
}

func validateUpdateAutoRotateRKE2Certs(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateAutoRotateRKE2Certs(newSetting)
}

func validateKubeConfigTTLSetting(newSetting *v1beta1.Setting) error {
	if newSetting.Value == "" {
		return nil
	}

	num, err := strconv.Atoi(newSetting.Value)
	if err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	if num < 0 {
		return werror.NewInvalidError("kubeconfig-default-token-ttl-minutes cannot be negative", "value")
	}
	return nil
}

func validateUpdateKubeConfigTTLSetting(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateKubeConfigTTLSetting(newSetting)
}

func validateDefaultSnapshotMaxCount(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	num, err := strconv.ParseInt(setting.Value, 10, 64)
	if err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	if num < 2 || num > 250 {
		return werror.NewInvalidError("snapshot max count should be between 2 and 250", "value")
	}

	return nil
}

func validateUpdateDefaultSnapshotMaxCount(_ *v1beta1.Setting, newSetting *v1beta1.Setting) error {
	return validateDefaultSnapshotMaxCount(newSetting)
}
