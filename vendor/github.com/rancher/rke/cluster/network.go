package cluster

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	cidr "github.com/apparentlymart/go-cidr/cidr"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/rancher/rke/docker"
	"github.com/rancher/rke/hosts"
	"github.com/rancher/rke/log"
	"github.com/rancher/rke/pki"
	"github.com/rancher/rke/templates"
	v3 "github.com/rancher/rke/types"
	"github.com/rancher/rke/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
)

const (
	NetworkPluginResourceName = "rke-network-plugin"

	PortCheckContainer        = "rke-port-checker"
	EtcdPortListenContainer   = "rke-etcd-port-listener"
	CPPortListenContainer     = "rke-cp-port-listener"
	WorkerPortListenContainer = "rke-worker-port-listener"

	KubeAPIPort      = "6443"
	EtcdPort1        = "2379"
	EtcdPort2        = "2380"
	KubeletPort      = "10250"
	FlannelVxLanPort = 8472

	FlannelVxLanNetworkIdentify = 1

	ProtocolTCP = "TCP"
	ProtocolUDP = "UDP"

	NoNetworkPlugin = "none"

	FlannelNetworkPlugin = "flannel"
	FlannelIface         = "flannel_iface"
	FlannelBackendType   = "flannel_backend_type"
	// FlannelBackendPort must be 4789 if using VxLan mode in the cluster with Windows nodes
	FlannelBackendPort = "flannel_backend_port"
	// FlannelBackendVxLanNetworkIdentify should be greater than or equal to 4096 if using VxLan mode in the cluster with Windows nodes
	FlannelBackendVxLanNetworkIdentify  = "flannel_backend_vni"
	KubeFlannelPriorityClassNameKeyName = "kube_flannel_priority_class_name"

	CalicoNetworkPlugin                           = "calico"
	CalicoNodeLabel                               = "calico-node"
	CalicoControllerLabel                         = "calico-kube-controllers"
	CalicoCloudProvider                           = "calico_cloud_provider"
	CalicoFlexVolPluginDirectory                  = "calico_flex_volume_plugin_dir"
	CalicoNodePriorityClassNameKeyName            = "calico_node_priority_class_name"
	CalicoKubeControllersPriorityClassNameKeyName = "calico_kube_controllers_priority_class_name"

	CanalNetworkPlugin      = "canal"
	CanalIface              = "canal_iface"
	CanalFlannelBackendType = "canal_flannel_backend_type"
	// CanalFlannelBackendPort must be 4789 if using Flannel VxLan mode in the cluster with Windows nodes
	CanalFlannelBackendPort = "canal_flannel_backend_port"
	// CanalFlannelBackendVxLanNetworkIdentify should be greater than or equal to 4096 if using Flannel VxLan mode in the cluster with Windows nodes
	CanalFlannelBackendVxLanNetworkIdentify = "canal_flannel_backend_vni"
	CanalFlexVolPluginDirectory             = "canal_flex_volume_plugin_dir"
	CanalPriorityClassNameKeyName           = "canal_priority_class_name"

	WeaveNetworkPlugin               = "weave"
	WeaveNetworkAppName              = "weave-net"
	WeaveNetPriorityClassNameKeyName = "weave_net_priority_class_name"

	AciNetworkPlugin                        = "aci"
	AciOVSMemoryLimit                       = "aci_ovs_memory_limit"
	AciOVSMemoryRequest                     = "aci_ovs_memory_request"
	AciImagePullPolicy                      = "aci_image_pull_policy"
	AciPBRTrackingNonSnat                   = "aci_pbr_tracking_non_snat"
	AciInstallIstio                         = "aci_install_istio"
	AciIstioProfile                         = "aci_istio_profile"
	AciDropLogEnable                        = "aci_drop_log_enable"
	AciControllerLogLevel                   = "aci_controller_log_level"
	AciHostAgentLogLevel                    = "aci_host_agent_log_level"
	AciOpflexAgentLogLevel                  = "aci_opflex_agent_log_level"
	AciApicRefreshTime                      = "aci_apic_refresh_time"
	AciServiceMonitorInterval               = "aci_server_monitor_interval"
	AciSystemIdentifier                     = "aci_system_identifier"
	AciToken                                = "aci_token"
	AciApicUserName                         = "aci_apic_user_name"
	AciApicUserKey                          = "aci_apic_user_key"
	AciApicUserCrt                          = "aci_apic_user_crt"
	AciVmmDomain                            = "aci_vmm_domain"
	AciVmmController                        = "aci_vmm_controller"
	AciEncapType                            = "aci_encap_type"
	AciAEP                                  = "aci_aep"
	AciVRFName                              = "aci_vrf_name"
	AciVRFTenant                            = "aci_vrf_tenant"
	AciL3Out                                = "aci_l3out"
	AciDynamicExternalSubnet                = "aci_dynamic_external_subnet"
	AciStaticExternalSubnet                 = "aci_static_external_subnet"
	AciServiceGraphSubnet                   = "aci_service_graph_subnet"
	AciKubeAPIVlan                          = "aci_kubeapi_vlan"
	AciServiceVlan                          = "aci_service_vlan"
	AciInfraVlan                            = "aci_infra_vlan"
	AciImagePullSecret                      = "aci_image_pull_secret"
	AciTenant                               = "aci_tenant"
	AciNodeSubnet                           = "aci_node_subnet"
	AciMcastRangeStart                      = "aci_mcast_range_start"
	AciMcastRangeEnd                        = "aci_mcast_range_end"
	AciUseAciCniPriorityClass               = "aci_use_aci_cni_priority_class"
	AciNoPriorityClass                      = "aci_no_priority_class"
	AciMaxNodesSvcGraph                     = "aci_max_nodes_svc_graph"
	AciSnatContractScope                    = "aci_snat_contract_scope"
	AciPodSubnetChunkSize                   = "aci_pod_subnet_chunk_size"
	AciEnableEndpointSlice                  = "aci_enable_endpoint_slice"
	AciSnatNamespace                        = "aci_snat_namespace"
	AciEpRegistry                           = "aci_ep_registry"
	AciOpflexMode                           = "aci_opflex_mode"
	AciSnatPortRangeStart                   = "aci_snat_port_range_start"
	AciSnatPortRangeEnd                     = "aci_snat_port_range_end"
	AciSnatPortsPerNode                     = "aci_snat_ports_per_node"
	AciOpflexClientSSL                      = "aci_opflex_client_ssl"
	AciUsePrivilegedContainer               = "aci_use_privileged_container"
	AciUseHostNetnsVolume                   = "aci_use_host_netns_volume"
	AciUseOpflexServerVolume                = "aci_use_opflex_server_volume"
	AciKafkaClientCrt                       = "aci_kafka_client_crt"
	AciKafkaClientKey                       = "aci_kafka_client_key"
	AciSubnetDomainName                     = "aci_subnet_domain_name"
	AciCApic                                = "aci_capic"
	AciUseAciAnywhereCRD                    = "aci_use_aci_anywhere_crd"
	AciOverlayVRFName                       = "aci_overlay_vrf_name"
	AciGbpPodSubnet                         = "aci_gbp_pod_subnet"
	AciRunGbpContainer                      = "aci_run_gbp_container"
	AciRunOpflexServerContainer             = "aci_run_opflex_server_container"
	AciOpflexServerPort                     = "aci_opflex_server_port"
	AciDurationWaitForNetwork               = "aci_duration_wait_for_network"
	AciDisableWaitForNetwork                = "aci_disable_wait_for_network"
	AciUseClusterRole                       = "aci_use_cluster_role"
	AciApicSubscriptionDelay                = "aci_apic_subscription_delay"
	AciApicRefreshTickerAdjust              = "aci_apic_refresh_ticker_adjust"
	AciDisablePeriodicSnatGlobalInfoSync    = "aci_disable_periodic_snat_global_info_sync"
	AciOpflexDeviceDeleteTimeout            = "aci_opflex_device_delete_timeout"
	AciMTUHeadRoom                          = "aci_mtu_head_room"
	AciNodePodIfEnable                      = "aci_node_pod_if_enable"
	AciSriovEnable                          = "aci_sriov_enable"
	AciMultusDisable                        = "aci_multus_disable"
	AciNoWaitForServiceEpReadiness          = "aci_no_wait_for_service_ep_readiness"
	AciAddExternalSubnetsToRdconfig         = "aci_add_external_subnets_to_rdconfig"
	AciServiceGraphEndpointAddDelay         = "aci_service_graph_endpoint_add_delay"
	AciHppOptimization                      = "aci_hpp_optimization"
	AciSleepTimeSnatGlobalInfoSync          = "aci_sleep_time_snat_global_info_sync"
	AciOpflexAgentOpflexAsyncjsonEnabled    = "aci_opflex_agent_opflex_asyncjson_enabled"
	AciOpflexAgentOvsAsyncjsonEnabled       = "aci_opflex_agent_ovs_asyncjson_enabled"
	AciOpflexAgentPolicyRetryDelayTimer     = "aci_opflex_agent_policy_retry_delay_timer"
	AciAciMultipod                          = "aci_aci_multipod"
	AciOpflexDeviceReconnectWaitTimeout     = "aci_opflex_device_reconnect_wait_timeout"
	AciAciMultipodUbuntu                    = "aci_aci_multipod_ubuntu"
	AciDhcpRenewMaxRetryCount               = "aci_dhcp_renew_max_retry_count"
	AciDhcpDelay                            = "aci_dhcp_delay"
	AciUseSystemNodePriorityClass           = "aci_use_system_node_priority_class"
	AciAciContainersControllerMemoryRequest = "aci_aci_containers_controller_memory_request"
	AciAciContainersControllerMemoryLimit   = "aci_aci_containers_controller_memory_limit"
	AciAciContainersHostMemoryRequest       = "aci_aci_containers_host_memory_request"
	AciAciContainersHostMemoryLimit         = "aci_aci_containers_host_memory_limit"
	AciMcastDaemonMemoryRequest             = "aci_mcast_daemon_memory_request"
	AciMcastDaemonMemoryLimit               = "aci_mcast_daemon_memory_limit"
	AciOpflexAgentMemoryRequest             = "aci_opflex_agent_memory_request"
	AciOpflexAgentMemoryLimit               = "aci_opflex_agent_memory_limit"
	AciAciContainersMemoryRequest           = "aci_aci_containers_memory_request"
	AciAciContainersMemoryLimit             = "aci_aci_containers_memory_limit"
	AciOpflexAgentStatistics                = "aci_opflex_agent_statistics"
	AciAddExternalContractToDefaultEpg      = "aci_add_external_contract_to_default_epg"
	AciEnableOpflexAgentReconnect           = "aci_enable_opflex_agent_reconnect"
	AciOpflexOpensslCompat                  = "aci_opflex_openssl_compat"
	AciTolerationSeconds                    = "aci_toleration_seconds"
	AciDisableHppRendering                  = "aci_disable_hpp_rendering"
	AciApicConnectionRetryLimit             = "aci_apic_connection_retry_limit"
	AciTaintNotReadyNode                    = "aci_taint_not_ready_node"
	AciDropLogDisableEvents                 = "aci_drop_log_disable_events"
	// List of map keys to be used with network templates

	// EtcdEndpoints is the server address for Etcd, used by calico
	EtcdEndpoints = "EtcdEndpoints"
	// APIRoot is the kubernetes API address
	APIRoot = "APIRoot"
	// kubernetes client certificates and kubeconfig paths

	EtcdClientCert     = "EtcdClientCert"
	EtcdClientKey      = "EtcdClientKey"
	EtcdClientCA       = "EtcdClientCA"
	EtcdClientCertPath = "EtcdClientCertPath"
	EtcdClientKeyPath  = "EtcdClientKeyPath"
	EtcdClientCAPath   = "EtcdClientCAPath"

	ClientCertPath = "ClientCertPath"
	ClientKeyPath  = "ClientKeyPath"
	ClientCAPath   = "ClientCAPath"

	KubeCfg = "KubeCfg"

	ClusterCIDR = "ClusterCIDR"
	// Images key names

	Image              = "Image"
	CNIImage           = "CNIImage"
	NodeImage          = "NodeImage"
	ControllersImage   = "ControllersImage"
	CanalFlannelImg    = "CanalFlannelImg"
	FlexVolImg         = "FlexVolImg"
	WeaveLoopbackImage = "WeaveLoopbackImage"

	Calicoctl = "Calicoctl"

	FlannelInterface                       = "FlannelInterface"
	FlannelBackend                         = "FlannelBackend"
	KubeFlannelPriorityClassName           = "KubeFlannelPriorityClassName"
	CalicoNodePriorityClassName            = "CalicoNodePriorityClassName"
	CalicoKubeControllersPriorityClassName = "CalicoKubeControllersPriorityClassName"
	CanalInterface                         = "CanalInterface"
	CanalPriorityClassName                 = "CanalPriorityClassName"
	FlexVolPluginDir                       = "FlexVolPluginDir"
	WeavePassword                          = "WeavePassword"
	WeaveNetPriorityClassName              = "WeaveNetPriorityClassName"
	MTU                                    = "MTU"
	RBACConfig                             = "RBACConfig"
	ClusterVersion                         = "ClusterVersion"
	SystemIdentifier                       = "SystemIdentifier"
	ApicHosts                              = "ApicHosts"
	Token                                  = "Token"
	ApicUserName                           = "ApicUserName"
	ApicUserKey                            = "ApicUserKey"
	ApicUserCrt                            = "ApicUserCrt"
	ApicRefreshTime                        = "ApicRefreshTime"
	VmmDomain                              = "VmmDomain"
	VmmController                          = "VmmController"
	EncapType                              = "EncapType"
	McastRangeStart                        = "McastRangeStart"
	McastRangeEnd                          = "McastRangeEnd"
	AEP                                    = "AEP"
	VRFName                                = "VRFName"
	VRFTenant                              = "VRFTenant"
	L3Out                                  = "L3Out"
	L3OutExternalNetworks                  = "L3OutExternalNetworks"
	DynamicExternalSubnet                  = "DynamicExternalSubnet"
	StaticExternalSubnet                   = "StaticExternalSubnet"
	ServiceGraphSubnet                     = "ServiceGraphSubnet"
	KubeAPIVlan                            = "KubeAPIVlan"
	ServiceVlan                            = "ServiceVlan"
	InfraVlan                              = "InfraVlan"
	ImagePullPolicy                        = "ImagePullPolicy"
	ImagePullSecret                        = "ImagePullSecret"
	Tenant                                 = "Tenant"
	ServiceMonitorInterval                 = "ServiceMonitorInterval"
	PBRTrackingNonSnat                     = "PBRTrackingNonSnat"
	InstallIstio                           = "InstallIstio"
	IstioProfile                           = "IstioProfile"
	DropLogEnable                          = "DropLogEnable"
	ControllerLogLevel                     = "ControllerLogLevel"
	HostAgentLogLevel                      = "HostAgentLogLevel"
	OpflexAgentLogLevel                    = "OpflexAgentLogLevel"
	AciCniDeployContainer                  = "AciCniDeployContainer"
	AciHostContainer                       = "AciHostContainer"
	AciOpflexContainer                     = "AciOpflexContainer"
	AciMcastContainer                      = "AciMcastContainer"
	AciOpenvSwitchContainer                = "AciOpenvSwitchContainer"
	AciControllerContainer                 = "AciControllerContainer"
	AciGbpServerContainer                  = "AciGbpServerContainer"
	AciOpflexServerContainer               = "AciOpflexServerContainer"
	StaticServiceIPPool                    = "StaticServiceIPPool"
	PodNetwork                             = "PodNetwork"
	PodSubnet                              = "PodSubnet"
	PodIPPool                              = "PodIPPool"
	NodeServiceIPStart                     = "NodeServiceIPStart"
	NodeServiceIPEnd                       = "NodeServiceIPEnd"
	ServiceIPPool                          = "ServiceIPPool"
	UseAciCniPriorityClass                 = "UseAciCniPriorityClass"
	NoPriorityClass                        = "NoPriorityClass"
	MaxNodesSvcGraph                       = "MaxNodesSvcGraph"
	SnatContractScope                      = "SnatContractScope"
	PodSubnetChunkSize                     = "PodSubnetChunkSize"
	EnableEndpointSlice                    = "EnableEndpointSlice"
	SnatNamespace                          = "SnatNamespace"
	EpRegistry                             = "EpRegistry"
	OpflexMode                             = "OpflexMode"
	SnatPortRangeStart                     = "SnatPortRangeStart"
	SnatPortRangeEnd                       = "SnatPortRangeEnd"
	SnatPortsPerNode                       = "SnatPortsPerNode"
	OpflexClientSSL                        = "OpflexClientSSL"
	UsePrivilegedContainer                 = "UsePrivilegedContainer"
	UseHostNetnsVolume                     = "UseHostNetnsVolume"
	UseOpflexServerVolume                  = "UseOpflexServerVolume"
	KafkaBrokers                           = "KafkaBrokers"
	KafkaClientCrt                         = "KafkaClientCrt"
	KafkaClientKey                         = "KafkaClientKey"
	SubnetDomainName                       = "SubnetDomainName"
	CApic                                  = "CApic"
	UseAciAnywhereCRD                      = "UseAciAnywhereCRD"
	OverlayVRFName                         = "OverlayVRFName"
	GbpPodSubnet                           = "GbpPodSubnet"
	RunGbpContainer                        = "RunGbpContainer"
	RunOpflexServerContainer               = "RunOpflexServerContainer"
	OpflexServerPort                       = "OpflexServerPort"
	DurationWaitForNetwork                 = "DurationWaitForNetwork"
	DisableWaitForNetwork                  = "DisableWaitForNetwork"
	UseClusterRole                         = "UseClusterRole"
	ApicSubscriptionDelay                  = "ApicSubscriptionDelay"
	ApicRefreshTickerAdjust                = "ApicRefreshTickerAdjust"
	DisablePeriodicSnatGlobalInfoSync      = "DisablePeriodicSnatGlobalInfoSync"
	OpflexDeviceDeleteTimeout              = "OpflexDeviceDeleteTimeout"
	MTUHeadRoom                            = "MTUHeadRoom"
	NodePodIfEnable                        = "NodePodIfEnable"
	SriovEnable                            = "SriovEnable"
	MultusDisable                          = "MultusDisable"
	NoWaitForServiceEpReadiness            = "NoWaitForServiceEpReadiness"
	AddExternalSubnetsToRdconfig           = "AddExternalSubnetsToRdconfig"
	ServiceGraphEndpointAddDelay           = "ServiceGraphEndpointAddDelay"
	ServiceGraphEndpointAddServices        = "ServiceGraphEndpointAddServices"
	HppOptimization                        = "HppOptimization"
	SleepTimeSnatGlobalInfoSync            = "SleepTimeSnatGlobalInfoSync"
	OpflexAgentOpflexAsyncjsonEnabled      = "OpflexAgentOpflexAsyncjsonEnabled"
	OpflexAgentOvsAsyncjsonEnabled         = "OpflexAgentOvsAsyncjsonEnabled"
	OpflexAgentPolicyRetryDelayTimer       = "OpflexAgentPolicyRetryDelayTimer"
	AciMultipod                            = "AciMultipod"
	OpflexDeviceReconnectWaitTimeout       = "OpflexDeviceReconnectWaitTimeout"
	AciMultipodUbuntu                      = "AciMultipodUbuntu"
	DhcpRenewMaxRetryCount                 = "DhcpRenewMaxRetryCount"
	DhcpDelay                              = "DhcpDelay"
	OVSMemoryLimit                         = "OVSMemoryLimit"
	OVSMemoryRequest                       = "OVSMemoryRequest"
	NodeSubnet                             = "NodeSubnet"
	NodeSelector                           = "NodeSelector"
	UpdateStrategy                         = "UpdateStrategy"
	Tolerations                            = "Tolerations"
	UseSystemNodePriorityClass             = "UseSystemNodePriorityClass"
	AciContainersControllerMemoryRequest   = "AciContainersControllerMemoryRequest"
	AciContainersControllerMemoryLimit     = "AciContainersControllerMemoryLimit"
	AciContainersHostMemoryRequest         = "AciContainersHostMemoryRequest"
	AciContainersHostMemoryLimit           = "AciContainersHostMemoryLimit"
	McastDaemonMemoryRequest               = "McastDaemonMemoryRequest"
	McastDaemonMemoryLimit                 = "McastDaemonMemoryLimit"
	OpflexAgentMemoryRequest               = "OpflexAgentMemoryRequest"
	OpflexAgentMemoryLimit                 = "OpflexAgentMemoryLimit"
	AciContainersMemoryRequest             = "AciContainersMemoryRequest"
	AciContainersMemoryLimit               = "AciContainersMemoryLimit"
	OpflexAgentStatistics                  = "OpflexAgentStatistics"
	AddExternalContractToDefaultEpg        = "AddExternalContractToDefaultEpg"
	EnableOpflexAgentReconnect             = "EnableOpflexAgentReconnect"
	OpflexOpensslCompat                    = "OpflexOpensslCompat"
	NodeSnatRedirectExclude                = "NodeSnatRedirectExclude"
	TolerationSeconds                      = "TolerationSeconds"
	DisableHppRendering                    = "DisableHppRendering"
	ApicConnectionRetryLimit               = "ApicConnectionRetryLimit"
	TaintNotReadyNode                      = "TaintNotReadyNode"
	DropLogDisableEvents                   = "DropLogDisableEvents"
)

