package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	cnitypes "github.com/containernetworking/cni/pkg/types"
)

// Datastore types
const (
	DefaultLeaderLeaseDuration    = 1500
	DefaultLeaderRenewDeadline    = 1000
	DefaultLeaderRetryPeriod      = 500
	AddTimeLimit                  = 2 * time.Minute
	DelTimeLimit                  = 1 * time.Minute
	DefaultOverlappingIPsFeatures = true
	DefaultSleepForRace           = 0
)

// Net is The top-level network config - IPAM plugins are passed the full configuration
// of the calling plugin, not just the IPAM section.
type Net struct {
	Name       string      `json:"name"`
	CNIVersion string      `json:"cniVersion"`
	IPAM       *IPAMConfig `json:"ipam"`
}

// NetConfList describes an ordered list of networks.
type NetConfList struct {
	CNIVersion string `json:"cniVersion,omitempty"`

	Name         string `json:"name,omitempty"`
	DisableCheck bool   `json:"disableCheck,omitempty"`
	Plugins      []*Net `json:"plugins,omitempty"`
}

type RangeConfiguration struct {
	OmitRanges []string `json:"exclude,omitempty"`
	Range      string   `json:"range"`
	RangeStart net.IP   `json:"range_start,omitempty"`
	RangeEnd   net.IP   `json:"range_end,omitempty"`
}

// IPAMConfig describes the expected json configuration for this plugin
type IPAMConfig struct {
	Name                     string
	Type                     string               `json:"type"`
	Routes                   []*cnitypes.Route    `json:"routes"`
	Addresses                []Address            `json:"addresses,omitempty"`
	IPRanges                 []RangeConfiguration `json:"ipRanges"`
	OmitRanges               []string             `json:"exclude,omitempty"`
	DNS                      cnitypes.DNS         `json:"dns"`
	Range                    string               `json:"range"`
	NodeSliceSize            string               `json:"node_slice_size"`
	RangeStart               net.IP               `json:"range_start,omitempty"`
	RangeEnd                 net.IP               `json:"range_end,omitempty"`
	GatewayStr               string               `json:"gateway"`
	LeaderLeaseDuration      int                  `json:"leader_lease_duration,omitempty"`
	LeaderRenewDeadline      int                  `json:"leader_renew_deadline,omitempty"`
	LeaderRetryPeriod        int                  `json:"leader_retry_period,omitempty"`
	LogFile                  string               `json:"log_file"`
	LogLevel                 string               `json:"log_level"`
	ReconcilerCronExpression string               `json:"reconciler_cron_expression,omitempty"`
	OverlappingRanges        bool                 `json:"enable_overlapping_ranges,omitempty"`
	SleepForRace             int                  `json:"sleep_for_race,omitempty"`
	Gateway                  net.IP
	Kubernetes               KubernetesConfig `json:"kubernetes,omitempty"`
	ConfigurationPath        string           `json:"configuration_path"`
	PodName                  string
	PodNamespace             string
	NetworkName              string `json:"network_name,omitempty"`
}

