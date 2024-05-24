package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/blang/semver"
	"github.com/rancher/rke/cloudprovider"
	"github.com/rancher/rke/cloudprovider/aws"
	"github.com/rancher/rke/docker"
	"github.com/rancher/rke/k8s"
	"github.com/rancher/rke/log"
	"github.com/rancher/rke/metadata"
	"github.com/rancher/rke/services"
	"github.com/rancher/rke/templates"
	v3 "github.com/rancher/rke/types"
	"github.com/rancher/rke/util"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	apiserverv1 "k8s.io/apiserver/pkg/apis/apiserver/v1"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	eventratelimitapi "k8s.io/kubernetes/plugin/pkg/admission/eventratelimit/apis/eventratelimit"
	admissionapiv1 "k8s.io/pod-security-admission/admission/api/v1"
	admissionapiv1beta1 "k8s.io/pod-security-admission/admission/api/v1beta1"
)

const (
	DefaultServiceClusterIPRange = "10.43.0.0/16"
	DefaultNodePortRange         = "30000-32767"
	DefaultClusterCIDR           = "10.42.0.0/16"
	DefaultClusterDNSService     = "10.43.0.10"
	DefaultClusterDomain         = "cluster.local"
	DefaultClusterName           = "local"
	DefaultClusterSSHKeyPath     = "~/.ssh/id_rsa"

	DefaultSSHPort        = "22"
	DefaultDockerSockPath = "/var/run/docker.sock"

	DefaultAuthStrategy      = "x509"
	DefaultAuthorizationMode = "rbac"

	DefaultAuthnWebhookFile  = templates.AuthnWebhook
	DefaultAuthnCacheTimeout = "5s"

	DefaultNetworkPlugin        = "canal"
	DefaultNetworkCloudProvider = "none"

	DefaultIngressController             = "nginx"
	DefaultEtcdBackupCreationPeriod      = "12h"
	DefaultEtcdBackupRetentionPeriod     = "72h"
	DefaultEtcdSnapshot                  = true
	DefaultMonitoringProvider            = "metrics-server"
	DefaultEtcdBackupConfigIntervalHours = 12
	DefaultEtcdBackupConfigRetention     = 6
	DefaultEtcdBackupConfigTimeout       = docker.WaitTimeout

	DefaultDNSProvider = "kube-dns"
	K8sVersionCoreDNS  = "1.14.0"

	DefaultEtcdHeartbeatIntervalName  = "heartbeat-interval"
	DefaultEtcdHeartbeatIntervalValue = "500"
	DefaultEtcdElectionTimeoutName    = "election-timeout"
	DefaultEtcdElectionTimeoutValue   = "5000"

	DefaultFlannelBackendVxLan     = "vxlan"
	DefaultFlannelBackendVxLanPort = "8472"
	DefaultFlannelBackendVxLanVNI  = "1"

	DefaultCalicoFlexVolPluginDirectory = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/nodeagent~uds"

	DefaultCanalFlexVolPluginDirectory = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/nodeagent~uds"

	DefaultAciApicRefreshTime                        = "1200"
	DefaultAciOVSMemoryLimit                         = "1Gi"
	DefaultAciOVSMemoryRequest                       = "128Mi"
	DefaultAciImagePullPolicy                        = "Always"
	DefaultAciServiceMonitorInterval                 = "5"
	DefaultAciPBRTrackingNonSnat                     = "false"
	DefaultAciInstallIstio                           = "false"
	DefaultAciIstioProfile                           = "demo"
	DefaultAciDropLogEnable                          = "true"
	DefaultAciControllerLogLevel                     = "info"
	DefaultAciHostAgentLogLevel                      = "info"
	DefaultAciOpflexAgentLogLevel                    = "info"
	DefaultAciUseAciCniPriorityClass                 = "false"
	DefaultAciNoPriorityClass                        = "false"
	DefaultAciMaxNodesSvcGraph                       = "32"
	DefaultAciSnatContractScope                      = "global"
	DefaultAciSnatNamespace                          = "aci-containers-system"
	DefaultAciCApic                                  = "false"
	DefaultAciPodSubnetChunkSize                     = "32"
	DefaultAciSnatPortRangeStart                     = "5000"
	DefaultAciSnatPortRangeEnd                       = "65000"
	DefaultAciSnatPortsPerNode                       = "3000"
	DefaultAciUseHostNetnsVolume                     = "false"
	DefaultAciRunGbpContainer                        = "false"
	DefaultAciRunOpflexServerContainer               = "false"
	DefaultAciUseAciAnywhereCRD                      = "false"
	DefaultAciEnableEndpointSlice                    = "false"
	DefaultAciOpflexClientSSL                        = "true"
	DefaultAciUsePrivilegedContainer                 = "false"
	DefaultAciUseOpflexServerVolume                  = "false"
	DefaultAciDurationWaitForNetwork                 = "210"
	DefaultAciUseClusterRole                         = "true"
	DefaultAciDisableWaitForNetwork                  = "false"
	DefaultAciApicSubscriptionDelay                  = "0"
	DefaultAciApicRefreshTickerAdjust                = "0"
	DefaultAciDisablePeriodicSnatGlobalInfoSync      = "false"
	DefaultAciOpflexDeviceDeleteTimeout              = "0"
	DefaultAciMTUHeadRoom                            = "0"
	DefaultAciNodePodIfEnable                        = "false"
	DefaultAciSriovEnable                            = "false"
	DefaultAciMultusDisable                          = "true"
	DefaultAciNoWaitForServiceEpReadiness            = "false"
	DefaultAciAddExternalSubnetsToRdconfig           = "false"
	DefaultAciServiceGraphEndpointAddDelay           = "0"
	DefaultAciHppOptimization                        = "false"
	DefaultAciSleepTimeSnatGlobalInfoSync            = "0"
	DefaultAciOpflexAgentOpflexAsyncjsonEnabled      = "false"
	DefaultAciOpflexAgentOvsAsyncjsonEnabled         = "false"
	DefaultAciOpflexAgentPolicyRetryDelayTimer       = "10"
	DefaultAciAciMultipod                            = "false"
	DefaultAciAciMultipodUbuntu                      = "false"
	DefaultAciDhcpRenewMaxRetryCount                 = "0"
	DefaultAciDhcpDelay                              = "0"
	DefaultAciUseSystemNodePriorityClass             = "false"
	DefaultAciAciContainersMemoryLimit               = "3Gi"
	DefaultAciAciContainersMemoryRequest             = "128Mi"
	DefaultAciOpflexAgentStatistics                  = "true"
	DefaultAciAddExternalContractToDefaultEpg        = "false"
	DefaultAciEnableOpflexAgentReconnect             = "false"
	DefaultAciOpflexOpensslCompat                    = "false"
	DefaultAciTolerationSeconds                      = "600"
	DefaultAciDisableHppRendering                    = "false"
	DefaultAciApicConnectionRetryLimit               = "5"
	DefaultAciTaintNotReadyNode                      = "false"
	DefaultAciDropLogDisableEvents                   = "false"
	KubeAPIArgAdmissionControlConfigFile             = "admission-control-config-file"
	DefaultKubeAPIArgAdmissionControlConfigFileValue = "/etc/kubernetes/admission.yaml"

	EventRateLimitPluginName = "EventRateLimit"
	PodSecurityPluginName    = "PodSecurity"
	PodSecurityPrivileged    = "privileged"
	PodSecurityRestricted    = "restricted"

	KubeAPIArgAuditLogPath                = "audit-log-path"
	KubeAPIArgAuditLogMaxAge              = "audit-log-maxage"
	KubeAPIArgAuditLogMaxBackup           = "audit-log-maxbackup"
	KubeAPIArgAuditLogMaxSize             = "audit-log-maxsize"
	KubeAPIArgAuditLogFormat              = "audit-log-format"
	KubeAPIArgAuditPolicyFile             = "audit-policy-file"
	DefaultKubeAPIArgAuditLogPathValue    = "/var/log/kube-audit/audit-log.json"
	DefaultKubeAPIArgAuditPolicyFileValue = "/etc/kubernetes/audit-policy.yaml"

	DefaultMaxUnavailableWorker       = "10%"
	DefaultMaxUnavailableControlplane = "1"
	DefaultNodeDrainTimeout           = 120
	DefaultNodeDrainGracePeriod       = -1
	DefaultHTTPPort                   = 80
	DefaultHTTPSPort                  = 443
	DefaultNetworkMode                = "hostNetwork"
	DefaultNetworkModeV121            = "hostPort"
)

var (
	DefaultNodeDrainIgnoreDaemonsets      = true
	DefaultDaemonSetMaxUnavailable        = intstr.FromInt(1)
	DefaultDeploymentUpdateStrategyParams = intstr.FromString("25%")
	DefaultDaemonSetUpdateStrategy        = v3.DaemonSetUpdateStrategy{
		Strategy:      appsv1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDaemonSet{MaxUnavailable: &DefaultDaemonSetMaxUnavailable},
	}
	DefaultDeploymentUpdateStrategy = v3.DeploymentStrategy{
		Strategy: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &DefaultDeploymentUpdateStrategyParams,
			MaxSurge:       &DefaultDeploymentUpdateStrategyParams,
		},
	}
	DefaultClusterProportionalAutoscalerLinearParams = v3.LinearAutoscalerParams{CoresPerReplica: 128, NodesPerReplica: 4, Min: 1, PreventSinglePointFailure: true}
	DefaultMonitoringAddonReplicas                   = int32(1)
	defaultUseInstanceMetadataHostname               = false
)