type IPPool struct {
	Start net.IP
	End   net.IP
}

type PodIPNetwork struct {
	Subnet  net.IPNet
	Gateway net.IP
}

var EtcdPortList = []string{
	EtcdPort1,
	EtcdPort2,
}

var ControlPlanePortList = []string{
	KubeAPIPort,
}

var WorkerPortList = []string{
	KubeletPort,
}

var EtcdClientPortList = []string{
	EtcdPort1,
}

var CalicoNetworkLabels = []string{CalicoNodeLabel, CalicoControllerLabel}
var IPv6CompatibleNetworkPlugins = []string{CalicoNetworkPlugin, AciNetworkPlugin}

func (c *Cluster) deployNetworkPlugin(ctx context.Context, data map[string]interface{}) error {
	log.Infof(ctx, "[network] Setting up network plugin: %s", c.Network.Plugin)
	switch c.Network.Plugin {
	case FlannelNetworkPlugin:
		return c.doFlannelDeploy(ctx, data)
	case CalicoNetworkPlugin:
		return c.doCalicoDeploy(ctx, data)
	case CanalNetworkPlugin:
		return c.doCanalDeploy(ctx, data)
	case WeaveNetworkPlugin:
		return c.doWeaveDeploy(ctx, data)
	case AciNetworkPlugin:
		return c.doAciDeploy(ctx, data)
	case NoNetworkPlugin:
		log.Infof(ctx, "[network] Not deploying a cluster network, expecting custom CNI")
		return nil
	default:
		return fmt.Errorf("[network] Unsupported network plugin: %s", c.Network.Plugin)
	}
}

