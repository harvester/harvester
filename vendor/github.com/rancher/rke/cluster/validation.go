package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/blang/semver"
	"github.com/rancher/rke/log"
	"github.com/rancher/rke/metadata"
	"github.com/rancher/rke/pki"
	"github.com/rancher/rke/services"
	"github.com/rancher/rke/util"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func (c *Cluster) ValidateCluster(ctx context.Context) error {
	// validate kubernetes version
	// Version Check
	if err := validateVersion(ctx, c); err != nil {
		return err
	}

	// validate duplicate nodes
	if err := validateDuplicateNodes(c); err != nil {
		return err
	}

	// validate hosts options
	if err := validateHostsOptions(c); err != nil {
		return err
	}

	// validate Auth options
	if err := validateAuthOptions(c); err != nil {
		return err
	}

	// validate Network options
	if err := validateNetworkOptions(c); err != nil {
		return err
	}

	// validate Ingress options
	if err := validateIngressOptions(c); err != nil {
		return err
	}

	// validate enabling CRIDockerd
	if err := validateCRIDockerdOption(c); err != nil {
		return err
	}

	// validate enabling Pod Security Policy
	if err := validatePodSecurityPolicy(c); err != nil {
		return err
	}
	// validate enabling Pod Security
	if err := validatePodSecurity(c); err != nil {
		return err
	}

	// validate services options
	return validateServicesOptions(c)
}

func validateAuthOptions(c *Cluster) error {
	for strategy, enabled := range c.AuthnStrategies {
		if !enabled {
			continue
		}
		strategy = strings.ToLower(strategy)
		if strategy != AuthnX509Provider && strategy != AuthnWebhookProvider {
			return fmt.Errorf("Authentication strategy [%s] is not supported", strategy)
		}
	}
	if !c.AuthnStrategies[AuthnX509Provider] {
		return fmt.Errorf("Authentication strategy must contain [%s]", AuthnX509Provider)
	}
	return nil
}

func transformAciNetworkOption(option string) (string, string) {
	var description string
	switch option {
	case AciSystemIdentifier:
		option = "system_id"
		description = "unique suffix for all cluster related objects in aci"
	case AciServiceGraphSubnet:
		option = "node_svc_subnet"
		description = "Subnet to use for service graph endpoints on aci"
	case AciStaticExternalSubnet:
		option = "extern_static"
		description = "Subnet to use for static external IPs on aci"
	case AciDynamicExternalSubnet:
		option = "extern_dynamic"
		description = "Subnet to use for dynamic external IPs on aci"
	case AciToken:
		description = "UUID for this version of the input configuration"
	case AciApicUserName:
		description = "User name for aci apic"
	case AciApicUserKey:
		description = "Base64 encoded private key for aci apic user"
	case AciApicUserCrt:
		description = "Base64 encoded certificate for aci apic user"
	case AciEncapType:
		description = "One of the supported encap types for aci(vlan/vxlan)"
	case AciMcastRangeStart:
		description = "Mcast range start address for endpoint groups on aci"
	case AciMcastRangeEnd:
		description = "Mcast range end address for endpoint groups on aci"
	case AciNodeSubnet:
		description = "Kubernetes node address subnet"
	case AciAEP:
		description = "Attachment entity profile name on aci"
	case AciVRFName:
		description = "VRF Name on aci"
	case AciVRFTenant:
		description = "Tenant for VRF on aci"
	case AciL3Out:
		description = "L3Out on aci"
	case AciKubeAPIVlan:
		description = "Vlan for node network on aci"
	case AciServiceVlan:
		description = "Vlan for service graph nodes on aci"
	case AciInfraVlan:
		description = "Vlan for infra network on aci"
	}
	return option, description
}