type ExternalFlags struct {
	CertificateDir   string
	ClusterFilePath  string
	DinD             bool
	ConfigDir        string
	CustomCerts      bool
	DisablePortCheck bool
	GenerateCSR      bool
	Local            bool
	UpdateOnly       bool
	UseLocalState    bool
}

func setDefaultIfEmptyMapValue(configMap map[string]string, key string, value string) {
	if _, ok := configMap[key]; !ok {
		configMap[key] = value
	}
}

func setDefaultIfEmpty(varName *string, defaultValue string) {
	if len(*varName) == 0 {
		*varName = defaultValue
	}
}

func (c *Cluster) setClusterDefaults(ctx context.Context, flags ExternalFlags) error {
	if len(c.SSHKeyPath) == 0 {
		c.SSHKeyPath = DefaultClusterSSHKeyPath
	}
	// Default Path prefix
	if len(c.PrefixPath) == 0 {
		c.PrefixPath = "/"
	}
	// Set bastion/jump host defaults
	if len(c.BastionHost.Address) > 0 {
		if len(c.BastionHost.Port) == 0 {
			c.BastionHost.Port = DefaultSSHPort
		}
		if len(c.BastionHost.SSHKeyPath) == 0 {
			c.BastionHost.SSHKeyPath = c.SSHKeyPath
		}
		c.BastionHost.SSHAgentAuth = c.SSHAgentAuth

	}
	for i, host := range c.Nodes {
		if len(host.InternalAddress) == 0 {
			c.Nodes[i].InternalAddress = c.Nodes[i].Address
		}
		if len(host.HostnameOverride) == 0 {
			// This is a temporary modification
			c.Nodes[i].HostnameOverride = c.Nodes[i].Address
		}
		if len(host.SSHKeyPath) == 0 {
			c.Nodes[i].SSHKeyPath = c.SSHKeyPath
		}
		if len(host.Port) == 0 {
			c.Nodes[i].Port = DefaultSSHPort
		}

		c.Nodes[i].HostnameOverride = strings.ToLower(c.Nodes[i].HostnameOverride)
		// For now, you can set at the global level only.
		c.Nodes[i].SSHAgentAuth = c.SSHAgentAuth
	}

	if len(c.Authorization.Mode) == 0 {
		c.Authorization.Mode = DefaultAuthorizationMode
	}
	if c.Services.KubeAPI.PodSecurityPolicy && c.Authorization.Mode != services.RBACAuthorizationMode {
		log.Warnf(ctx, "PodSecurityPolicy can't be enabled with RBAC support disabled")
		c.Services.KubeAPI.PodSecurityPolicy = false
	}
	if len(c.Ingress.Provider) == 0 {
		c.Ingress.Provider = DefaultIngressController
	}
	if len(c.ClusterName) == 0 {
		c.ClusterName = DefaultClusterName
	}
	if len(c.Version) == 0 {
		c.Version = metadata.DefaultK8sVersion
	}
	if c.AddonJobTimeout == 0 {
		c.AddonJobTimeout = k8s.DefaultTimeout
	}
	if len(c.Monitoring.Provider) == 0 {
		c.Monitoring.Provider = DefaultMonitoringProvider
	}
	// set docker private registry URL
	for _, pr := range c.PrivateRegistries {
		if pr.URL == "" {
			pr.URL = docker.DockerRegistryURL
		}
		c.PrivateRegistriesMap[pr.URL] = pr
	}

	err := c.setClusterImageDefaults()
	if err != nil {
		return err
	}

	if c.RancherKubernetesEngineConfig.RotateCertificates != nil ||
		flags.CustomCerts {
		c.ForceDeployCerts = true
	}

	if c.CloudProvider.Name == k8s.AWSCloudProvider && c.CloudProvider.UseInstanceMetadataHostname == nil {
		c.CloudProvider.UseInstanceMetadataHostname = &defaultUseInstanceMetadataHostname
	}

	// enable cri-dockerd for k8s >= 1.24
	err = c.setCRIDockerd()
	if err != nil {
		return err
	}

	err = c.setClusterDNSDefaults()
	if err != nil {
		return err
	}
	c.setClusterServicesDefaults()
	c.setClusterNetworkDefaults()
	c.setClusterAuthnDefaults()
	c.setNodeUpgradeStrategy()
	c.setAddonsDefaults()
	return nil
}

func (c *Cluster) setNodeUpgradeStrategy() {
	if c.UpgradeStrategy == nil {
		logrus.Debugf("No input provided for maxUnavailableWorker, setting it to default value of %v percent", strings.TrimRight(DefaultMaxUnavailableWorker, "%"))
		logrus.Debugf("No input provided for maxUnavailableControlplane, setting it to default value of %v", DefaultMaxUnavailableControlplane)
		c.UpgradeStrategy = &v3.NodeUpgradeStrategy{
			MaxUnavailableWorker:       DefaultMaxUnavailableWorker,
			MaxUnavailableControlplane: DefaultMaxUnavailableControlplane,
		}
		return
	}
	setDefaultIfEmpty(&c.UpgradeStrategy.MaxUnavailableWorker, DefaultMaxUnavailableWorker)
	setDefaultIfEmpty(&c.UpgradeStrategy.MaxUnavailableControlplane, DefaultMaxUnavailableControlplane)
	if c.UpgradeStrategy.Drain != nil && *c.UpgradeStrategy.Drain {
		return
	}
	if c.UpgradeStrategy.DrainInput == nil {
		c.UpgradeStrategy.DrainInput = &v3.NodeDrainInput{
			IgnoreDaemonSets: &DefaultNodeDrainIgnoreDaemonsets,
			// default to 120 seems to work better for controlplane nodes
			Timeout: DefaultNodeDrainTimeout,
			// Period of time in seconds given to each pod to terminate gracefully.
			// If negative, the default value specified in the pod will be used
			GracePeriod: DefaultNodeDrainGracePeriod,
		}
	}
}

// setCRIDockerd set enable_cri_dockerd = true when the following two conditions are met:
// the cluster's version is at least 1.24 and the option enable_cri_dockerd is not configured
func (c *Cluster) setCRIDockerd() error {
	parsedVersion, err := getClusterVersion(c.Version)
	if err != nil {
		return err
	}
	if parsedRangeAtLeast124(parsedVersion) {
		if c.EnableCRIDockerd == nil {
			enable := true
			c.EnableCRIDockerd = &enable
		}
	}
	return nil
}