func (c *Cluster) doFlannelDeploy(ctx context.Context, data map[string]interface{}) error {
	vni, err := atoiWithDefault(c.Network.Options[FlannelBackendVxLanNetworkIdentify], FlannelVxLanNetworkIdentify)
	if err != nil {
		return err
	}
	port, err := atoiWithDefault(c.Network.Options[FlannelBackendPort], FlannelVxLanPort)
	if err != nil {
		return err
	}

	flannelConfig := map[string]interface{}{
		ClusterCIDR:      c.ClusterCIDR,
		Image:            c.SystemImages.Flannel,
		CNIImage:         c.SystemImages.FlannelCNI,
		FlannelInterface: c.Network.Options[FlannelIface],
		FlannelBackend: map[string]interface{}{
			"Type": c.Network.Options[FlannelBackendType],
			"VNI":  vni,
			"Port": port,
		},
		RBACConfig:     c.Authorization.Mode,
		ClusterVersion: util.GetTagMajorVersion(c.Version),
		NodeSelector:   c.Network.NodeSelector,
		UpdateStrategy: &appsv1.DaemonSetUpdateStrategy{
			Type:          c.Network.UpdateStrategy.Strategy,
			RollingUpdate: c.Network.UpdateStrategy.RollingUpdate,
		},
		KubeFlannelPriorityClassName: c.Network.Options[KubeFlannelPriorityClassNameKeyName],
	}
	pluginYaml, err := c.getNetworkPluginManifest(flannelConfig, data)
	if err != nil {
		return err
	}
	return c.doAddonDeploy(ctx, pluginYaml, NetworkPluginResourceName, true)
}