func validateAciCloudOptionsDisabled(option string, value string) (string, string, bool) {
	var description string
	ok := false
	switch option {
	case AciUseOpflexServerVolume:
		if value == DefaultAciUseOpflexServerVolume {
			ok = true
		}
		description = "Use mounted volume for opflex server"
	case AciUseHostNetnsVolume:
		if value == DefaultAciUseHostNetnsVolume {
			ok = true
		}
		description = "Mount host netns for opflex server"
	case AciCApic:
		if value == DefaultAciCApic {
			ok = true
		}
		description = "Provision cloud apic"
	case AciUseAciAnywhereCRD:
		if value == DefaultAciUseAciAnywhereCRD {
			ok = true
		}
		description = "Use Aci anywhere CRD"
	case AciRunGbpContainer:
		if value == DefaultAciRunGbpContainer {
			ok = true
		}
		description = "Run Gbp Server"
	case AciRunOpflexServerContainer:
		if value == DefaultAciRunOpflexServerContainer {
			ok = true
		}
		description = "Run Opflex Server"
	case AciEpRegistry:
		if value == "" {
			ok = true
		}
		description = "Registry for Ep whether CRD or MODB"
	case AciOpflexMode:
		if value == "" {
			ok = true
		}
		description = "Opflex overlay mode or on-prem"
	case AciSubnetDomainName:
		if value == "" {
			ok = true
		}
		description = "Subnet domain name"
	case AciKafkaClientCrt:
		if value == "" {
			ok = true
		}
		description = "CApic Kafka client certificate"
	case AciKafkaClientKey:
		if value == "" {
			ok = true
		}
		description = "CApic Kafka client key"
	case AciOverlayVRFName:
		if value == "" {
			ok = true
		}
		description = "Overlay VRF name"
	case AciGbpPodSubnet:
		if value == "" {
			ok = true
		}
		description = "Gbp pod subnet"
	case AciOpflexServerPort:
		if value == "" {
			ok = true
		}
		description = "Opflex server port"
	}
	return option, description, ok
}

func validateNetworkOptions(c *Cluster) error {
	if c.Network.Plugin != NoNetworkPlugin && c.Network.Plugin != FlannelNetworkPlugin && c.Network.Plugin != CalicoNetworkPlugin && c.Network.Plugin != CanalNetworkPlugin && c.Network.Plugin != WeaveNetworkPlugin && c.Network.Plugin != AciNetworkPlugin {
		return fmt.Errorf("Network plugin [%s] is not supported", c.Network.Plugin)
	}
	if c.Network.Plugin == FlannelNetworkPlugin && c.Network.MTU != 0 {
		return fmt.Errorf("Network plugin [%s] does not support configuring MTU", FlannelNetworkPlugin)
	}

	if c.Network.Plugin == WeaveNetworkPlugin {
		if err := warnWeaveDeprecation(c.Version); err != nil {
			return fmt.Errorf("Error while printing Weave deprecation message: %w", err)
		}
	}

	dualStack := false
	serviceClusterRanges := strings.Split(c.Services.KubeAPI.ServiceClusterIPRange, ",")
	if len(serviceClusterRanges) > 1 {
		logrus.Debugf("Found more than 1 service cluster IP range, assuming dual stack")
		dualStack = true
	}
	clusterCIDRs := strings.Split(c.Services.KubeController.ClusterCIDR, ",")
	if len(clusterCIDRs) > 1 {
		logrus.Debugf("Found more than 1 cluster CIDR, assuming dual stack")
		dualStack = true
	}
	if dualStack {
		IPv6CompatibleNetworkPluginFound := false
		for _, networkPlugin := range IPv6CompatibleNetworkPlugins {
			if c.Network.Plugin == networkPlugin {
				logrus.Debugf("Found IPv6 compatible network plugin [%s] == [%s]", c.Network.Plugin, networkPlugin)
				IPv6CompatibleNetworkPluginFound = true
				break
			}
		}
		if !IPv6CompatibleNetworkPluginFound {
			return fmt.Errorf("Network plugin [%s] does not support IPv6 (dualstack)", c.Network.Plugin)
		}
		if c.Network.Plugin == AciNetworkPlugin {
			k8sVersion := c.RancherKubernetesEngineConfig.Version
			toMatch, err := semver.Make(k8sVersion[1:])
			if err != nil {
				return fmt.Errorf("Cluster version [%s] is not valid semver", k8sVersion)
			}
			logrus.Debugf("Checking if cluster version [%s] has dualstack supported aci cni version", k8sVersion)
			//k8s version needs to have aci version >= 5.2.7.1
			clusterDualstackAciRange, err := semver.ParseRange(">=1.23.16-rancher2-3 <=1.23.99 || >=1.24.13-rancher2-2 <=1.24.99 || >=1.25.9-rancher2-2")
			if err != nil {
				return errors.New("Failed to parse semver range for checking dualstack supported aci cni versions")
			}
			if !clusterDualstackAciRange(toMatch) {
				return fmt.Errorf("Cluster version [%s] does not have dualstack supported aci cni version", k8sVersion)
			}
			logrus.Debugf("Cluster version [%s] has dualstack supported aci cni version", k8sVersion)
		}
	}

	if c.Network.Plugin == AciNetworkPlugin {
		// Skip cloud options and throw an error.
		cloudOptionsList := []string{AciEpRegistry, AciOpflexMode, AciUseHostNetnsVolume, AciUseOpflexServerVolume,
			AciSubnetDomainName, AciKafkaClientCrt, AciKafkaClientKey, AciCApic, UseAciAnywhereCRD,
			AciOverlayVRFName, AciGbpPodSubnet, AciRunGbpContainer, AciRunOpflexServerContainer, AciOpflexServerPort}
		for _, v := range cloudOptionsList {
			val, ok := c.Network.Options[v]
			_, _, disabled := validateAciCloudOptionsDisabled(v, val)
			if ok && !disabled {
				return fmt.Errorf("Network plugin aci: %s = %s is provided,but cloud options are not allowed in this release", v, val)
			}
		}

		networkOptionsList := []string{AciSystemIdentifier, AciToken, AciApicUserName, AciApicUserKey,
			AciApicUserCrt, AciEncapType, AciMcastRangeStart, AciMcastRangeEnd,
			AciNodeSubnet, AciAEP, AciVRFName, AciVRFTenant, AciL3Out, AciDynamicExternalSubnet,
			AciStaticExternalSubnet, AciServiceGraphSubnet, AciKubeAPIVlan, AciServiceVlan, AciInfraVlan}
		for _, v := range networkOptionsList {
			val, ok := c.Network.Options[v]
			if !ok || val == "" {
				var description string
				v, description = transformAciNetworkOption(v)
				return fmt.Errorf("Network plugin aci: %s(%s) under aci_network_provider is not provided", strings.TrimPrefix(v, "aci_"), description)
			}
		}
		if c.Network.AciNetworkProvider != nil {
			if c.Network.AciNetworkProvider.ApicHosts == nil {
				return fmt.Errorf("Network plugin aci: %s(address of aci apic hosts) under aci_network_provider is not provided", "apic_hosts")
			}
			if c.Network.AciNetworkProvider.L3OutExternalNetworks == nil {
				return fmt.Errorf("Network plugin aci: %s(external network name/s on aci) under aci_network_provider is not provided", "l3out_external_networks")
			}
		} else {
			var requiredArgs []string
			for _, v := range networkOptionsList {
				v, _ = transformAciNetworkOption(v)
				requiredArgs = append(requiredArgs, fmt.Sprintf(" %s", strings.TrimPrefix("aci_", v)))
			}
			requiredArgs = append(requiredArgs, fmt.Sprintf(" %s", ApicHosts))
			requiredArgs = append(requiredArgs, fmt.Sprintf(" %s", L3OutExternalNetworks))
			return fmt.Errorf("Network plugin aci: multiple parameters under aci_network_provider are not provided: %s", requiredArgs)
		}
	}
	return nil
}