func (c *Cluster) setClusterServicesDefaults() {
	// We don't accept per service images anymore.
	c.Services.KubeAPI.Image = c.SystemImages.Kubernetes
	c.Services.Scheduler.Image = c.SystemImages.Kubernetes
	c.Services.KubeController.Image = c.SystemImages.Kubernetes
	c.Services.Kubelet.Image = c.SystemImages.Kubernetes
	c.Services.Kubeproxy.Image = c.SystemImages.Kubernetes
	c.Services.Etcd.Image = c.SystemImages.Etcd

	// enable etcd snapshots by default
	if c.Services.Etcd.Snapshot == nil {
		defaultSnapshot := DefaultEtcdSnapshot
		c.Services.Etcd.Snapshot = &defaultSnapshot
	}

	serviceConfigDefaultsMap := map[*string]string{
		&c.Services.KubeAPI.ServiceClusterIPRange:        DefaultServiceClusterIPRange,
		&c.Services.KubeAPI.ServiceNodePortRange:         DefaultNodePortRange,
		&c.Services.KubeController.ServiceClusterIPRange: DefaultServiceClusterIPRange,
		&c.Services.KubeController.ClusterCIDR:           DefaultClusterCIDR,
		&c.Services.Kubelet.ClusterDNSServer:             DefaultClusterDNSService,
		&c.Services.Kubelet.ClusterDomain:                DefaultClusterDomain,
		&c.Services.Kubelet.InfraContainerImage:          c.SystemImages.PodInfraContainer,
		&c.Services.Etcd.Creation:                        DefaultEtcdBackupCreationPeriod,
		&c.Services.Etcd.Retention:                       DefaultEtcdBackupRetentionPeriod,
	}
	for k, v := range serviceConfigDefaultsMap {
		setDefaultIfEmpty(k, v)
	}
	// Add etcd timeouts
	if c.Services.Etcd.ExtraArgs == nil {
		c.Services.Etcd.ExtraArgs = make(map[string]string)
	}
	if _, ok := c.Services.Etcd.ExtraArgs[DefaultEtcdElectionTimeoutName]; !ok {
		c.Services.Etcd.ExtraArgs[DefaultEtcdElectionTimeoutName] = DefaultEtcdElectionTimeoutValue
	}
	if _, ok := c.Services.Etcd.ExtraArgs[DefaultEtcdHeartbeatIntervalName]; !ok {
		c.Services.Etcd.ExtraArgs[DefaultEtcdHeartbeatIntervalName] = DefaultEtcdHeartbeatIntervalValue
	}

	if c.Services.Etcd.BackupConfig != nil &&
		(c.Services.Etcd.BackupConfig.Enabled == nil ||
			(c.Services.Etcd.BackupConfig.Enabled != nil && *c.Services.Etcd.BackupConfig.Enabled)) {
		if c.Services.Etcd.BackupConfig.IntervalHours == 0 {
			c.Services.Etcd.BackupConfig.IntervalHours = DefaultEtcdBackupConfigIntervalHours
		}
		if c.Services.Etcd.BackupConfig.Retention == 0 {
			c.Services.Etcd.BackupConfig.Retention = DefaultEtcdBackupConfigRetention
		}
		if c.Services.Etcd.BackupConfig.Timeout == 0 {
			c.Services.Etcd.BackupConfig.Timeout = DefaultEtcdBackupConfigTimeout
		}
	}

	if _, ok := c.Services.KubeAPI.ExtraArgs[KubeAPIArgAdmissionControlConfigFile]; !ok {
		if c.Services.KubeAPI.EventRateLimit != nil &&
			c.Services.KubeAPI.EventRateLimit.Enabled &&
			c.Services.KubeAPI.EventRateLimit.Configuration == nil {
			c.Services.KubeAPI.EventRateLimit.Configuration = newDefaultEventRateLimitConfig()
		}
		parsedVersion, err := getClusterVersion(c.Version)
		if err != nil {
			logrus.Warnf("Can not parse the cluster version [%s] to determine wether to set the default PodSecurityConfiguration: %v", c.Version, err)
		} else {
			if parsedRangeAtLeast123(parsedVersion) {
				if len(c.Services.KubeAPI.PodSecurityConfiguration) == 0 {
					c.Services.KubeAPI.PodSecurityConfiguration = PodSecurityPrivileged
				}
			}
		}
	}

	enableKubeAPIAuditLog, err := checkVersionNeedsKubeAPIAuditLog(c.Version)
	if err != nil {
		logrus.Warnf("Can not determine if cluster version [%s] needs to have kube-api audit log enabled: %v", c.Version, err)
	}
	if enableKubeAPIAuditLog {
		logrus.Debugf("Enabling kube-api audit log for cluster version [%s]", c.Version)

		if c.Services.KubeAPI.AuditLog == nil {
			c.Services.KubeAPI.AuditLog = &v3.AuditLog{Enabled: true}
		}

	}
	if c.Services.KubeAPI.AuditLog != nil &&
		c.Services.KubeAPI.AuditLog.Enabled {
		if c.Services.KubeAPI.AuditLog.Configuration == nil {
			alc := newDefaultAuditLogConfig()
			c.Services.KubeAPI.AuditLog.Configuration = alc
		} else {
			if c.Services.KubeAPI.AuditLog.Configuration.Policy == nil {
				c.Services.KubeAPI.AuditLog.Configuration.Policy = newDefaultAuditPolicy()
			}
		}
	}
}

func newDefaultAuditPolicy() *auditv1.Policy {
	p := &auditv1.Policy{
		TypeMeta: v1.TypeMeta{
			Kind:       "Policy",
			APIVersion: auditv1.SchemeGroupVersion.String(),
		},
		Rules: []auditv1.PolicyRule{
			{
				Level: "Metadata",
			},
		},
		OmitStages: nil,
	}
	return p
}

func newDefaultAuditLogConfig() *v3.AuditLogConfig {
	p := newDefaultAuditPolicy()
	c := &v3.AuditLogConfig{
		MaxAge:    30,
		MaxBackup: 10,
		MaxSize:   100,
		Path:      DefaultKubeAPIArgAuditLogPathValue,
		Format:    "json",
		Policy:    p,
	}
	return c
}

func getEventRateLimitPluginFromConfig(c *eventratelimitapi.Configuration) (apiserverv1.AdmissionPluginConfiguration, error) {
	plugin := apiserverv1.AdmissionPluginConfiguration{
		Name: EventRateLimitPluginName,
		Configuration: &runtime.Unknown{
			ContentType: "application/json",
		},
	}

	cBytes, err := json.Marshal(c)
	if err != nil {
		return plugin, fmt.Errorf("error marshalling eventratelimit config: %v", err)
	}
	plugin.Configuration.Raw = cBytes

	return plugin, nil
}

func newDefaultEventRateLimitConfig() *eventratelimitapi.Configuration {
	return &eventratelimitapi.Configuration{
		TypeMeta: v1.TypeMeta{
			Kind:       "Configuration",
			APIVersion: "eventratelimit.admission.k8s.io/v1alpha1",
		},
		Limits: []eventratelimitapi.Limit{
			{
				Type:  eventratelimitapi.ServerLimitType,
				QPS:   5000,
				Burst: 20000,
			},
		},
	}
}

func newDefaultEventRateLimitPlugin() (apiserverv1.AdmissionPluginConfiguration, error) {
	plugin := apiserverv1.AdmissionPluginConfiguration{
		Name: EventRateLimitPluginName,
		Configuration: &runtime.Unknown{
			ContentType: "application/json",
		},
	}

	c := newDefaultEventRateLimitConfig()
	cBytes, err := json.Marshal(c)
	if err != nil {
		return plugin, fmt.Errorf("error marshalling eventratelimit config: %v", err)
	}
	plugin.Configuration.Raw = cBytes

	return plugin, nil
}

func newDefaultAdmissionConfiguration() (*apiserverv1.AdmissionConfiguration, error) {
	var admissionConfiguration *apiserverv1.AdmissionConfiguration
	admissionConfiguration = &apiserverv1.AdmissionConfiguration{
		TypeMeta: v1.TypeMeta{
			Kind:       "AdmissionConfiguration",
			APIVersion: apiserverv1.SchemeGroupVersion.String(),
		},
	}
	return admissionConfiguration, nil
}

func newDefaultPodSecurityPluginConfigurationRestricted(Version string) (apiserverv1.AdmissionPluginConfiguration, error) {
	plugin := apiserverv1.AdmissionPluginConfiguration{
		Name: PodSecurityPluginName,
		Configuration: &runtime.Unknown{
			ContentType: "application/json",
		},
	}
	parsedVersion, err := getClusterVersion(Version)
	if err != nil {
		return plugin, err
	}
	var cBytes []byte
	if parsedRangeAtLeast125(parsedVersion) {
		configuration := admissionapiv1.PodSecurityConfiguration{
			TypeMeta: v1.TypeMeta{
				Kind:       "PodSecurityConfiguration",
				APIVersion: admissionapiv1.SchemeGroupVersion.String(),
			},
			Defaults: admissionapiv1.PodSecurityDefaults{
				Enforce:        "restricted",
				EnforceVersion: "latest",
				Audit:          "restricted",
				AuditVersion:   "latest",
				Warn:           "restricted",
				WarnVersion:    "latest",
			},
			Exemptions: admissionapiv1.PodSecurityExemptions{
				Usernames:      nil,
				Namespaces:     []string{"ingress-nginx", "kube-system"},
				RuntimeClasses: nil,
			},
		}
		cBytes, err = json.Marshal(configuration)
		if err != nil {
			return plugin, fmt.Errorf("error marshalling podSecurity config: %v", err)
		}
	}
	if parsedRange123(parsedVersion) || parsedRange124(parsedVersion) {
		configuration := admissionapiv1beta1.PodSecurityConfiguration{
			TypeMeta: v1.TypeMeta{
				Kind:       "PodSecurityConfiguration",
				APIVersion: admissionapiv1beta1.SchemeGroupVersion.String(),
			},
			Defaults: admissionapiv1beta1.PodSecurityDefaults{
				Enforce:        "restricted",
				EnforceVersion: "latest",
				Audit:          "restricted",
				AuditVersion:   "latest",
				Warn:           "restricted",
				WarnVersion:    "latest",
			},
			Exemptions: admissionapiv1beta1.PodSecurityExemptions{
				Usernames:      nil,
				Namespaces:     []string{"ingress-nginx", "kube-system"},
				RuntimeClasses: nil,
			},
		}
		cBytes, err = json.Marshal(configuration)
		if err != nil {
			return plugin, fmt.Errorf("error marshalling podSecurity config: %v", err)
		}
	}
	plugin.Configuration.Raw = cBytes
	return plugin, nil
}