func (c *Cluster) doCalicoDeploy(ctx context.Context, data map[string]interface{}) error {
	clientConfig := pki.GetConfigPath(pki.KubeNodeCertName)

	calicoConfig := map[string]interface{}{
		KubeCfg:          clientConfig,
		ClusterCIDR:      c.ClusterCIDR,
		CNIImage:         c.SystemImages.CalicoCNI,
		NodeImage:        c.SystemImages.CalicoNode,
		Calicoctl:        c.SystemImages.CalicoCtl,
		ControllersImage: c.SystemImages.CalicoControllers,
		CloudProvider:    c.Network.Options[CalicoCloudProvider],
		FlexVolImg:       c.SystemImages.CalicoFlexVol,
		RBACConfig:       c.Authorization.Mode,
		NodeSelector:     c.Network.NodeSelector,
		MTU:              c.Network.MTU,
		UpdateStrategy: &appsv1.DaemonSetUpdateStrategy{
			Type:          c.Network.UpdateStrategy.Strategy,
			RollingUpdate: c.Network.UpdateStrategy.RollingUpdate,
		},
		Tolerations:                            c.Network.Tolerations,
		FlexVolPluginDir:                       c.Network.Options[CalicoFlexVolPluginDirectory],
		CalicoNodePriorityClassName:            c.Network.Options[CalicoNodePriorityClassNameKeyName],
		CalicoKubeControllersPriorityClassName: c.Network.Options[CalicoKubeControllersPriorityClassNameKeyName],
	}
	pluginYaml, err := c.getNetworkPluginManifest(calicoConfig, data)
	if err != nil {
		return err
	}
	return c.doAddonDeploy(ctx, pluginYaml, NetworkPluginResourceName, true)
}

func (c *Cluster) doCanalDeploy(ctx context.Context, data map[string]interface{}) error {
	flannelVni, err := atoiWithDefault(c.Network.Options[CanalFlannelBackendVxLanNetworkIdentify], FlannelVxLanNetworkIdentify)
	if err != nil {
		return err
	}
	flannelPort, err := atoiWithDefault(c.Network.Options[CanalFlannelBackendPort], FlannelVxLanPort)
	if err != nil {
		return err
	}

	clientConfig := pki.GetConfigPath(pki.KubeNodeCertName)
	canalConfig := map[string]interface{}{
		ClientCertPath:   pki.GetCertPath(pki.KubeNodeCertName),
		APIRoot:          "https://127.0.0.1:6443",
		ClientKeyPath:    pki.GetKeyPath(pki.KubeNodeCertName),
		ClientCAPath:     pki.GetCertPath(pki.CACertName),
		KubeCfg:          clientConfig,
		ClusterCIDR:      c.ClusterCIDR,
		NodeImage:        c.SystemImages.CanalNode,
		CNIImage:         c.SystemImages.CanalCNI,
		ControllersImage: c.SystemImages.CanalControllers,
		CanalFlannelImg:  c.SystemImages.CanalFlannel,
		RBACConfig:       c.Authorization.Mode,
		CanalInterface:   c.Network.Options[CanalIface],
		FlexVolImg:       c.SystemImages.CanalFlexVol,
		FlannelBackend: map[string]interface{}{
			"Type": c.Network.Options[CanalFlannelBackendType],
			"VNI":  flannelVni,
			"Port": flannelPort,
		},
		NodeSelector: c.Network.NodeSelector,
		MTU:          c.Network.MTU,
		UpdateStrategy: &appsv1.DaemonSetUpdateStrategy{
			Type:          c.Network.UpdateStrategy.Strategy,
			RollingUpdate: c.Network.UpdateStrategy.RollingUpdate,
		},
		Tolerations:                            c.Network.Tolerations,
		FlexVolPluginDir:                       c.Network.Options[CanalFlexVolPluginDirectory],
		CanalPriorityClassName:                 c.Network.Options[CanalPriorityClassNameKeyName],
		CalicoKubeControllersPriorityClassName: c.Network.Options[CalicoKubeControllersPriorityClassNameKeyName],
	}
	pluginYaml, err := c.getNetworkPluginManifest(canalConfig, data)
	if err != nil {
		return err
	}
	return c.doAddonDeploy(ctx, pluginYaml, NetworkPluginResourceName, true)
}

