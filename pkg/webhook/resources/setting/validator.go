package setting

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/longhorn/backupstore"
	_ "github.com/longhorn/backupstore/nfs"
	_ "github.com/longhorn/backupstore/s3"
	"github.com/rancher/wrangler/pkg/slice"
	"golang.org/x/net/http/httpproxy"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/backup"
	settingctl "github.com/harvester/harvester/pkg/controller/master/setting"
	ctlv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	tlsutil "github.com/harvester/harvester/pkg/util/tls"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

var certs = getSystemCerts()

// See supported TLS protocols of ingress-nginx and Nginx.
// - https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/configmap/#ssl-protocols
// - http://nginx.org/en/docs/http/ngx_http_ssl_module.html#ssl_protocols
var supportedSSLProtocols = []string{"SSLv2", "SSLv3", "TLSv1", "TLSv1.1", "TLSv1.2", "TLSv1.3"}

type validateSettingFunc func(setting *v1beta1.Setting) error

var validateSettingFuncs = map[string]validateSettingFunc{
	settings.HttpProxySettingName:            validateHTTPProxy,
	settings.VMForceResetPolicySettingName:   validateVMForceResetPolicy,
	settings.SupportBundleTimeoutSettingName: validateSupportBundleTimeout,
	settings.OvercommitConfigSettingName:     validateOvercommitConfig,
	settings.VipPoolsConfigSettingName:       validateVipPoolsConfig,
	settings.SSLCertificatesSettingName:      validateSSLCertificates,
	settings.SSLParametersName:               validateSSLParameters,
}

func NewValidator(settingCache ctlv1beta1.SettingCache, vmBackupCache ctlv1beta1.VirtualMachineBackupCache) types.Validator {
	validator := &settingValidator{
		settingCache:  settingCache,
		vmBackupCache: vmBackupCache,
	}
	validateSettingFuncs[settings.BackupTargetSettingName] = validator.validateBackupTarget
	return validator
}

type settingValidator struct {
	types.DefaultValidator

	settingCache  ctlv1beta1.SettingCache
	vmBackupCache ctlv1beta1.VirtualMachineBackupCache
}

func (v *settingValidator) Resource() types.Resource {
	return types.Resource{
		Name:       v1beta1.SettingResourceName,
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.Setting{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *settingValidator) Create(request *types.Request, newObj runtime.Object) error {
	return validateSetting(newObj)
}

func (v *settingValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	return validateSetting(newObj)
}

func validateSetting(newObj runtime.Object) error {
	setting := newObj.(*v1beta1.Setting)

	if validateFunc, ok := validateSettingFuncs[setting.Name]; ok {
		return validateFunc(setting)
	}

	return nil
}

func validateHTTPProxy(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(setting.Value), &util.HTTPProxyConfig{}); err != nil {
		message := fmt.Sprintf("failed to unmarshal the setting value, %v", err)
		return werror.NewInvalidError(message, "value")
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
	if overcommit.Cpu < 100 {
		return emit(overcommit.Cpu, "cpu")
	}
	if overcommit.Memory < 100 {
		return emit(overcommit.Memory, "memory")
	}
	if overcommit.Storage < 100 {
		return emit(overcommit.Storage, "storage")
	}
	return nil
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

func (v *settingValidator) validateBackupTarget(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	target, err := settings.DecodeBackupTarget(setting.Value)
	if err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	// Since we remove S3 access key id and secret access key after S3 backup target has been verified,
	// we stop the controller to reconcile it again.
	if target.Type == settings.S3BackupType && (target.SecretAccessKey == "" || target.AccessKeyID == "") {
		return nil
	}

	if target.Type == settings.NFSBackupType && (target.BucketName != "" || target.BucketRegion != "") {
		return werror.NewInvalidError("NFS backup target should not have bucket name or region ", "value")
	}

	vmBackups, err := v.vmBackupCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return werror.NewInternalError(err.Error())
	}
	if hasVMBackupInCreatingOrDeletingProgress(vmBackups) {
		return werror.NewBadRequest("There is VMBackup in creating or deleting progress")
	}

	// Set OS environment variables for S3
	if target.Type == settings.S3BackupType {
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
	httpProxySetting, err := v.settingCache.Get(settings.HttpProxySettingName)
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