func newDefaultPodSecurityPluginConfigurationPrivileged(Version string) (apiserverv1.AdmissionPluginConfiguration, error) {
	plugin := apiserverv1.AdmissionPluginConfiguration{
		Name: PodSecurityPluginName,
		Configuration: &runtime.Unknown{
			ContentType: "application/json",
		},
	}
	parsedVersion, err := getClusterVersion(Version)
	if err != nil {
		return plugin, err
	}
	var cBytes []byte
	if parsedRangeAtLeast125(parsedVersion) {
		configuration := admissionapiv1.PodSecurityConfiguration{
			TypeMeta: v1.TypeMeta{
				Kind:       "PodSecurityConfiguration",
				APIVersion: admissionapiv1.SchemeGroupVersion.String(),
			},
			Defaults: admissionapiv1.PodSecurityDefaults{
				Enforce:        "privileged",
				EnforceVersion: "latest",
			},
		}
		cBytes, err = json.Marshal(configuration)
		if err != nil {
			return plugin, fmt.Errorf("error marshalling podSecurity config: %v", err)
		}
	}
	if parsedRange123(parsedVersion) || parsedRange124(parsedVersion) {
		configuration := admissionapiv1beta1.PodSecurityConfiguration{
			TypeMeta: v1.TypeMeta{
				Kind:       "PodSecurityConfiguration",
				APIVersion: admissionapiv1beta1.SchemeGroupVersion.String(),
			},
			Defaults: admissionapiv1beta1.PodSecurityDefaults{
				Enforce:        "privileged",
				EnforceVersion: "latest",
			},
		}
		cBytes, err = json.Marshal(configuration)
		if err != nil {
			return plugin, fmt.Errorf("error marshalling podSecurity config: %v", err)
		}
	}
	plugin.Configuration.Raw = cBytes
	return plugin, nil
}

func (c *Cluster) setClusterImageDefaults() error {
	var privRegURL string

	imageDefaults, ok := metadata.K8sVersionToRKESystemImages[c.Version]
	if !ok {
		return nil
	}

	for _, privReg := range c.PrivateRegistries {
		if privReg.IsDefault {
			privRegURL = privReg.URL
			break
		}
	}
	systemImagesDefaultsMap := map[*string]string{
		&c.SystemImages.Alpine:                    d(imageDefaults.Alpine, privRegURL),
		&c.SystemImages.NginxProxy:                d(imageDefaults.NginxProxy, privRegURL),
		&c.SystemImages.CertDownloader:            d(imageDefaults.CertDownloader, privRegURL),
		&c.SystemImages.KubeDNS:                   d(imageDefaults.KubeDNS, privRegURL),
		&c.SystemImages.KubeDNSSidecar:            d(imageDefaults.KubeDNSSidecar, privRegURL),
		&c.SystemImages.DNSmasq:                   d(imageDefaults.DNSmasq, privRegURL),
		&c.SystemImages.KubeDNSAutoscaler:         d(imageDefaults.KubeDNSAutoscaler, privRegURL),
		&c.SystemImages.CoreDNS:                   d(imageDefaults.CoreDNS, privRegURL),
		&c.SystemImages.CoreDNSAutoscaler:         d(imageDefaults.CoreDNSAutoscaler, privRegURL),
		&c.SystemImages.KubernetesServicesSidecar: d(imageDefaults.KubernetesServicesSidecar, privRegURL),
		&c.SystemImages.Etcd:                      d(imageDefaults.Etcd, privRegURL),
		&c.SystemImages.Kubernetes:                d(imageDefaults.Kubernetes, privRegURL),
		&c.SystemImages.PodInfraContainer:         d(imageDefaults.PodInfraContainer, privRegURL),
		&c.SystemImages.Flannel:                   d(imageDefaults.Flannel, privRegURL),
		&c.SystemImages.FlannelCNI:                d(imageDefaults.FlannelCNI, privRegURL),
		&c.SystemImages.CalicoNode:                d(imageDefaults.CalicoNode, privRegURL),
		&c.SystemImages.CalicoCNI:                 d(imageDefaults.CalicoCNI, privRegURL),
		&c.SystemImages.CalicoCtl:                 d(imageDefaults.CalicoCtl, privRegURL),
		&c.SystemImages.CalicoControllers:         d(imageDefaults.CalicoControllers, privRegURL),
		&c.SystemImages.CalicoFlexVol:             d(imageDefaults.CalicoFlexVol, privRegURL),
		&c.SystemImages.CanalNode:                 d(imageDefaults.CanalNode, privRegURL),
		&c.SystemImages.CanalCNI:                  d(imageDefaults.CanalCNI, privRegURL),
		&c.SystemImages.CanalControllers:          d(imageDefaults.CanalControllers, privRegURL),
		&c.SystemImages.CanalFlannel:              d(imageDefaults.CanalFlannel, privRegURL),
		&c.SystemImages.CanalFlexVol:              d(imageDefaults.CanalFlexVol, privRegURL),
		&c.SystemImages.WeaveNode:                 d(imageDefaults.WeaveNode, privRegURL),
		&c.SystemImages.WeaveCNI:                  d(imageDefaults.WeaveCNI, privRegURL),
		&c.SystemImages.Ingress:                   d(imageDefaults.Ingress, privRegURL),
		&c.SystemImages.IngressBackend:            d(imageDefaults.IngressBackend, privRegURL),
		&c.SystemImages.IngressWebhook:            d(imageDefaults.IngressWebhook, privRegURL),
		&c.SystemImages.MetricsServer:             d(imageDefaults.MetricsServer, privRegURL),
		&c.SystemImages.Nodelocal:                 d(imageDefaults.Nodelocal, privRegURL),
		&c.SystemImages.AciCniDeployContainer:     d(imageDefaults.AciCniDeployContainer, privRegURL),
		&c.SystemImages.AciHostContainer:          d(imageDefaults.AciHostContainer, privRegURL),
		&c.SystemImages.AciOpflexContainer:        d(imageDefaults.AciOpflexContainer, privRegURL),
		&c.SystemImages.AciMcastContainer:         d(imageDefaults.AciMcastContainer, privRegURL),
		&c.SystemImages.AciOpenvSwitchContainer:   d(imageDefaults.AciOpenvSwitchContainer, privRegURL),
		&c.SystemImages.AciControllerContainer:    d(imageDefaults.AciControllerContainer, privRegURL),
		&c.SystemImages.AciOpflexServerContainer:  d(imageDefaults.AciOpflexServerContainer, privRegURL),
		&c.SystemImages.AciGbpServerContainer:     d(imageDefaults.AciGbpServerContainer, privRegURL),

		// this's a stopgap, we could drop this after https://github.com/kubernetes/kubernetes/pull/75618 merged
		&c.SystemImages.WindowsPodInfraContainer: d(imageDefaults.WindowsPodInfraContainer, privRegURL),
	}

	for k, v := range systemImagesDefaultsMap {
		setDefaultIfEmpty(k, v)
	}

	return nil
}

func (c *Cluster) setClusterDNSDefaults() error {
	if c.DNS != nil && len(c.DNS.Provider) != 0 {
		return nil
	}
	clusterSemVer, err := util.StrToSemVer(c.Version)
	if err != nil {
		return err
	}
	logrus.Debugf("No DNS provider configured, setting default based on cluster version [%s]", clusterSemVer)
	K8sVersionCoreDNSSemVer, err := util.StrToSemVer(K8sVersionCoreDNS)
	if err != nil {
		return err
	}
	// Default DNS provider for cluster version 1.14.0 and higher is coredns
	ClusterDNSProvider := CoreDNSProvider
	// If cluster version is less than 1.14.0 (K8sVersionCoreDNSSemVer), use kube-dns
	if clusterSemVer.LessThan(*K8sVersionCoreDNSSemVer) {
		logrus.Debugf("Cluster version [%s] is less than version [%s], using DNS provider [%s]", clusterSemVer, K8sVersionCoreDNSSemVer, DefaultDNSProvider)
		ClusterDNSProvider = DefaultDNSProvider
	}
	if c.DNS == nil {
		c.DNS = &v3.DNSConfig{}
	}
	c.DNS.Provider = ClusterDNSProvider
	logrus.Debugf("DNS provider set to [%s]", ClusterDNSProvider)
	if c.DNS.Options == nil {
		// don't break if the user didn't define options
		c.DNS.Options = make(map[string]string)
	}
	return nil
}