func (c *Cluster) doWeaveDeploy(ctx context.Context, data map[string]interface{}) error {
	weaveConfig := map[string]interface{}{
		ClusterCIDR:        c.ClusterCIDR,
		WeavePassword:      c.Network.Options[WeavePassword],
		Image:              c.SystemImages.WeaveNode,
		CNIImage:           c.SystemImages.WeaveCNI,
		WeaveLoopbackImage: c.SystemImages.Alpine,
		RBACConfig:         c.Authorization.Mode,
		NodeSelector:       c.Network.NodeSelector,
		MTU:                c.Network.MTU,
		UpdateStrategy: &appsv1.DaemonSetUpdateStrategy{
			Type:          c.Network.UpdateStrategy.Strategy,
			RollingUpdate: c.Network.UpdateStrategy.RollingUpdate,
		},
		WeaveNetPriorityClassName: c.Network.Options[WeaveNetPriorityClassNameKeyName],
	}
	pluginYaml, err := c.getNetworkPluginManifest(weaveConfig, data)
	if err != nil {
		return err
	}
	return c.doAddonDeploy(ctx, pluginYaml, NetworkPluginResourceName, true)
}

func (c *Cluster) doAciDeploy(ctx context.Context, data map[string]interface{}) error {
	var podIPPool []IPPool
	var podNetwork []PodIPNetwork
	var podSubnet []string

	ClusterCIDRs := strings.Split(c.ClusterCIDR, ",")
	for _, clusterCIDR := range ClusterCIDRs {
		podSubnet = append(podSubnet, fmt.Sprintf("\"%s\"", clusterCIDR))
		_, clusterCIDR, err := net.ParseCIDR(clusterCIDR)
		if err != nil {
			return err
		}
		podIPStart, podIPEnd := cidr.AddressRange(clusterCIDR)
		podIPPool = append(podIPPool, IPPool{Start: cidr.Inc(cidr.Inc(podIPStart)), End: cidr.Dec(podIPEnd)})
		podNetwork = append(podNetwork, PodIPNetwork{Subnet: *clusterCIDR, Gateway: cidr.Inc(podIPStart)})
	}

	var staticServiceIPPool []IPPool
	var staticExtern []string
	staticExternalSubnets := strings.Split(c.Network.Options[AciStaticExternalSubnet], ",")
	for _, staticExternalSubnet := range staticExternalSubnets {
		staticExtern = append(staticExtern, fmt.Sprintf("\"%s\"", staticExternalSubnet))
		_, externStatic, err := net.ParseCIDR(staticExternalSubnet)
		if err != nil {
			return err
		}
		staticServiceIPStart, staticServiceIPEnd := cidr.AddressRange(externStatic)
		staticServiceIPPool = append(staticServiceIPPool, IPPool{Start: cidr.Inc(cidr.Inc(staticServiceIPStart)), End: cidr.Dec(staticServiceIPEnd)})
	}

	_, svcGraphSubnet, err := net.ParseCIDR(c.Network.Options[AciServiceGraphSubnet])
	if err != nil {
		return err
	}
	nodeServiceIPStart, nodeServiceIPEnd := cidr.AddressRange(svcGraphSubnet)

	var serviceIPPool []IPPool
	var dynamicExtern []string
	dynamicExternalSubnets := strings.Split(c.Network.Options[AciDynamicExternalSubnet], ",")
	for _, dynamicExternalSubnet := range dynamicExternalSubnets {
		dynamicExtern = append(dynamicExtern, fmt.Sprintf("\"%s\"", dynamicExternalSubnet))
		_, externDynamic, err := net.ParseCIDR(dynamicExternalSubnet)
		if err != nil {
			return err
		}
		serviceIPStart, serviceIPEnd := cidr.AddressRange(externDynamic)
		serviceIPPool = append(serviceIPPool, IPPool{Start: cidr.Inc(cidr.Inc(serviceIPStart)), End: cidr.Dec(serviceIPEnd)})
	}

	var nodeSubnets []string
	NodeSubnets := strings.Split(c.Network.Options[AciNodeSubnet], ",")
	for _, nodeSubnet := range NodeSubnets {
		nodeSubnets = append(nodeSubnets, fmt.Sprintf("\"%s\"", nodeSubnet))
	}

	if c.Network.Options[AciTenant] == "" {
		c.Network.Options[AciTenant] = c.Network.Options[AciSystemIdentifier]
	}

	AciConfig := map[string]interface{}{
		SystemIdentifier:                     c.Network.Options[AciSystemIdentifier],
		ApicHosts:                            c.Network.AciNetworkProvider.ApicHosts,
		Token:                                c.Network.Options[AciToken],
		ApicUserName:                         c.Network.Options[AciApicUserName],
		ApicUserKey:                          c.Network.Options[AciApicUserKey],
		ApicUserCrt:                          c.Network.Options[AciApicUserCrt],
		ApicRefreshTime:                      c.Network.Options[AciApicRefreshTime],
		VmmDomain:                            c.Network.Options[AciVmmDomain],
		VmmController:                        c.Network.Options[AciVmmController],
		EncapType:                            c.Network.Options[AciEncapType],
		McastRangeStart:                      c.Network.Options[AciMcastRangeStart],
		McastRangeEnd:                        c.Network.Options[AciMcastRangeEnd],
		NodeSubnet:                           nodeSubnets,
		AEP:                                  c.Network.Options[AciAEP],
		VRFName:                              c.Network.Options[AciVRFName],
		VRFTenant:                            c.Network.Options[AciVRFTenant],
		L3Out:                                c.Network.Options[AciL3Out],
		L3OutExternalNetworks:                c.Network.AciNetworkProvider.L3OutExternalNetworks,
		DynamicExternalSubnet:                dynamicExtern,
		StaticExternalSubnet:                 staticExtern,
		ServiceGraphSubnet:                   c.Network.Options[AciServiceGraphSubnet],
		KubeAPIVlan:                          c.Network.Options[AciKubeAPIVlan],
		ServiceVlan:                          c.Network.Options[AciServiceVlan],
		InfraVlan:                            c.Network.Options[AciInfraVlan],
		ImagePullPolicy:                      c.Network.Options[AciImagePullPolicy],
		ImagePullSecret:                      c.Network.Options[AciImagePullSecret],
		Tenant:                               c.Network.Options[AciTenant],
		ServiceMonitorInterval:               c.Network.Options[AciServiceMonitorInterval],
		PBRTrackingNonSnat:                   c.Network.Options[AciPBRTrackingNonSnat],
		InstallIstio:                         c.Network.Options[AciInstallIstio],
		IstioProfile:                         c.Network.Options[AciIstioProfile],
		DropLogEnable:                        c.Network.Options[AciDropLogEnable],
		ControllerLogLevel:                   c.Network.Options[AciControllerLogLevel],
		HostAgentLogLevel:                    c.Network.Options[AciHostAgentLogLevel],
		OpflexAgentLogLevel:                  c.Network.Options[AciOpflexAgentLogLevel],
		OVSMemoryLimit:                       c.Network.Options[AciOVSMemoryLimit],
		OVSMemoryRequest:                     c.Network.Options[AciOVSMemoryRequest],
		ClusterCIDR:                          c.ClusterCIDR,
		PodNetwork:                           podNetwork,
		PodIPPool:                            podIPPool,
		StaticServiceIPPool:                  staticServiceIPPool,
		ServiceIPPool:                        serviceIPPool,
		PodSubnet:                            podSubnet,
		NodeServiceIPStart:                   cidr.Inc(cidr.Inc(nodeServiceIPStart)),
		NodeServiceIPEnd:                     cidr.Dec(nodeServiceIPEnd),
		UseAciCniPriorityClass:               c.Network.Options[AciUseAciCniPriorityClass],
		NoPriorityClass:                      c.Network.Options[AciNoPriorityClass],
		MaxNodesSvcGraph:                     c.Network.Options[AciMaxNodesSvcGraph],
		SnatContractScope:                    c.Network.Options[AciSnatContractScope],
		PodSubnetChunkSize:                   c.Network.Options[AciPodSubnetChunkSize],
		EnableEndpointSlice:                  c.Network.Options[AciEnableEndpointSlice],
		SnatNamespace:                        c.Network.Options[AciSnatNamespace],
		EpRegistry:                           c.Network.Options[AciEpRegistry],
		OpflexMode:                           c.Network.Options[AciOpflexMode],
		SnatPortRangeStart:                   c.Network.Options[AciSnatPortRangeStart],
		SnatPortRangeEnd:                     c.Network.Options[AciSnatPortRangeEnd],
		SnatPortsPerNode:                     c.Network.Options[AciSnatPortsPerNode],
		OpflexClientSSL:                      c.Network.Options[AciOpflexClientSSL],
		UsePrivilegedContainer:               c.Network.Options[AciUsePrivilegedContainer],
		UseHostNetnsVolume:                   c.Network.Options[AciUseHostNetnsVolume],
		UseOpflexServerVolume:                c.Network.Options[AciUseOpflexServerVolume],
		KafkaBrokers:                         c.Network.AciNetworkProvider.KafkaBrokers,
		KafkaClientCrt:                       c.Network.Options[AciKafkaClientCrt],
		KafkaClientKey:                       c.Network.Options[AciKafkaClientKey],
		SubnetDomainName:                     c.Network.Options[AciSubnetDomainName],
		CApic:                                c.Network.Options[AciCApic],
		UseAciAnywhereCRD:                    c.Network.Options[AciUseAciAnywhereCRD],
		OverlayVRFName:                       c.Network.Options[AciOverlayVRFName],
		GbpPodSubnet:                         c.Network.Options[AciGbpPodSubnet],
		RunGbpContainer:                      c.Network.Options[AciRunGbpContainer],
		RunOpflexServerContainer:             c.Network.Options[AciRunOpflexServerContainer],
		OpflexServerPort:                     c.Network.Options[AciOpflexServerPort],
		DurationWaitForNetwork:               c.Network.Options[AciDurationWaitForNetwork],
		DisableWaitForNetwork:                c.Network.Options[AciDisableWaitForNetwork],
		UseClusterRole:                       c.Network.Options[AciUseClusterRole],
		ApicSubscriptionDelay:                c.Network.Options[AciApicSubscriptionDelay],
		ApicRefreshTickerAdjust:              c.Network.Options[AciApicRefreshTickerAdjust],
		DisablePeriodicSnatGlobalInfoSync:    c.Network.Options[AciDisablePeriodicSnatGlobalInfoSync],
		OpflexDeviceDeleteTimeout:            c.Network.Options[AciOpflexDeviceDeleteTimeout],
		MTUHeadRoom:                          c.Network.Options[AciMTUHeadRoom],
		NodePodIfEnable:                      c.Network.Options[AciNodePodIfEnable],
		SriovEnable:                          c.Network.Options[AciSriovEnable],
		MultusDisable:                        c.Network.Options[AciMultusDisable],
		NoWaitForServiceEpReadiness:          c.Network.Options[AciNoWaitForServiceEpReadiness],
		AddExternalSubnetsToRdconfig:         c.Network.Options[AciAddExternalSubnetsToRdconfig],
		ServiceGraphEndpointAddDelay:         c.Network.Options[AciServiceGraphEndpointAddDelay],
		ServiceGraphEndpointAddServices:      c.Network.AciNetworkProvider.ServiceGraphEndpointAddServices,
		HppOptimization:                      c.Network.Options[AciHppOptimization],
		SleepTimeSnatGlobalInfoSync:          c.Network.Options[AciSleepTimeSnatGlobalInfoSync],
		OpflexAgentOpflexAsyncjsonEnabled:    c.Network.Options[AciOpflexAgentOpflexAsyncjsonEnabled],
		OpflexAgentOvsAsyncjsonEnabled:       c.Network.Options[AciOpflexAgentOvsAsyncjsonEnabled],
		OpflexAgentPolicyRetryDelayTimer:     c.Network.Options[AciOpflexAgentPolicyRetryDelayTimer],
		AciMultipod:                          c.Network.Options[AciAciMultipod],
		OpflexDeviceReconnectWaitTimeout:     c.Network.Options[AciOpflexDeviceReconnectWaitTimeout],
		AciMultipodUbuntu:                    c.Network.Options[AciAciMultipodUbuntu],
		DhcpRenewMaxRetryCount:               c.Network.Options[AciDhcpRenewMaxRetryCount],
		DhcpDelay:                            c.Network.Options[AciDhcpDelay],
		UseSystemNodePriorityClass:           c.Network.Options[AciUseSystemNodePriorityClass],
		AciContainersControllerMemoryRequest: c.Network.Options[AciAciContainersControllerMemoryRequest],
		AciContainersControllerMemoryLimit:   c.Network.Options[AciAciContainersControllerMemoryLimit],
		AciContainersHostMemoryRequest:       c.Network.Options[AciAciContainersHostMemoryRequest],
		AciContainersHostMemoryLimit:         c.Network.Options[AciAciContainersHostMemoryLimit],
		McastDaemonMemoryRequest:             c.Network.Options[AciMcastDaemonMemoryRequest],
		McastDaemonMemoryLimit:               c.Network.Options[AciMcastDaemonMemoryLimit],
		OpflexAgentMemoryRequest:             c.Network.Options[AciOpflexAgentMemoryRequest],
		OpflexAgentMemoryLimit:               c.Network.Options[AciOpflexAgentMemoryLimit],
		AciContainersMemoryRequest:           c.Network.Options[AciAciContainersMemoryRequest],
		AciContainersMemoryLimit:             c.Network.Options[AciAciContainersMemoryLimit],
		OpflexAgentStatistics:                c.Network.Options[AciOpflexAgentStatistics],
		AddExternalContractToDefaultEpg:      c.Network.Options[AciAddExternalContractToDefaultEpg],
		EnableOpflexAgentReconnect:           c.Network.Options[AciEnableOpflexAgentReconnect],
		OpflexOpensslCompat:                  c.Network.Options[AciOpflexOpensslCompat],
		TolerationSeconds:                    c.Network.Options[AciTolerationSeconds],
		DisableHppRendering:                  c.Network.Options[AciDisableHppRendering],
		ApicConnectionRetryLimit:             c.Network.Options[AciApicConnectionRetryLimit],
		TaintNotReadyNode:                    c.Network.Options[AciTaintNotReadyNode],
		DropLogDisableEvents:                 c.Network.Options[AciDropLogDisableEvents],
		NodeSnatRedirectExclude:              c.Network.AciNetworkProvider.NodeSnatRedirectExclude,
		AciCniDeployContainer:                c.SystemImages.AciCniDeployContainer,
		AciHostContainer:                     c.SystemImages.AciHostContainer,
		AciOpflexContainer:                   c.SystemImages.AciOpflexContainer,
		AciMcastContainer:                    c.SystemImages.AciMcastContainer,
		AciOpenvSwitchContainer:              c.SystemImages.AciOpenvSwitchContainer,
		AciControllerContainer:               c.SystemImages.AciControllerContainer,
		AciGbpServerContainer:                c.SystemImages.AciGbpServerContainer,
		AciOpflexServerContainer:             c.SystemImages.AciOpflexServerContainer,
		MTU:                                  c.Network.MTU,
	}

	pluginYaml, err := c.getNetworkPluginManifest(AciConfig, data)
	if err != nil {
		return err
	}
	return c.doAddonDeploy(ctx, pluginYaml, NetworkPluginResourceName, true)
}