func (ic *IPAMConfig) UnmarshalJSON(data []byte) error {
	type IPAMConfigAlias struct {
		Name                     string
		Type                     string               `json:"type"`
		Routes                   []*cnitypes.Route    `json:"routes"`
		Datastore                string               `json:"datastore"`
		Addresses                []Address            `json:"addresses,omitempty"`
		IPRanges                 []RangeConfiguration `json:"ipRanges"`
		NodeSliceSize            string               `json:"node_slice_size"`
		OmitRanges               []string             `json:"exclude,omitempty"`
		DNS                      cnitypes.DNS         `json:"dns"`
		Range                    string               `json:"range"`
		RangeStart               string               `json:"range_start,omitempty"`
		RangeEnd                 string               `json:"range_end,omitempty"`
		GatewayStr               string               `json:"gateway"`
		EtcdHost                 string               `json:"etcd_host,omitempty"`
		EtcdUsername             string               `json:"etcd_username,omitempty"`
		EtcdPassword             string               `json:"etcd_password,omitempty"`
		EtcdKeyFile              string               `json:"etcd_key_file,omitempty"`
		EtcdCertFile             string               `json:"etcd_cert_file,omitempty"`
		EtcdCACertFile           string               `json:"etcd_ca_cert_file,omitempty"`
		LeaderLeaseDuration      int                  `json:"leader_lease_duration,omitempty"`
		LeaderRenewDeadline      int                  `json:"leader_renew_deadline,omitempty"`
		LeaderRetryPeriod        int                  `json:"leader_retry_period,omitempty"`
		LogFile                  string               `json:"log_file"`
		LogLevel                 string               `json:"log_level"`
		ReconcilerCronExpression string               `json:"reconciler_cron_expression,omitempty"`
		OverlappingRanges        bool                 `json:"enable_overlapping_ranges,omitempty"`
		SleepForRace             int                  `json:"sleep_for_race,omitempty"`
		Gateway                  string
		Kubernetes               KubernetesConfig `json:"kubernetes,omitempty"`
		ConfigurationPath        string           `json:"configuration_path"`
		PodName                  string
		PodNamespace             string
		NetworkName              string `json:"network_name,omitempty"`
	}

	ipamConfigAlias := IPAMConfigAlias{
		OverlappingRanges: DefaultOverlappingIPsFeatures,
		SleepForRace:      DefaultSleepForRace,
	}
	if err := json.Unmarshal(data, &ipamConfigAlias); err != nil {
		return err
	}

	*ic = IPAMConfig{
		Name:                     ipamConfigAlias.Name,
		Type:                     ipamConfigAlias.Type,
		Routes:                   ipamConfigAlias.Routes,
		Addresses:                ipamConfigAlias.Addresses,
		IPRanges:                 ipamConfigAlias.IPRanges,
		OmitRanges:               ipamConfigAlias.OmitRanges,
		DNS:                      ipamConfigAlias.DNS,
		Range:                    ipamConfigAlias.Range,
		RangeStart:               backwardsCompatibleIPAddress(ipamConfigAlias.RangeStart),
		RangeEnd:                 backwardsCompatibleIPAddress(ipamConfigAlias.RangeEnd),
		NodeSliceSize:            ipamConfigAlias.NodeSliceSize,
		GatewayStr:               ipamConfigAlias.GatewayStr,
		LeaderLeaseDuration:      ipamConfigAlias.LeaderLeaseDuration,
		LeaderRenewDeadline:      ipamConfigAlias.LeaderRenewDeadline,
		LeaderRetryPeriod:        ipamConfigAlias.LeaderRetryPeriod,
		LogFile:                  ipamConfigAlias.LogFile,
		LogLevel:                 ipamConfigAlias.LogLevel,
		OverlappingRanges:        ipamConfigAlias.OverlappingRanges,
		ReconcilerCronExpression: ipamConfigAlias.ReconcilerCronExpression,
		SleepForRace:             ipamConfigAlias.SleepForRace,
		Gateway:                  backwardsCompatibleIPAddress(ipamConfigAlias.Gateway),
		Kubernetes:               ipamConfigAlias.Kubernetes,
		ConfigurationPath:        ipamConfigAlias.ConfigurationPath,
		PodName:                  ipamConfigAlias.PodName,
		PodNamespace:             ipamConfigAlias.PodNamespace,
		NetworkName:              ipamConfigAlias.NetworkName,
	}
	return nil
}

func (ic *IPAMConfig) GetPodRef() string {
	return fmt.Sprintf("%s/%s", ic.PodNamespace, ic.PodName)
}

func backwardsCompatibleIPAddress(ip string) net.IP {
	var ipAddr net.IP
	if sanitizedIP, err := sanitizeIP(ip); err == nil {
		ipAddr = sanitizedIP
	}
	return ipAddr
}

// IPAMEnvArgs are the environment vars we expect
type IPAMEnvArgs struct {
	cnitypes.CommonArgs
	IP                         cnitypes.UnmarshallableString `json:"ip,omitempty"`
	GATEWAY                    cnitypes.UnmarshallableString `json:"gateway,omitempty"`
	K8S_POD_NAME               cnitypes.UnmarshallableString //revive:disable-line
	K8S_POD_NAMESPACE          cnitypes.UnmarshallableString //revive:disable-line
	K8S_POD_INFRA_CONTAINER_ID cnitypes.UnmarshallableString //revive:disable-line
}

// KubernetesConfig describes the kubernetes-specific configuration details
type KubernetesConfig struct {
	KubeConfigPath string `json:"kubeconfig,omitempty"`
	K8sAPIRoot     string `json:"k8s_api_root,omitempty"`
}

// Address is our standard address.
type Address struct {
	AddressStr string `json:"address"`
	Gateway    net.IP `json:"gateway,omitempty"`
	Address    net.IPNet
	Version    string
}

// IPReservation is an address that has been reserved by this plugin
type IPReservation struct {
	IP          net.IP `json:"ip"`
	ContainerID string `json:"id"`
	PodRef      string `json:"podref"`
	IfName      string `json:"ifName"`
	IsAllocated bool
}

func (ir IPReservation) String() string {
	return fmt.Sprintf("IP: %s is reserved for pod: %s", ir.IP.String(), ir.PodRef)
}

const (
	// Allocate operation identifier
	Allocate = 0
	// Deallocate operation identifier
	Deallocate = 1
)

var ErrNoIPRanges = errors.New("no IP ranges in whereabouts config")