func (c *Cluster) setClusterNetworkDefaults() {
	setDefaultIfEmpty(&c.Network.Plugin, DefaultNetworkPlugin)

	if c.Network.Options == nil {
		// don't break if the user didn't define options
		c.Network.Options = make(map[string]string)
	}
	networkPluginConfigDefaultsMap := make(map[string]string)
	// This is still needed because RKE doesn't use c.Network.*NetworkProvider, that's a rancher type
	switch c.Network.Plugin {
	case CalicoNetworkPlugin:
		networkPluginConfigDefaultsMap = map[string]string{
			CalicoCloudProvider:          DefaultNetworkCloudProvider,
			CalicoFlexVolPluginDirectory: DefaultCalicoFlexVolPluginDirectory,
		}
	case FlannelNetworkPlugin:
		networkPluginConfigDefaultsMap = map[string]string{
			FlannelBackendType:                 DefaultFlannelBackendVxLan,
			FlannelBackendPort:                 DefaultFlannelBackendVxLanPort,
			FlannelBackendVxLanNetworkIdentify: DefaultFlannelBackendVxLanVNI,
		}
	case CanalNetworkPlugin:
		networkPluginConfigDefaultsMap = map[string]string{
			CanalFlannelBackendType:                 DefaultFlannelBackendVxLan,
			CanalFlannelBackendPort:                 DefaultFlannelBackendVxLanPort,
			CanalFlannelBackendVxLanNetworkIdentify: DefaultFlannelBackendVxLanVNI,
			CanalFlexVolPluginDirectory:             DefaultCanalFlexVolPluginDirectory,
		}
	case AciNetworkPlugin:
		networkPluginConfigDefaultsMap = map[string]string{
			AciOVSMemoryLimit:                    DefaultAciOVSMemoryLimit,
			AciOVSMemoryRequest:                  DefaultAciOVSMemoryRequest,
			AciImagePullPolicy:                   DefaultAciImagePullPolicy,
			AciPBRTrackingNonSnat:                DefaultAciPBRTrackingNonSnat,
			AciInstallIstio:                      DefaultAciInstallIstio,
			AciIstioProfile:                      DefaultAciIstioProfile,
			AciDropLogEnable:                     DefaultAciDropLogEnable,
			AciControllerLogLevel:                DefaultAciControllerLogLevel,
			AciHostAgentLogLevel:                 DefaultAciHostAgentLogLevel,
			AciOpflexAgentLogLevel:               DefaultAciOpflexAgentLogLevel,
			AciApicRefreshTime:                   DefaultAciApicRefreshTime,
			AciServiceMonitorInterval:            DefaultAciServiceMonitorInterval,
			AciUseAciCniPriorityClass:            DefaultAciUseAciCniPriorityClass,
			AciNoPriorityClass:                   DefaultAciNoPriorityClass,
			AciMaxNodesSvcGraph:                  DefaultAciMaxNodesSvcGraph,
			AciSnatContractScope:                 DefaultAciSnatContractScope,
			AciPodSubnetChunkSize:                DefaultAciPodSubnetChunkSize,
			AciEnableEndpointSlice:               DefaultAciEnableEndpointSlice,
			AciSnatNamespace:                     DefaultAciSnatNamespace,
			AciSnatPortRangeStart:                DefaultAciSnatPortRangeStart,
			AciSnatPortRangeEnd:                  DefaultAciSnatPortRangeEnd,
			AciSnatPortsPerNode:                  DefaultAciSnatPortsPerNode,
			AciOpflexClientSSL:                   DefaultAciOpflexClientSSL,
			AciUsePrivilegedContainer:            DefaultAciUsePrivilegedContainer,
			AciUseOpflexServerVolume:             DefaultAciUseOpflexServerVolume,
			AciUseHostNetnsVolume:                DefaultAciUseHostNetnsVolume,
			AciCApic:                             DefaultAciCApic,
			AciUseAciAnywhereCRD:                 DefaultAciUseAciAnywhereCRD,
			AciRunGbpContainer:                   DefaultAciRunGbpContainer,
			AciRunOpflexServerContainer:          DefaultAciRunOpflexServerContainer,
			AciDurationWaitForNetwork:            DefaultAciDurationWaitForNetwork,
			AciUseClusterRole:                    DefaultAciUseClusterRole,
			AciDisableWaitForNetwork:             DefaultAciDisableWaitForNetwork,
			AciApicSubscriptionDelay:             DefaultAciApicSubscriptionDelay,
			AciApicRefreshTickerAdjust:           DefaultAciApicRefreshTickerAdjust,
			AciDisablePeriodicSnatGlobalInfoSync: DefaultAciDisablePeriodicSnatGlobalInfoSync,
			AciOpflexDeviceDeleteTimeout:         DefaultAciOpflexDeviceDeleteTimeout,
			AciMTUHeadRoom:                       DefaultAciMTUHeadRoom,
			AciNodePodIfEnable:                   DefaultAciNodePodIfEnable,
			AciSriovEnable:                       DefaultAciSriovEnable,
			AciMultusDisable:                     DefaultAciMultusDisable,
			AciNoWaitForServiceEpReadiness:       DefaultAciNoWaitForServiceEpReadiness,
			AciAddExternalSubnetsToRdconfig:      DefaultAciAddExternalSubnetsToRdconfig,
			AciServiceGraphEndpointAddDelay:      DefaultAciServiceGraphEndpointAddDelay,
			AciHppOptimization:                   DefaultAciHppOptimization,
			AciSleepTimeSnatGlobalInfoSync:       DefaultAciSleepTimeSnatGlobalInfoSync,
			AciOpflexAgentOpflexAsyncjsonEnabled: DefaultAciOpflexAgentOpflexAsyncjsonEnabled,
			AciOpflexAgentOvsAsyncjsonEnabled:    DefaultAciOpflexAgentOvsAsyncjsonEnabled,
			AciOpflexAgentPolicyRetryDelayTimer:  DefaultAciOpflexAgentPolicyRetryDelayTimer,
			AciAciMultipod:                       DefaultAciAciMultipod,
			AciAciMultipodUbuntu:                 DefaultAciAciMultipodUbuntu,
			AciDhcpRenewMaxRetryCount:            DefaultAciDhcpRenewMaxRetryCount,
			AciDhcpDelay:                         DefaultAciDhcpDelay,
			AciUseSystemNodePriorityClass:        DefaultAciUseSystemNodePriorityClass,
			AciContainersMemoryRequest:           DefaultAciAciContainersMemoryRequest,
			AciContainersMemoryLimit:             DefaultAciAciContainersMemoryLimit,
		}
	}
	if c.Network.CalicoNetworkProvider != nil {
		setDefaultIfEmpty(&c.Network.CalicoNetworkProvider.CloudProvider, DefaultNetworkCloudProvider)
		networkPluginConfigDefaultsMap[CalicoCloudProvider] = c.Network.CalicoNetworkProvider.CloudProvider
	}
	if c.Network.FlannelNetworkProvider != nil {
		networkPluginConfigDefaultsMap[FlannelIface] = c.Network.FlannelNetworkProvider.Iface

	}
	if c.Network.CanalNetworkProvider != nil {
		networkPluginConfigDefaultsMap[CanalIface] = c.Network.CanalNetworkProvider.Iface
	}
	if c.Network.WeaveNetworkProvider != nil {
		networkPluginConfigDefaultsMap[WeavePassword] = c.Network.WeaveNetworkProvider.Password
	}
	if c.Network.AciNetworkProvider != nil {
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.OVSMemoryLimit, DefaultAciOVSMemoryLimit)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.OVSMemoryRequest, DefaultAciOVSMemoryRequest)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.ImagePullPolicy, DefaultAciImagePullPolicy)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.PBRTrackingNonSnat, DefaultAciPBRTrackingNonSnat)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.InstallIstio, DefaultAciInstallIstio)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.IstioProfile, DefaultAciIstioProfile)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.DropLogEnable, DefaultAciDropLogEnable)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.ControllerLogLevel, DefaultAciControllerLogLevel)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.HostAgentLogLevel, DefaultAciHostAgentLogLevel)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.OpflexAgentLogLevel, DefaultAciOpflexAgentLogLevel)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.ApicRefreshTime, DefaultAciApicRefreshTime)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.ServiceMonitorInterval, DefaultAciServiceMonitorInterval)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.NoPriorityClass, DefaultAciNoPriorityClass)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.MaxNodesSvcGraph, DefaultAciMaxNodesSvcGraph)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.SnatContractScope, DefaultAciSnatContractScope)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.PodSubnetChunkSize, DefaultAciPodSubnetChunkSize)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.EnableEndpointSlice, DefaultAciEnableEndpointSlice)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.SnatNamespace, DefaultAciSnatNamespace)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.SnatPortRangeStart, DefaultAciSnatPortRangeStart)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.SnatPortRangeEnd, DefaultAciSnatPortRangeEnd)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.SnatPortsPerNode, DefaultAciSnatPortsPerNode)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.OpflexClientSSL, DefaultAciOpflexClientSSL)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.UsePrivilegedContainer, DefaultAciUsePrivilegedContainer)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.UseOpflexServerVolume, DefaultAciUseOpflexServerVolume)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.UseHostNetnsVolume, DefaultAciUseHostNetnsVolume)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.CApic, DefaultAciCApic)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.UseAciAnywhereCRD, DefaultAciUseAciAnywhereCRD)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.RunGbpContainer, DefaultAciRunGbpContainer)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.RunOpflexServerContainer, DefaultAciRunOpflexServerContainer)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.SriovEnable, DefaultAciSriovEnable)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.NodePodIfEnable, DefaultAciNodePodIfEnable)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.MultusDisable, DefaultAciMultusDisable)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.DisablePeriodicSnatGlobalInfoSync, DefaultAciDisablePeriodicSnatGlobalInfoSync)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.ApicSubscriptionDelay, DefaultAciApicSubscriptionDelay)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.ApicRefreshTickerAdjust, DefaultAciApicRefreshTickerAdjust)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.OpflexDeviceDeleteTimeout, DefaultAciOpflexDeviceDeleteTimeout)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.MTUHeadRoom, DefaultAciMTUHeadRoom)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.DurationWaitForNetwork, DefaultAciDurationWaitForNetwork)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.DisableWaitForNetwork, DefaultAciDisableWaitForNetwork)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.UseClusterRole, DefaultAciUseClusterRole)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.NoWaitForServiceEpReadiness, DefaultAciNoWaitForServiceEpReadiness)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.AddExternalSubnetsToRdconfig, DefaultAciAddExternalSubnetsToRdconfig)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.ServiceGraphEndpointAddDelay, DefaultAciServiceGraphEndpointAddDelay)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.HppOptimization, DefaultAciHppOptimization)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.SleepTimeSnatGlobalInfoSync, DefaultAciSleepTimeSnatGlobalInfoSync)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.OpflexAgentOpflexAsyncjsonEnabled, DefaultAciOpflexAgentOpflexAsyncjsonEnabled)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.OpflexAgentOvsAsyncjsonEnabled, DefaultAciOpflexAgentOvsAsyncjsonEnabled)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.OpflexAgentPolicyRetryDelayTimer, DefaultAciOpflexAgentPolicyRetryDelayTimer)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.AciMultipod, DefaultAciAciMultipod)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.AciMultipodUbuntu, DefaultAciAciMultipodUbuntu)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.DhcpRenewMaxRetryCount, DefaultAciDhcpRenewMaxRetryCount)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.DhcpDelay, DefaultAciDhcpDelay)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.UseSystemNodePriorityClass, DefaultAciUseSystemNodePriorityClass)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.AciContainersMemoryLimit, DefaultAciAciContainersMemoryLimit)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.AciContainersMemoryRequest, DefaultAciAciContainersMemoryRequest)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.OpflexAgentStatistics, DefaultAciOpflexAgentStatistics)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.AddExternalContractToDefaultEpg, DefaultAciAddExternalContractToDefaultEpg)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.EnableOpflexAgentReconnect, DefaultAciEnableOpflexAgentReconnect)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.OpflexOpensslCompat, DefaultAciOpflexOpensslCompat)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.TolerationSeconds, DefaultAciTolerationSeconds)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.DisableHppRendering, DefaultAciDisableHppRendering)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.ApicConnectionRetryLimit, DefaultAciApicConnectionRetryLimit)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.TaintNotReadyNode, DefaultAciTaintNotReadyNode)
		setDefaultIfEmpty(&c.Network.AciNetworkProvider.DropLogDisableEvents, DefaultAciDropLogDisableEvents)
		networkPluginConfigDefaultsMap[AciOVSMemoryLimit] = c.Network.AciNetworkProvider.OVSMemoryLimit
		networkPluginConfigDefaultsMap[AciOVSMemoryRequest] = c.Network.AciNetworkProvider.OVSMemoryRequest
		networkPluginConfigDefaultsMap[AciImagePullPolicy] = c.Network.AciNetworkProvider.ImagePullPolicy
		networkPluginConfigDefaultsMap[AciPBRTrackingNonSnat] = c.Network.AciNetworkProvider.PBRTrackingNonSnat
		networkPluginConfigDefaultsMap[AciInstallIstio] = c.Network.AciNetworkProvider.InstallIstio
		networkPluginConfigDefaultsMap[AciIstioProfile] = c.Network.AciNetworkProvider.IstioProfile
		networkPluginConfigDefaultsMap[AciDropLogEnable] = c.Network.AciNetworkProvider.DropLogEnable
		networkPluginConfigDefaultsMap[AciControllerLogLevel] = c.Network.AciNetworkProvider.ControllerLogLevel
		networkPluginConfigDefaultsMap[AciHostAgentLogLevel] = c.Network.AciNetworkProvider.HostAgentLogLevel
		networkPluginConfigDefaultsMap[AciOpflexAgentLogLevel] = c.Network.AciNetworkProvider.OpflexAgentLogLevel
		networkPluginConfigDefaultsMap[AciApicRefreshTime] = c.Network.AciNetworkProvider.ApicRefreshTime
		networkPluginConfigDefaultsMap[AciServiceMonitorInterval] = c.Network.AciNetworkProvider.ServiceMonitorInterval
		networkPluginConfigDefaultsMap[AciNoPriorityClass] = c.Network.AciNetworkProvider.NoPriorityClass
		networkPluginConfigDefaultsMap[AciMaxNodesSvcGraph] = c.Network.AciNetworkProvider.MaxNodesSvcGraph
		networkPluginConfigDefaultsMap[AciSnatContractScope] = c.Network.AciNetworkProvider.SnatContractScope
		networkPluginConfigDefaultsMap[AciPodSubnetChunkSize] = c.Network.AciNetworkProvider.PodSubnetChunkSize
		networkPluginConfigDefaultsMap[AciEnableEndpointSlice] = c.Network.AciNetworkProvider.EnableEndpointSlice
		networkPluginConfigDefaultsMap[AciSnatNamespace] = c.Network.AciNetworkProvider.SnatNamespace
		networkPluginConfigDefaultsMap[AciSnatPortRangeStart] = c.Network.AciNetworkProvider.SnatPortRangeStart
		networkPluginConfigDefaultsMap[AciSnatPortRangeEnd] = c.Network.AciNetworkProvider.SnatPortRangeEnd
		networkPluginConfigDefaultsMap[AciSnatPortsPerNode] = c.Network.AciNetworkProvider.SnatPortsPerNode
		networkPluginConfigDefaultsMap[AciOpflexClientSSL] = c.Network.AciNetworkProvider.OpflexClientSSL
		networkPluginConfigDefaultsMap[AciUsePrivilegedContainer] = c.Network.AciNetworkProvider.UsePrivilegedContainer
		networkPluginConfigDefaultsMap[AciUseOpflexServerVolume] = c.Network.AciNetworkProvider.UseOpflexServerVolume
		networkPluginConfigDefaultsMap[AciUseHostNetnsVolume] = c.Network.AciNetworkProvider.UseHostNetnsVolume
		networkPluginConfigDefaultsMap[AciCApic] = c.Network.AciNetworkProvider.CApic
		networkPluginConfigDefaultsMap[AciUseAciAnywhereCRD] = c.Network.AciNetworkProvider.UseAciAnywhereCRD
		networkPluginConfigDefaultsMap[AciRunGbpContainer] = c.Network.AciNetworkProvider.RunGbpContainer
		networkPluginConfigDefaultsMap[AciRunOpflexServerContainer] = c.Network.AciNetworkProvider.RunOpflexServerContainer
		networkPluginConfigDefaultsMap[AciDurationWaitForNetwork] = c.Network.AciNetworkProvider.DurationWaitForNetwork
		networkPluginConfigDefaultsMap[AciDisableWaitForNetwork] = c.Network.AciNetworkProvider.DisableWaitForNetwork
		networkPluginConfigDefaultsMap[AciUseClusterRole] = c.Network.AciNetworkProvider.UseClusterRole
		networkPluginConfigDefaultsMap[AciApicSubscriptionDelay] = c.Network.AciNetworkProvider.ApicSubscriptionDelay
		networkPluginConfigDefaultsMap[AciApicRefreshTickerAdjust] = c.Network.AciNetworkProvider.ApicRefreshTickerAdjust
		networkPluginConfigDefaultsMap[AciDisablePeriodicSnatGlobalInfoSync] = c.Network.AciNetworkProvider.DisablePeriodicSnatGlobalInfoSync
		networkPluginConfigDefaultsMap[AciOpflexDeviceDeleteTimeout] = c.Network.AciNetworkProvider.OpflexDeviceDeleteTimeout
		networkPluginConfigDefaultsMap[AciMTUHeadRoom] = c.Network.AciNetworkProvider.MTUHeadRoom
		networkPluginConfigDefaultsMap[AciNodePodIfEnable] = c.Network.AciNetworkProvider.NodePodIfEnable
		networkPluginConfigDefaultsMap[AciSriovEnable] = c.Network.AciNetworkProvider.SriovEnable
		networkPluginConfigDefaultsMap[AciMultusDisable] = c.Network.AciNetworkProvider.MultusDisable
		networkPluginConfigDefaultsMap[AciNoWaitForServiceEpReadiness] = c.Network.AciNetworkProvider.NoWaitForServiceEpReadiness
		networkPluginConfigDefaultsMap[AciAddExternalSubnetsToRdconfig] = c.Network.AciNetworkProvider.AddExternalSubnetsToRdconfig
		networkPluginConfigDefaultsMap[AciServiceGraphEndpointAddDelay] = c.Network.AciNetworkProvider.ServiceGraphEndpointAddDelay
		networkPluginConfigDefaultsMap[AciHppOptimization] = c.Network.AciNetworkProvider.HppOptimization
		networkPluginConfigDefaultsMap[AciSleepTimeSnatGlobalInfoSync] = c.Network.AciNetworkProvider.SleepTimeSnatGlobalInfoSync
		networkPluginConfigDefaultsMap[AciOpflexAgentOpflexAsyncjsonEnabled] = c.Network.AciNetworkProvider.OpflexAgentOpflexAsyncjsonEnabled
		networkPluginConfigDefaultsMap[AciOpflexAgentOvsAsyncjsonEnabled] = c.Network.AciNetworkProvider.OpflexAgentOvsAsyncjsonEnabled
		networkPluginConfigDefaultsMap[AciOpflexAgentPolicyRetryDelayTimer] = c.Network.AciNetworkProvider.OpflexAgentPolicyRetryDelayTimer
		networkPluginConfigDefaultsMap[AciDhcpRenewMaxRetryCount] = c.Network.AciNetworkProvider.DhcpRenewMaxRetryCount
		networkPluginConfigDefaultsMap[AciDhcpDelay] = c.Network.AciNetworkProvider.DhcpDelay
		networkPluginConfigDefaultsMap[AciAciMultipod] = c.Network.AciNetworkProvider.AciMultipod
		networkPluginConfigDefaultsMap[AciOpflexDeviceReconnectWaitTimeout] = c.Network.AciNetworkProvider.OpflexDeviceReconnectWaitTimeout
		networkPluginConfigDefaultsMap[AciAciMultipodUbuntu] = c.Network.AciNetworkProvider.AciMultipodUbuntu
		networkPluginConfigDefaultsMap[AciSystemIdentifier] = c.Network.AciNetworkProvider.SystemIdentifier
		networkPluginConfigDefaultsMap[AciToken] = c.Network.AciNetworkProvider.Token
		networkPluginConfigDefaultsMap[AciApicUserName] = c.Network.AciNetworkProvider.ApicUserName
		networkPluginConfigDefaultsMap[AciApicUserKey] = c.Network.AciNetworkProvider.ApicUserKey
		networkPluginConfigDefaultsMap[AciApicUserCrt] = c.Network.AciNetworkProvider.ApicUserCrt
		networkPluginConfigDefaultsMap[AciApicRefreshTime] = c.Network.AciNetworkProvider.ApicRefreshTime
		networkPluginConfigDefaultsMap[AciVmmDomain] = c.Network.AciNetworkProvider.VmmDomain
		networkPluginConfigDefaultsMap[AciVmmController] = c.Network.AciNetworkProvider.VmmController
		networkPluginConfigDefaultsMap[AciEncapType] = c.Network.AciNetworkProvider.EncapType
		networkPluginConfigDefaultsMap[AciMcastRangeStart] = c.Network.AciNetworkProvider.McastRangeStart
		networkPluginConfigDefaultsMap[AciMcastRangeEnd] = c.Network.AciNetworkProvider.McastRangeEnd
		networkPluginConfigDefaultsMap[AciNodeSubnet] = c.Network.AciNetworkProvider.NodeSubnet
		networkPluginConfigDefaultsMap[AciAEP] = c.Network.AciNetworkProvider.AEP
		networkPluginConfigDefaultsMap[AciVRFName] = c.Network.AciNetworkProvider.VRFName
		networkPluginConfigDefaultsMap[AciVRFTenant] = c.Network.AciNetworkProvider.VRFTenant
		networkPluginConfigDefaultsMap[AciL3Out] = c.Network.AciNetworkProvider.L3Out
		networkPluginConfigDefaultsMap[AciDynamicExternalSubnet] = c.Network.AciNetworkProvider.DynamicExternalSubnet
		networkPluginConfigDefaultsMap[AciStaticExternalSubnet] = c.Network.AciNetworkProvider.StaticExternalSubnet
		networkPluginConfigDefaultsMap[AciServiceGraphSubnet] = c.Network.AciNetworkProvider.ServiceGraphSubnet
		networkPluginConfigDefaultsMap[AciKubeAPIVlan] = c.Network.AciNetworkProvider.KubeAPIVlan
		networkPluginConfigDefaultsMap[AciServiceVlan] = c.Network.AciNetworkProvider.ServiceVlan
		networkPluginConfigDefaultsMap[AciInfraVlan] = c.Network.AciNetworkProvider.InfraVlan
		networkPluginConfigDefaultsMap[AciImagePullPolicy] = c.Network.AciNetworkProvider.ImagePullPolicy
		networkPluginConfigDefaultsMap[AciImagePullSecret] = c.Network.AciNetworkProvider.ImagePullSecret
		networkPluginConfigDefaultsMap[AciTenant] = c.Network.AciNetworkProvider.Tenant
		networkPluginConfigDefaultsMap[AciKafkaClientCrt] = c.Network.AciNetworkProvider.KafkaClientCrt
		networkPluginConfigDefaultsMap[AciKafkaClientKey] = c.Network.AciNetworkProvider.KafkaClientKey
		networkPluginConfigDefaultsMap[AciSubnetDomainName] = c.Network.AciNetworkProvider.SubnetDomainName
		networkPluginConfigDefaultsMap[AciEpRegistry] = c.Network.AciNetworkProvider.EpRegistry
		networkPluginConfigDefaultsMap[AciOpflexMode] = c.Network.AciNetworkProvider.OpflexMode
		networkPluginConfigDefaultsMap[AciOverlayVRFName] = c.Network.AciNetworkProvider.OverlayVRFName
		networkPluginConfigDefaultsMap[AciGbpPodSubnet] = c.Network.AciNetworkProvider.GbpPodSubnet
		networkPluginConfigDefaultsMap[AciOpflexServerPort] = c.Network.AciNetworkProvider.OpflexServerPort
		networkPluginConfigDefaultsMap[AciUseSystemNodePriorityClass] = c.Network.AciNetworkProvider.UseSystemNodePriorityClass
		networkPluginConfigDefaultsMap[AciAciContainersControllerMemoryRequest] = c.Network.AciNetworkProvider.AciContainersControllerMemoryRequest
		networkPluginConfigDefaultsMap[AciAciContainersControllerMemoryLimit] = c.Network.AciNetworkProvider.AciContainersControllerMemoryLimit
		networkPluginConfigDefaultsMap[AciAciContainersHostMemoryRequest] = c.Network.AciNetworkProvider.AciContainersHostMemoryRequest
		networkPluginConfigDefaultsMap[AciAciContainersHostMemoryLimit] = c.Network.AciNetworkProvider.AciContainersHostMemoryLimit
		networkPluginConfigDefaultsMap[AciMcastDaemonMemoryRequest] = c.Network.AciNetworkProvider.McastDaemonMemoryRequest
		networkPluginConfigDefaultsMap[AciMcastDaemonMemoryLimit] = c.Network.AciNetworkProvider.McastDaemonMemoryLimit
		networkPluginConfigDefaultsMap[AciOpflexAgentMemoryRequest] = c.Network.AciNetworkProvider.OpflexAgentMemoryRequest
		networkPluginConfigDefaultsMap[AciOpflexAgentMemoryLimit] = c.Network.AciNetworkProvider.OpflexAgentMemoryLimit
		networkPluginConfigDefaultsMap[AciAciContainersMemoryRequest] = c.Network.AciNetworkProvider.AciContainersMemoryRequest
		networkPluginConfigDefaultsMap[AciAciContainersMemoryLimit] = c.Network.AciNetworkProvider.AciContainersMemoryLimit
		networkPluginConfigDefaultsMap[AciOpflexAgentStatistics] = c.Network.AciNetworkProvider.OpflexAgentStatistics
		networkPluginConfigDefaultsMap[AciAddExternalContractToDefaultEpg] = c.Network.AciNetworkProvider.AddExternalContractToDefaultEpg
		networkPluginConfigDefaultsMap[AciEnableOpflexAgentReconnect] = c.Network.AciNetworkProvider.EnableOpflexAgentReconnect
		networkPluginConfigDefaultsMap[AciOpflexOpensslCompat] = c.Network.AciNetworkProvider.OpflexOpensslCompat
		networkPluginConfigDefaultsMap[AciTolerationSeconds] = c.Network.AciNetworkProvider.TolerationSeconds
		networkPluginConfigDefaultsMap[AciDisableHppRendering] = c.Network.AciNetworkProvider.DisableHppRendering
		networkPluginConfigDefaultsMap[AciApicConnectionRetryLimit] = c.Network.AciNetworkProvider.ApicConnectionRetryLimit
		networkPluginConfigDefaultsMap[AciTaintNotReadyNode] = c.Network.AciNetworkProvider.TaintNotReadyNode
		networkPluginConfigDefaultsMap[AciDropLogDisableEvents] = c.Network.AciNetworkProvider.DropLogDisableEvents
	}
	for k, v := range networkPluginConfigDefaultsMap {
		setDefaultIfEmptyMapValue(c.Network.Options, k, v)
	}
}