func (c *Cluster) getNetworkPluginManifest(pluginConfig, data map[string]interface{}) (string, error) {
	switch c.Network.Plugin {
	case CanalNetworkPlugin, FlannelNetworkPlugin, CalicoNetworkPlugin, WeaveNetworkPlugin, AciNetworkPlugin:
		tmplt, err := templates.GetVersionedTemplates(c.Network.Plugin, data, c.Version)
		if err != nil {
			return "", err
		}
		return templates.CompileTemplateFromMap(tmplt, pluginConfig)
	default:
		return "", fmt.Errorf("[network] Unsupported network plugin: %s", c.Network.Plugin)
	}
}

func (c *Cluster) CheckClusterPorts(ctx context.Context, currentCluster *Cluster) error {
	if currentCluster != nil {
		newEtcdHost := hosts.GetToAddHosts(currentCluster.EtcdHosts, c.EtcdHosts)
		newControlPlaneHosts := hosts.GetToAddHosts(currentCluster.ControlPlaneHosts, c.ControlPlaneHosts)
		newWorkerHosts := hosts.GetToAddHosts(currentCluster.WorkerHosts, c.WorkerHosts)

		if len(newEtcdHost) == 0 &&
			len(newWorkerHosts) == 0 &&
			len(newControlPlaneHosts) == 0 {
			log.Infof(ctx, "[network] No hosts added existing cluster, skipping port check")
			return nil
		}
	}
	if err := c.deployTCPPortListeners(ctx, currentCluster); err != nil {
		return err
	}
	if err := c.runServicePortChecks(ctx); err != nil {
		return err
	}
	// Skip kubeapi check if we are using custom k8s dialer or bastion/jump host
	if c.K8sWrapTransport == nil && len(c.BastionHost.Address) == 0 {
		if err := c.checkKubeAPIPort(ctx); err != nil {
			return err
		}
	} else {
		log.Infof(ctx, "[network] Skipping kubeapi port check")
	}

	return c.removeTCPPortListeners(ctx)
}