func validateHostsOptions(c *Cluster) error {
	for i, host := range c.Nodes {
		if len(host.Address) == 0 {
			return fmt.Errorf("Address for host (%d) is not provided", i+1)
		}
		if len(host.User) == 0 {
			return fmt.Errorf("User for host (%d) is not provided", i+1)
		}
		if len(host.Role) == 0 {
			return fmt.Errorf("Role for host (%d) is not provided", i+1)
		}
		if errs := validation.IsDNS1123Subdomain(host.HostnameOverride); len(errs) > 0 {
			return fmt.Errorf("Hostname_override [%s] for host (%d) is not valid: %v", host.HostnameOverride, i+1, errs)
		}
		for _, role := range host.Role {
			if role != services.ETCDRole && role != services.ControlRole && role != services.WorkerRole {
				return fmt.Errorf("Role [%s] for host (%d) is not recognized", role, i+1)
			}
		}
	}
	return nil
}

func validateServicesOptions(c *Cluster) error {
	servicesOptions := map[string]string{
		"etcd_image":                               c.Services.Etcd.Image,
		"kube_api_image":                           c.Services.KubeAPI.Image,
		"kube_api_service_cluster_ip_range":        c.Services.KubeAPI.ServiceClusterIPRange,
		"kube_controller_image":                    c.Services.KubeController.Image,
		"kube_controller_service_cluster_ip_range": c.Services.KubeController.ServiceClusterIPRange,
		"kube_controller_cluster_cidr":             c.Services.KubeController.ClusterCIDR,
		"scheduler_image":                          c.Services.Scheduler.Image,
		"kubelet_image":                            c.Services.Kubelet.Image,
		"kubelet_cluster_dns_service":              c.Services.Kubelet.ClusterDNSServer,
		"kubelet_cluster_domain":                   c.Services.Kubelet.ClusterDomain,
		"kubelet_infra_container_image":            c.Services.Kubelet.InfraContainerImage,
		"kubeproxy_image":                          c.Services.Kubeproxy.Image,
	}
	for optionName, OptionValue := range servicesOptions {
		if len(OptionValue) == 0 {
			return fmt.Errorf("%s can't be empty", strings.Join(strings.Split(optionName, "_"), " "))
		}
	}
	// Validate external etcd information
	if len(c.Services.Etcd.ExternalURLs) > 0 {
		if len(c.Services.Etcd.CACert) == 0 {
			return errors.New("External CA Certificate for etcd can't be empty")
		}
		if len(c.Services.Etcd.Cert) == 0 {
			return errors.New("External Client Certificate for etcd can't be empty")
		}
		if len(c.Services.Etcd.Key) == 0 {
			return errors.New("External Client Key for etcd can't be empty")
		}
		if len(c.Services.Etcd.Path) == 0 {
			return errors.New("External etcd path can't be empty")
		}
	}

	// validate etcd s3 backup backend configurations
	return validateEtcdBackupOptions(c)
}