func (c *Cluster) setClusterAuthnDefaults() {
	setDefaultIfEmpty(&c.Authentication.Strategy, DefaultAuthStrategy)

	for _, strategy := range strings.Split(c.Authentication.Strategy, "|") {
		strategy = strings.ToLower(strings.TrimSpace(strategy))
		c.AuthnStrategies[strategy] = true
	}

	if c.AuthnStrategies[AuthnWebhookProvider] && c.Authentication.Webhook == nil {
		c.Authentication.Webhook = &v3.AuthWebhookConfig{}
	}
	if c.Authentication.Webhook != nil {
		webhookConfigDefaultsMap := map[*string]string{
			&c.Authentication.Webhook.ConfigFile:   DefaultAuthnWebhookFile,
			&c.Authentication.Webhook.CacheTimeout: DefaultAuthnCacheTimeout,
		}
		for k, v := range webhookConfigDefaultsMap {
			setDefaultIfEmpty(k, v)
		}
	}
}

func d(image, defaultRegistryURL string) string {
	if len(defaultRegistryURL) == 0 {
		return image
	}
	return fmt.Sprintf("%s/%s", defaultRegistryURL, image)
}

func (c *Cluster) setCloudProvider() error {
	p, err := cloudprovider.InitCloudProvider(c.CloudProvider)
	if err != nil {
		return fmt.Errorf("Failed to initialize cloud provider: %v", err)
	}
	if p != nil {
		c.CloudConfigFile, err = p.GenerateCloudConfigFile()
		if err != nil {
			return fmt.Errorf("failed to parse cloud config file: %v", err)
		}
		c.CloudProvider.Name = p.GetName()
		if c.CloudProvider.Name == "" {
			return fmt.Errorf("name of the cloud provider is not defined for custom provider")
		}
		if c.CloudProvider.Name == aws.AWSCloudProviderName {
			clusterVersion, err := getClusterVersion(c.Version)
			if err != nil {
				return fmt.Errorf("failed to get cluster version for checking cloud provider: %v", err)
			}
			// cloud provider must be external or external-aws for >=1.27
			defaultExternalAwsRange, err := semver.ParseRange(">=1.27.0-rancher0")
			if err != nil {
				return fmt.Errorf("failed to parse semver range for checking cloud provider %v", err)
			}
			if defaultExternalAwsRange(clusterVersion) {
				return fmt.Errorf(fmt.Sprintf("Cloud provider %s is invalid for [%s]", aws.AWSCloudProviderName, c.Version))
			}
		}
	}
	return nil
}