func (c *Cluster) checkKubeAPIPort(ctx context.Context) error {
	log.Infof(ctx, "[network] Checking KubeAPI port Control Plane hosts")
	for _, host := range c.ControlPlaneHosts {
		logrus.Debugf("[network] Checking KubeAPI port [%s] on host: %s", KubeAPIPort, host.Address)
		address := fmt.Sprintf("%s:%s", host.Address, KubeAPIPort)
		conn, err := net.Dial("tcp", address)
		if err != nil {
			return fmt.Errorf("[network] Can't access KubeAPI port [%s] on Control Plane host: %s", KubeAPIPort, host.Address)
		}
		conn.Close()
	}
	return nil
}

func (c *Cluster) deployTCPPortListeners(ctx context.Context, currentCluster *Cluster) error {
	log.Infof(ctx, "[network] Deploying port listener containers")

	// deploy ectd listeners
	if err := c.deployListenerOnPlane(ctx, EtcdPortList, c.EtcdHosts, EtcdPortListenContainer); err != nil {
		return err
	}

	// deploy controlplane listeners
	if err := c.deployListenerOnPlane(ctx, ControlPlanePortList, c.ControlPlaneHosts, CPPortListenContainer); err != nil {
		return err
	}

	// deploy worker listeners
	if err := c.deployListenerOnPlane(ctx, WorkerPortList, c.WorkerHosts, WorkerPortListenContainer); err != nil {
		return err
	}
	log.Infof(ctx, "[network] Port listener containers deployed successfully")
	return nil
}

func (c *Cluster) deployListenerOnPlane(ctx context.Context, portList []string, hostPlane []*hosts.Host, containerName string) error {
	var errgrp errgroup.Group
	hostsQueue := util.GetObjectQueue(hostPlane)
	for w := 0; w < WorkerThreads; w++ {
		errgrp.Go(func() error {
			var errList []error
			for host := range hostsQueue {
				err := c.deployListener(ctx, host.(*hosts.Host), portList, containerName)
				if err != nil {
					errList = append(errList, err)
				}
			}
			return util.ErrList(errList)
		})
	}
	return errgrp.Wait()
}

func (c *Cluster) deployListener(ctx context.Context, host *hosts.Host, portList []string, containerName string) error {
	imageCfg := &container.Config{
		Image: c.SystemImages.Alpine,
		Cmd: []string{
			"nc",
			"-kl",
			"-p",
			"1337",
			"-e",
			"echo",
		},
		ExposedPorts: nat.PortSet{
			"1337/tcp": {},
		},
	}
	hostCfg := &container.HostConfig{
		PortBindings: nat.PortMap{
			"1337/tcp": getPortBindings("0.0.0.0", portList),
		},
	}

	logrus.Debugf("[network] Starting deployListener [%s] on host [%s]", containerName, host.Address)
	if err := docker.DoRunContainer(ctx, host.DClient, imageCfg, hostCfg, containerName, host.Address, "network", c.PrivateRegistriesMap); err != nil {
		if strings.Contains(err.Error(), "bind: address already in use") {
			logrus.Debugf("[network] Service is already up on host [%s]", host.Address)
			return nil
		}
		return err
	}
	return nil
}