func validateEtcdBackupOptions(c *Cluster) error {
	if c.Services.Etcd.BackupConfig != nil {
		if c.Services.Etcd.BackupConfig.S3BackupConfig != nil {
			if len(c.Services.Etcd.BackupConfig.S3BackupConfig.Endpoint) == 0 {
				return errors.New("etcd s3 backup backend endpoint can't be empty")
			}
			if len(c.Services.Etcd.BackupConfig.S3BackupConfig.BucketName) == 0 {
				return errors.New("etcd s3 backup backend bucketName can't be empty")
			}
			if len(c.Services.Etcd.BackupConfig.S3BackupConfig.CustomCA) != 0 {
				if isValid, err := pki.IsValidCertStr(c.Services.Etcd.BackupConfig.S3BackupConfig.CustomCA); !isValid {
					return fmt.Errorf("invalid S3 endpoint CA certificate: %v", err)
				}
			}
		}
	}
	return nil
}

func validateIngressOptions(c *Cluster) error {
	// Should be changed when adding more ingress types
	if c.Ingress.Provider != DefaultIngressController && c.Ingress.Provider != "none" {
		return fmt.Errorf("Ingress controller %s is incorrect", c.Ingress.Provider)
	}

	if c.Ingress.DNSPolicy != "" &&
		!(c.Ingress.DNSPolicy == string(v1.DNSClusterFirst) ||
			c.Ingress.DNSPolicy == string(v1.DNSClusterFirstWithHostNet) ||
			c.Ingress.DNSPolicy == string(v1.DNSNone) ||
			c.Ingress.DNSPolicy == string(v1.DNSDefault)) {
		return fmt.Errorf("DNSPolicy %s was not a valid DNS Policy", c.Ingress.DNSPolicy)
	}

	if c.Ingress.NetworkMode == "hostPort" {
		if !(c.Ingress.HTTPSPort >= 0 && c.Ingress.HTTPSPort <= 65535) {
			return fmt.Errorf("https port is invalid. Needs to be within 0 to 65535")
		}
		if !(c.Ingress.HTTPPort >= 0 && c.Ingress.HTTPPort <= 65535) {
			return fmt.Errorf("http port is invalid. Needs to be within 0 to 65535")
		}
		if c.Ingress.HTTPPort != 0 && c.Ingress.HTTPSPort != 0 &&
			(c.Ingress.HTTPPort == c.Ingress.HTTPSPort) {
			return fmt.Errorf("http and https ports need to be different")
		}
	}

	if !(c.Ingress.NetworkMode == "" ||
		c.Ingress.NetworkMode == "hostNetwork" || c.Ingress.NetworkMode == "hostPort" ||
		c.Ingress.NetworkMode == "none") {
		return fmt.Errorf("NetworkMode %s was not a valid network mode", c.Ingress.NetworkMode)
	}
	return nil
}