func GetExternalFlags(local, updateOnly, disablePortCheck, useLocalState bool, configDir, clusterFilePath string) ExternalFlags {
	return ExternalFlags{
		Local:            local,
		UpdateOnly:       updateOnly,
		DisablePortCheck: disablePortCheck,
		ConfigDir:        configDir,
		ClusterFilePath:  clusterFilePath,
		UseLocalState:    useLocalState,
	}
}

func (c *Cluster) setAddonsDefaults() {
	c.Ingress.UpdateStrategy = setDaemonsetAddonDefaults(c.Ingress.UpdateStrategy)
	c.Network.UpdateStrategy = setDaemonsetAddonDefaults(c.Network.UpdateStrategy)
	c.DNS.UpdateStrategy = setDNSDeploymentAddonDefaults(c.DNS.UpdateStrategy, c.DNS.Provider)
	if c.DNS.LinearAutoscalerParams == nil {
		c.DNS.LinearAutoscalerParams = &DefaultClusterProportionalAutoscalerLinearParams
	}
	c.Monitoring.UpdateStrategy = setDeploymentAddonDefaults(c.Monitoring.UpdateStrategy)
	if c.Monitoring.Replicas == nil {
		c.Monitoring.Replicas = &DefaultMonitoringAddonReplicas
	}
	k8sVersion := c.RancherKubernetesEngineConfig.Version
	toMatch, err := semver.Make(k8sVersion[1:])
	if err != nil {
		logrus.Warnf("Cluster version [%s] can not be parsed as semver", k8sVersion)
	}

	if c.Ingress.NetworkMode == "" {
		networkMode := DefaultNetworkMode
		logrus.Debugf("Checking network mode for ingress for cluster version [%s]", k8sVersion)
		// network mode will be hostport for k8s 1.21 and up
		hostPortRange, err := semver.ParseRange(">=1.21.0-rancher0")
		if err != nil {
			logrus.Warnf("Failed to parse semver range for checking ingress network mode")
		}
		if hostPortRange(toMatch) {
			logrus.Debugf("Cluster version [%s] needs to have ingress network mode set to hostPort", k8sVersion)
			networkMode = DefaultNetworkModeV121
		}

		c.Ingress.NetworkMode = networkMode
	}

	if c.Ingress.HTTPPort == 0 {
		c.Ingress.HTTPPort = DefaultHTTPPort
	}

	if c.Ingress.HTTPSPort == 0 {
		c.Ingress.HTTPSPort = DefaultHTTPSPort
	}

	if c.Ingress.DefaultBackend == nil {
		defaultBackend := true
		logrus.Debugf("Checking ingress default backend for cluster version [%s]", k8sVersion)
		// default backend will be false for k8s 1.21 and up
		disableIngressDefaultBackendRange, err := semver.ParseRange(">=1.21.0-rancher0")
		if err != nil {
			logrus.Warnf("Failed to parse semver range for checking ingress default backend")
		}
		if disableIngressDefaultBackendRange(toMatch) {
			logrus.Debugf("Cluster version [%s] needs to have ingress default backend disabled", k8sVersion)
			defaultBackend = false
		}
		c.Ingress.DefaultBackend = &defaultBackend
	}
	if c.Ingress.DefaultIngressClass == nil {
		defaultIngressClass := true
		c.Ingress.DefaultIngressClass = &defaultIngressClass
	}
}