func (c *Cluster) removeTCPPortListeners(ctx context.Context) error {
	log.Infof(ctx, "[network] Removing port listener containers")

	if err := removeListenerFromPlane(ctx, c.EtcdHosts, EtcdPortListenContainer); err != nil {
		return err
	}
	if err := removeListenerFromPlane(ctx, c.ControlPlaneHosts, CPPortListenContainer); err != nil {
		return err
	}
	if err := removeListenerFromPlane(ctx, c.WorkerHosts, WorkerPortListenContainer); err != nil {
		return err
	}
	log.Infof(ctx, "[network] Port listener containers removed successfully")
	return nil
}

func removeListenerFromPlane(ctx context.Context, hostPlane []*hosts.Host, containerName string) error {
	var errgrp errgroup.Group

	hostsQueue := util.GetObjectQueue(hostPlane)
	for w := 0; w < WorkerThreads; w++ {
		errgrp.Go(func() error {
			var errList []error
			for host := range hostsQueue {
				runHost := host.(*hosts.Host)
				err := docker.DoRemoveContainer(ctx, runHost.DClient, containerName, runHost.Address)
				if err != nil {
					errList = append(errList, err)
				}
			}
			return util.ErrList(errList)
		})
	}
	return errgrp.Wait()
}

func (c *Cluster) runServicePortChecks(ctx context.Context) error {
	var errgrp errgroup.Group
	// check etcd <-> etcd
	// one etcd host is a pass
	if len(c.EtcdHosts) > 1 {
		log.Infof(ctx, "[network] Running etcd <-> etcd port checks")
		hostsQueue := util.GetObjectQueue(c.EtcdHosts)
		for w := 0; w < WorkerThreads; w++ {
			errgrp.Go(func() error {
				var errList []error
				for host := range hostsQueue {
					err := checkPlaneTCPPortsFromHost(ctx, host.(*hosts.Host), EtcdPortList, c.EtcdHosts, c.SystemImages.Alpine, c.PrivateRegistriesMap)
					if err != nil {
						errList = append(errList, err)
					}
				}
				return util.ErrList(errList)
			})
		}
		if err := errgrp.Wait(); err != nil {
			return err
		}
	}
	// check control -> etcd connectivity
	log.Infof(ctx, "[network] Running control plane -> etcd port checks")
	hostsQueue := util.GetObjectQueue(c.ControlPlaneHosts)
	for w := 0; w < WorkerThreads; w++ {
		errgrp.Go(func() error {
			var errList []error
			for host := range hostsQueue {
				err := checkPlaneTCPPortsFromHost(ctx, host.(*hosts.Host), EtcdClientPortList, c.EtcdHosts, c.SystemImages.Alpine, c.PrivateRegistriesMap)
				if err != nil {
					errList = append(errList, err)
				}
			}
			return util.ErrList(errList)
		})
	}
	if err := errgrp.Wait(); err != nil {
		return err
	}
	// check controle plane -> Workers
	log.Infof(ctx, "[network] Running control plane -> worker port checks")
	hostsQueue = util.GetObjectQueue(c.ControlPlaneHosts)
	for w := 0; w < WorkerThreads; w++ {
		errgrp.Go(func() error {
			var errList []error
			for host := range hostsQueue {
				err := checkPlaneTCPPortsFromHost(ctx, host.(*hosts.Host), WorkerPortList, c.WorkerHosts, c.SystemImages.Alpine, c.PrivateRegistriesMap)
				if err != nil {
					errList = append(errList, err)
				}
			}
			return util.ErrList(errList)
		})
	}
	if err := errgrp.Wait(); err != nil {
		return err
	}
	// check workers -> control plane
	log.Infof(ctx, "[network] Running workers -> control plane port checks")
	hostsQueue = util.GetObjectQueue(c.WorkerHosts)
	for w := 0; w < WorkerThreads; w++ {
		errgrp.Go(func() error {
			var errList []error
			for host := range hostsQueue {
				err := checkPlaneTCPPortsFromHost(ctx, host.(*hosts.Host), ControlPlanePortList, c.ControlPlaneHosts, c.SystemImages.Alpine, c.PrivateRegistriesMap)
				if err != nil {
					errList = append(errList, err)
				}
			}
			return util.ErrList(errList)
		})
	}
	return errgrp.Wait()
}

func checkPlaneTCPPortsFromHost(ctx context.Context, host *hosts.Host, portList []string, planeHosts []*hosts.Host, image string, prsMap map[string]v3.PrivateRegistry) error {
	var hosts []string
	var portCheckLogs string
	for _, host := range planeHosts {
		hosts = append(hosts, host.InternalAddress)
	}
	imageCfg := &container.Config{
		Image: image,
		Env: []string{
			fmt.Sprintf("HOSTS=%s", strings.Join(hosts, " ")),
			fmt.Sprintf("PORTS=%s", strings.Join(portList, " ")),
		},
		Cmd: []string{
			"sh",
			"-c",
			"for host in $HOSTS; do for port in $PORTS ; do echo \"Checking host ${host} on port ${port}\" >&1 & nc -w 5 -z $host $port > /dev/null || echo \"${host}:${port}\" >&2 & done; wait; done",
		},
	}
	hostCfg := &container.HostConfig{
		NetworkMode: "host",
		LogConfig: container.LogConfig{
			Type: "json-file",
		},
	}
	for retries := 0; retries < 3; retries++ {
		logrus.Infof("[network] Checking if host [%s] can connect to host(s) [%s] on port(s) [%s], try #%d", host.Address, strings.Join(hosts, " "), strings.Join(portList, " "), retries+1)
		if err := docker.DoRemoveContainer(ctx, host.DClient, PortCheckContainer, host.Address); err != nil {
			return err
		}
		if err := docker.DoRunContainer(ctx, host.DClient, imageCfg, hostCfg, PortCheckContainer, host.Address, "network", prsMap); err != nil {
			return err
		}

		containerLog, _, logsErr := docker.GetContainerLogsStdoutStderr(ctx, host.DClient, PortCheckContainer, "all", true)
		if logsErr != nil {
			log.Warnf(ctx, "[network] Failed to get network port check logs: %v", logsErr)
		}
		logrus.Debugf("[network] containerLog [%s] on host: %s", containerLog, host.Address)

		if err := docker.RemoveContainer(ctx, host.DClient, host.Address, PortCheckContainer); err != nil {
			return err
		}
		logrus.Debugf("[network] Length of containerLog is [%d] on host: %s", len(containerLog), host.Address)
		if len(containerLog) == 0 {
			return nil
		}
		portCheckLogs = strings.Join(strings.Split(strings.TrimSpace(containerLog), "\n"), ", ")
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("[network] Host [%s] is not able to connect to the following ports: [%s]. Please check network policies and firewall rules", host.Address, portCheckLogs)
}

func getPortBindings(hostAddress string, portList []string) []nat.PortBinding {
	portBindingList := []nat.PortBinding{}
	for _, portNumber := range portList {
		rawPort := fmt.Sprintf("%s:%s:1337/tcp", hostAddress, portNumber)
		portMapping, _ := nat.ParsePortSpec(rawPort)
		portBindingList = append(portBindingList, portMapping[0].Binding)
	}
	return portBindingList
}

func atoiWithDefault(val string, defaultVal int) (int, error) {
	if val == "" {
		return defaultVal, nil
	}

	ret, err := strconv.Atoi(val)
	if err != nil {
		return 0, err
	}

	return ret, nil
}