func ValidateHostCount(c *Cluster) error {
	if len(c.EtcdHosts) == 0 && len(c.Services.Etcd.ExternalURLs) == 0 {
		failedEtcdHosts := []string{}
		for _, host := range c.InactiveHosts {
			if host.IsEtcd {
				failedEtcdHosts = append(failedEtcdHosts, host.Address)
			}
			return fmt.Errorf("Cluster must have at least one etcd plane host: failed to connect to the following etcd host(s) %v", failedEtcdHosts)
		}
		return errors.New("Cluster must have at least one etcd plane host: please specify one or more etcd in cluster config")
	}
	if len(c.EtcdHosts) > 0 && len(c.Services.Etcd.ExternalURLs) > 0 {
		return errors.New("Cluster can't have both internal and external etcd")
	}
	return nil
}

func validateDuplicateNodes(c *Cluster) error {
	addresses := make(map[string]struct{}, len(c.Nodes))
	hostnames := make(map[string]struct{}, len(c.Nodes))
	for i := range c.Nodes {
		if _, ok := addresses[c.Nodes[i].Address]; ok {
			return fmt.Errorf("Cluster can't have duplicate node: %s", c.Nodes[i].Address)
		}
		addresses[c.Nodes[i].Address] = struct{}{}
		if _, ok := hostnames[c.Nodes[i].HostnameOverride]; ok {
			return fmt.Errorf("Cluster can't have duplicate node: %s", c.Nodes[i].HostnameOverride)
		}
		hostnames[c.Nodes[i].HostnameOverride] = struct{}{}
	}
	return nil
}

func validateVersion(ctx context.Context, c *Cluster) error {
	_, err := util.StrToSemVer(c.Version)
	if err != nil {
		return fmt.Errorf("%s is not valid semver", c.Version)
	}
	_, ok := metadata.K8sVersionToRKESystemImages[c.Version]
	if !ok {
		if err := validateSystemImages(c); err != nil {
			return fmt.Errorf("%s is an unsupported Kubernetes version and system images are not populated: %v", c.Version, err)
		}
		return nil
	}

	if _, ok := metadata.K8sBadVersions[c.Version]; ok {
		log.Warnf(ctx, "%s version exists but its recommended to install this version - see 'rke config --system-images --all' for versions supported with this release", c.Version)
		return fmt.Errorf("%s is an unsupported Kubernetes version and system images are not populated: %v", c.Version, err)
	}

	return nil
}

func validateSystemImages(c *Cluster) error {
	if err := validateKubernetesImages(c); err != nil {
		return err
	}
	if err := validateNetworkImages(c); err != nil {
		return err
	}
	if err := validateDNSImages(c); err != nil {
		return err
	}
	if err := validateMetricsImages(c); err != nil {
		return err
	}
	return validateIngressImages(c)
}

func validateKubernetesImages(c *Cluster) error {
	if len(c.SystemImages.Etcd) == 0 {
		return errors.New("etcd image is not populated")
	}
	if len(c.SystemImages.Kubernetes) == 0 {
		return errors.New("kubernetes image is not populated")
	}
	if len(c.SystemImages.PodInfraContainer) == 0 {
		return errors.New("pod infrastructure container image is not populated")
	}
	if len(c.SystemImages.Alpine) == 0 {
		return errors.New("alpine image is not populated")
	}
	if len(c.SystemImages.NginxProxy) == 0 {
		return errors.New("nginx proxy image is not populated")
	}
	if len(c.SystemImages.CertDownloader) == 0 {
		return errors.New("certificate downloader image is not populated")
	}
	if len(c.SystemImages.KubernetesServicesSidecar) == 0 {
		return errors.New("kubernetes sidecar image is not populated")
	}
	return nil
}

