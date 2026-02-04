package utils

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	cniv1 "github.com/containernetworking/cni/pkg/types"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	harvesterutil "github.com/harvester/harvester/pkg/util"
)

const (
	CNITypeKubeOVN = "kube-ovn"
	ovnProvider    = "ovn"
)

type Connectivity string

const (
	Connectable   Connectivity = "true"
	Unconnectable Connectivity = "false"
	DHCPFailed    Connectivity = "DHCP failed"
	PingFailed    Connectivity = "ping failed"
)

type Mode string

const (
	Auto   Mode = "auto"
	Manual Mode = "manual"
)

type NetworkType string

const (
	L2VlanNetwork   NetworkType = "L2VlanNetwork"
	UntaggedNetwork NetworkType = "UntaggedNetwork"
	OverlayNetwork  NetworkType = "OverlayNetwork"
)

type NadSelectedNetworks []nadv1.NetworkSelectionElement

type Layer3NetworkConf struct {
	Mode         Mode         `json:"mode,omitempty"`
	CIDR         string       `json:"cidr,omitempty"`
	Gateway      string       `json:"gateway,omitempty"`
	ServerIPAddr string       `json:"serverIPAddr,omitempty"`
	Connectivity Connectivity `json:"connectivity,omitempty"`
	Outdated     bool         `json:"outdated,omitempty"`
}

func NewLayer3NetworkConf(conf string) (*Layer3NetworkConf, error) {
	if conf == "" {
		return &Layer3NetworkConf{}, nil
	}

	networkConf := &Layer3NetworkConf{}

	if err := json.Unmarshal([]byte(conf), networkConf); err != nil {
		return nil, fmt.Errorf("unmarshal %s faield, error: %w", conf, err)
	}

	// validate
	if networkConf.Mode != "" && networkConf.Mode != Auto && networkConf.Mode != Manual {
		return nil, fmt.Errorf("unknown mode %s", networkConf.Mode)
	}

	// validate cidr and gateway when the mode is manual
	if networkConf.Mode == Manual {
		_, ipnet, err := net.ParseCIDR(networkConf.CIDR)
		if err != nil || (ipnet != nil && isMaskZero(ipnet)) {
			return nil, fmt.Errorf("the CIDR %s is invalid", networkConf.CIDR)
		}

		if net.ParseIP(networkConf.Gateway) == nil {
			return nil, fmt.Errorf("the gateway %s is invalid", networkConf.Gateway)
		}
	}

	return networkConf, nil
}

func (c *Layer3NetworkConf) ToString() (string, error) {
	bytes, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func NewNADSelectedNetworks(conf string) (NadSelectedNetworks, error) {
	networks := make([]nadv1.NetworkSelectionElement, 1)
	if err := json.Unmarshal([]byte(conf), &networks); err != nil {
		return nil, err
	}

	return networks, nil
}

func (n NadSelectedNetworks) ToString() (string, error) {
	bytes, err := json.Marshal(n)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

type NetConf struct {
	cniv1.NetConf
	BrName       string `json:"bridge"`
	IsGW         bool   `json:"isGateway"`
	IsDefaultGW  bool   `json:"isDefaultGateway"`
	ForceAddress bool   `json:"forceAddress"`
	IPMasq       bool   `json:"ipMasq"`
	MTU          int    `json:"mtu"`
	HairpinMode  bool   `json:"hairpinMode"`
	PromiscMode  bool   `json:"promiscMode"`
	Vlan         int    `json:"vlan"`
	Provider     string `json:"provider"`
}

func IsVlanNad(nad *nadv1.NetworkAttachmentDefinition) bool {
	if nad == nil || nad.Spec.Config == "" || nad.Labels == nil || nad.Labels[KeyNetworkType] == "" ||
		nad.Labels[KeyClusterNetworkLabel] == "" || nad.Labels[KeyVlanLabel] == "" {
		return false
	}

	return true
}

func isMaskZero(ipnet *net.IPNet) bool {
	for _, b := range ipnet.Mask {
		if b != 0 {
			return false
		}
	}

	return true
}

// if this nad is a storagenetwork nad
func IsStorageNetworkNad(nad *nadv1.NetworkAttachmentDefinition) bool {
	if nad == nil || nad.Namespace != harvesterutil.HarvesterSystemNamespaceName {
		return false
	}

	// seems Harvester webhook has no protection on this annotation
	if nad.Annotations != nil && nad.Annotations[StorageNetworkAnnotation] == "true" {
		return true
	}

	// check name
	if strings.HasPrefix(nad.Name, StorageNetworkNetAttachDefPrefix) {
		return true
	}

	return false
}

// filter the first active storage network nad from a list of nads
func FilterFirstActiveStorageNetworkNad(nads []*nadv1.NetworkAttachmentDefinition) *nadv1.NetworkAttachmentDefinition {
	if len(nads) == 0 {
		return nil
	}
	for _, nad := range nads {
		if IsStorageNetworkNad(nad) && nad.DeletionTimestamp == nil {
			return nad
		}
	}
	return nil
}

type NadGetter struct {
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache
}

func NewNadGetter(nadCache ctlcniv1.NetworkAttachmentDefinitionCache) *NadGetter {
	return &NadGetter{nadCache: nadCache}
}

// list all nads attached to a cluster network
func (n *NadGetter) ListNadsOnClusterNetwork(cnName string) ([]*nadv1.NetworkAttachmentDefinition, error) {
	nads, err := n.nadCache.List(corev1.NamespaceAll, labels.Set(map[string]string{
		KeyClusterNetworkLabel: cnName,
	}).AsSelector())
	if err != nil {
		return nil, err
	}

	if len(nads) == 0 {
		return nil, nil
	}
	return nads, nil
}

func (n *NadGetter) GetFirstActiveStorageNetworkNadOnClusterNetwork(cnName string) (*nadv1.NetworkAttachmentDefinition, error) {
	nads, err := n.nadCache.List(harvesterutil.HarvesterSystemNamespaceName, labels.Set(map[string]string{
		KeyClusterNetworkLabel: cnName,
	}).AsSelector())
	if err != nil {
		return nil, err
	}

	if len(nads) == 0 {
		return nil, nil
	}

	return FilterFirstActiveStorageNetworkNad(nads), nil
}

func (n *NadGetter) NadNamesOnClusterNetwork(cnName string) ([]string, error) {
	nads, err := n.ListNadsOnClusterNetwork(cnName)
	if err != nil {
		return nil, err
	}
	return generateNadNameList(nads), nil
}

func generateNadNameList(nads []*nadv1.NetworkAttachmentDefinition) []string {
	if len(nads) == 0 {
		return nil
	}
	nadStrList := make([]string, len(nads))
	for i, nad := range nads {
		nadStrList[i] = nad.Namespace + "/" + nad.Name
	}
	return nadStrList
}

func GetNadNameFromProvider(provider string) (nadName string, nadNamespace string, err error) {
	if provider == "" {
		return "", "", fmt.Errorf("provider is empty for cni type %s", CNITypeKubeOVN)
	}

	nad := strings.Split(provider, ".")
	if len(nad) < 3 {
		return "", "", fmt.Errorf("invalid provider length %d for provider %s", len(nad), provider)
	}

	return nad[0], nad[1], nil
}

func GetProviderFromNad(nadName string, nadNamespace string) (provider string, err error) {
	if nadName == "" {
		return "", fmt.Errorf("nad name %s is empty", nadName)
	}

	if nadNamespace == "" {
		nadNamespace = defaultNamespace
	}

	return nadName + "." + nadNamespace + "." + ovnProvider, nil
}