func setDaemonsetAddonDefaults(updateStrategy *v3.DaemonSetUpdateStrategy) *v3.DaemonSetUpdateStrategy {
	if updateStrategy != nil && updateStrategy.Strategy != appsv1.RollingUpdateDaemonSetStrategyType {
		return updateStrategy
	}
	if updateStrategy == nil || updateStrategy.RollingUpdate == nil || updateStrategy.RollingUpdate.MaxUnavailable == nil {
		return &DefaultDaemonSetUpdateStrategy
	}
	return updateStrategy
}

func setDeploymentAddonDefaults(updateStrategy *v3.DeploymentStrategy) *v3.DeploymentStrategy {
	if updateStrategy != nil && updateStrategy.Strategy != appsv1.RollingUpdateDeploymentStrategyType {
		return updateStrategy
	}
	if updateStrategy == nil || updateStrategy.RollingUpdate == nil {
		return &DefaultDeploymentUpdateStrategy
	}
	if updateStrategy.RollingUpdate.MaxUnavailable == nil {
		updateStrategy.RollingUpdate.MaxUnavailable = &DefaultDeploymentUpdateStrategyParams
	}
	if updateStrategy.RollingUpdate.MaxSurge == nil {
		updateStrategy.RollingUpdate.MaxSurge = &DefaultDeploymentUpdateStrategyParams
	}
	return updateStrategy
}

func setDNSDeploymentAddonDefaults(updateStrategy *v3.DeploymentStrategy, dnsProvider string) *v3.DeploymentStrategy {
	var (
		coreDNSMaxUnavailable, coreDNSMaxSurge = intstr.FromInt(1), intstr.FromInt(0)
		kubeDNSMaxSurge, kubeDNSMaxUnavailable = intstr.FromString("10%"), intstr.FromInt(0)
	)
	if updateStrategy != nil && updateStrategy.Strategy != appsv1.RollingUpdateDeploymentStrategyType &&
		updateStrategy.Strategy != "" {
		return updateStrategy
	}
	switch dnsProvider {
	case CoreDNSProvider:
		if updateStrategy == nil || updateStrategy.RollingUpdate == nil {
			return &v3.DeploymentStrategy{
				Strategy: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &coreDNSMaxUnavailable,
					MaxSurge:       &coreDNSMaxSurge,
				},
			}
		}
		if updateStrategy.RollingUpdate.MaxUnavailable == nil {
			updateStrategy.RollingUpdate.MaxUnavailable = &coreDNSMaxUnavailable
		}
	case KubeDNSProvider:
		if updateStrategy == nil || updateStrategy.RollingUpdate == nil {
			return &v3.DeploymentStrategy{
				Strategy: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &kubeDNSMaxUnavailable,
					MaxSurge:       &kubeDNSMaxSurge,
				},
			}
		}
		if updateStrategy.RollingUpdate.MaxSurge == nil {
			updateStrategy.RollingUpdate.MaxSurge = &kubeDNSMaxSurge
		}
	}

	return updateStrategy
}

func checkVersionNeedsKubeAPIAuditLog(k8sVersion string) (bool, error) {
	toMatch, err := semver.Make(k8sVersion[1:])
	if err != nil {
		return false, fmt.Errorf("Cluster version [%s] can not be parsed as semver", k8sVersion[1:])
	}
	logrus.Debugf("Checking if cluster version [%s] needs to have kube-api audit log enabled", k8sVersion[1:])
	// kube-api audit log needs to be enabled for k8s 1.15.11 and up, k8s 1.16.8 and up, k8s 1.17.4 and up
	clusterKubeAPIAuditLogRange, err := semver.ParseRange(">=1.15.11-rancher0 <=1.15.99 || >=1.16.8-rancher0 <=1.16.99 || >=1.17.4-rancher0")
	if err != nil {
		return false, errors.New("Failed to parse semver range for checking to enable kube-api audit log")
	}
	if clusterKubeAPIAuditLogRange(toMatch) {
		logrus.Debugf("Cluster version [%s] needs to have kube-api audit log enabled", k8sVersion[1:])
		return true, nil
	}
	logrus.Debugf("Cluster version [%s] does not need to have kube-api audit log enabled", k8sVersion[1:])
	return false, nil
}