func validateNetworkImages(c *Cluster) error {
	// check network provider images
	if c.Network.Plugin == FlannelNetworkPlugin {
		if len(c.SystemImages.Flannel) == 0 {
			return errors.New("flannel image is not populated")
		}
		if len(c.SystemImages.FlannelCNI) == 0 {
			return errors.New("flannel cni image is not populated")
		}
	} else if c.Network.Plugin == CanalNetworkPlugin {
		if len(c.SystemImages.CanalNode) == 0 {
			return errors.New("canal image is not populated")
		}
		if len(c.SystemImages.CanalCNI) == 0 {
			return errors.New("canal cni image is not populated")
		}
		if len(c.SystemImages.CanalFlannel) == 0 {
			return errors.New("flannel image is not populated")
		}
	} else if c.Network.Plugin == CalicoNetworkPlugin {
		if len(c.SystemImages.CalicoCNI) == 0 {
			return errors.New("calico cni image is not populated")
		}
		if len(c.SystemImages.CalicoCtl) == 0 {
			return errors.New("calico ctl image is not populated")
		}
		if len(c.SystemImages.CalicoNode) == 0 {
			return errors.New("calico image is not populated")
		}
		if len(c.SystemImages.CalicoControllers) == 0 {
			return errors.New("calico controllers image is not populated")
		}
	} else if c.Network.Plugin == WeaveNetworkPlugin {
		if len(c.SystemImages.WeaveCNI) == 0 {
			return errors.New("weave cni image is not populated")
		}
		if len(c.SystemImages.WeaveNode) == 0 {
			return errors.New("weave image is not populated")
		}
	} else if c.Network.Plugin == AciNetworkPlugin {
		if len(c.SystemImages.AciCniDeployContainer) == 0 {
			return errors.New("aci cnideploy image is not populated")
		}
		if len(c.SystemImages.AciHostContainer) == 0 {
			return errors.New("aci host container image is not populated")
		}
		if len(c.SystemImages.AciOpflexContainer) == 0 {
			return errors.New("aci opflex agent image is not populated")
		}
		if len(c.SystemImages.AciMcastContainer) == 0 {
			return errors.New("aci mcast container image is not populated")
		}
		if len(c.SystemImages.AciOpenvSwitchContainer) == 0 {
			return errors.New("aci openvswitch image is not populated")
		}
		if len(c.SystemImages.AciControllerContainer) == 0 {
			return errors.New("aci controller image is not populated")
		}
		// Skipping Cloud image validation.
		// c.SystemImages.AciOpflexServerContainer
		// c.SystemImages.AciGbpServerContainer
	}
	return nil
}

func validateDNSImages(c *Cluster) error {
	// check dns provider images
	if c.DNS.Provider == "kube-dns" {
		if len(c.SystemImages.KubeDNS) == 0 {
			return errors.New("kubedns image is not populated")
		}
		if len(c.SystemImages.DNSmasq) == 0 {
			return errors.New("dnsmasq image is not populated")
		}
		if len(c.SystemImages.KubeDNSSidecar) == 0 {
			return errors.New("kubedns sidecar image is not populated")
		}
		if len(c.SystemImages.KubeDNSAutoscaler) == 0 {
			return errors.New("kubedns autoscaler image is not populated")
		}
	} else if c.DNS.Provider == "coredns" {
		if len(c.SystemImages.CoreDNS) == 0 {
			return errors.New("coredns image is not populated")
		}
		if len(c.SystemImages.CoreDNSAutoscaler) == 0 {
			return errors.New("coredns autoscaler image is not populated")
		}
	}
	if c.DNS.Nodelocal != nil && len(c.SystemImages.Nodelocal) == 0 {
		return errors.New("nodelocal image is not populated")
	}
	return nil
}

func validateMetricsImages(c *Cluster) error {
	// checl metrics server image
	if c.Monitoring.Provider != "none" {
		if len(c.SystemImages.MetricsServer) == 0 {
			return errors.New("metrics server images is not populated")
		}
	}
	return nil
}

func validateIngressImages(c *Cluster) error {
	// check ingress images
	if c.Ingress.Provider != "none" {
		if len(c.SystemImages.Ingress) == 0 {
			return errors.New("ingress image is not populated")
		}
		if len(c.SystemImages.IngressBackend) == 0 {
			return errors.New("ingress backend image is not populated")
		}
		// Ingress Webhook image is used starting with k8s 1.21
		k8sVersion := c.RancherKubernetesEngineConfig.Version
		toMatch, err := semver.Make(k8sVersion[1:])
		if err != nil {
			return fmt.Errorf("%s is not valid semver", k8sVersion)
		}
		logrus.Debugf("Checking if ingress webhook image is required for cluster version [%s]", k8sVersion)

		IngressWebhookImageRequiredRange, err := semver.ParseRange(">=1.21.0-rancher0")
		if err != nil {
			logrus.Warnf("Failed to parse semver range for checking required ingress webhook image")
		}
		if IngressWebhookImageRequiredRange(toMatch) {
			logrus.Debugf("Cluster version [%s] uses ingress webhook image, checking if image is populated", k8sVersion)
			if len(c.SystemImages.IngressWebhook) == 0 {
				return errors.New("ingress webhook image is not populated")
			}

		}
	}
	return nil
}

func validateCRIDockerdOption(c *Cluster) error {
	if c.EnableCRIDockerd != nil && *c.EnableCRIDockerd {
		k8sVersion := c.RancherKubernetesEngineConfig.Version
		parsedVersion, err := getClusterVersion(k8sVersion)
		if err != nil {
			return err
		}
		logrus.Debugf("Checking cri-dockerd for cluster version [%s]", k8sVersion)
		// cri-dockerd can be enabled for k8s 1.21 and up
		CRIDockerdAllowedRange, err := semver.ParseRange(">=1.21.0-rancher0")
		if err != nil {
			logrus.Warnf("Failed to parse semver range for checking cri-dockerd")
		}
		if !CRIDockerdAllowedRange(parsedVersion) {
			logrus.Debugf("Cluster version [%s] is not allowed to enable cri-dockerd", k8sVersion)
			return fmt.Errorf("Enabling cri-dockerd for cluster version [%s] is not supported", k8sVersion)
		}
		logrus.Debugf("cri-dockerd is enabled for cluster version [%s]", k8sVersion)
	}
	return nil
}

func validatePodSecurityPolicy(c *Cluster) error {
	parsedVersion, err := getClusterVersion(c.Version)
	if err != nil {
		logrus.Warnf("Failed to parse semver range for validating Pod Security Policy")
		return err
	}
	logrus.Debugf("Checking PodSecurityPolicy for cluster version [%s]", c.Version)
	if c.Services.KubeAPI.PodSecurityPolicy {
		if c.Authorization.Mode != services.RBACAuthorizationMode {
			return errors.New("PodSecurityPolicy can't be enabled with RBAC support disabled")
		}
		if parsedRangeAtLeast125(parsedVersion) {
			return errors.New("PodSecurityPolicy has been removed and can not be enabled since k8s v1.25")
		}
	}
	return nil
}

func validatePodSecurity(c *Cluster) error {
	parsedVersion, err := getClusterVersion(c.Version)
	if err != nil {
		logrus.Warnf("Failed to parse semver range for validating Pod Security")
		return err
	}
	logrus.Debugf("Checking PodSecurity for cluster version [%s]", c.Version)
	// The following requirements must be met to set the default Pod Security Admission Config:
	// - RBAC is enabled on the cluster
	// - Cluster version is at least 1.23
	// - valid values are privileged and restricted
	level := c.Services.KubeAPI.PodSecurityConfiguration
	if len(level) != 0 {
		if c.Authorization.Mode != services.RBACAuthorizationMode {
			return errors.New("PodSecurity can't be enabled with RBAC support disabled")
		}
		if !parsedRangeAtLeast123(parsedVersion) {
			return errors.New("cluster version must be at least v1.23 to use PodSecurity in RKE")
		}
		if level != PodSecurityPrivileged && level != PodSecurityRestricted {
			return fmt.Errorf("invalid pod_security_configuration [%s]. Supported values: [%s, %s]",
				level, PodSecurityPrivileged, PodSecurityRestricted)
		}
	}
	return nil
}

func getClusterVersion(version string) (semver.Version, error) {
	var parsedVersion semver.Version
	if len(version) <= 1 || !strings.HasPrefix(version, "v") {
		return parsedVersion, fmt.Errorf("%s is not valid version", version)
	}
	parsedVersion, err := semver.Parse(version[1:])
	if err != nil {
		return parsedVersion, fmt.Errorf("%s is not valid semver", version)
	}
	return parsedVersion, nil
}

// warnWeaveDeprecation prints a deprecation warning if version is higher than 1.27
func warnWeaveDeprecation(k8sVersion string) error {
	version, err := util.StrToSemVer(k8sVersion)
	if err != nil {
		return fmt.Errorf("error while parsing cluster version [%s]: %w", k8sVersion, err)
	}
	version127, err := util.StrToSemVer("v1.27.0")
	if err != nil {
		return fmt.Errorf("failed to translate v1.27.0 to semver notation: %w", err)
	}
	if !version.LessThan(*version127) {
		logrus.Warn("Weave CNI plugin is deprecated starting with Kubernetes v1.27 and will be removed in Kubernetes v1.30")
	}
	return nil
}
