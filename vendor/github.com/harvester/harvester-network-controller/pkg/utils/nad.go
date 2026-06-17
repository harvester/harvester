package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	cniv1 "github.com/containernetworking/cni/pkg/types"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	ctlcniv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/k8s.cni.cncf.io/v1"
)

const (
	CNITypeKubeOVN = "kube-ovn"
	ovnProvider    = "ovn"

	CNITypeBridge       = "bridge"
	CNITypeDefaultEmpty = "" // potential empty type, is treated as CNITypeBridge
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
	L2VlanNetwork      NetworkType = "L2VlanNetwork"
	L2VlanTrunkNetwork NetworkType = "L2VlanTrunkNetwork"
	UntaggedNetwork    NetworkType = "UntaggedNetwork"
	OverlayNetwork     NetworkType = "OverlayNetwork"

	InvalidNetwork NetworkType = "InvalidNetwork"
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

func OutdateLayer3NetworkConfPerMode(conf string) (string, error) {
	if conf == "" {
		return "", nil
	}

	layer3NetworkConf := Layer3NetworkConf{}
	if err := json.Unmarshal([]byte(conf), &layer3NetworkConf); err != nil {
		return "", fmt.Errorf("unmarshal %s failed, error: %w", conf, err)
	}

	if layer3NetworkConf.Mode == Auto {
		layer3NetworkConf.Outdated = true
	} else {
		// e.g. both vid and route mode are changed, ensure all legacy fields are wiped out
		layer3NetworkConf.Outdated = false
	}

	outdatedRoute, err := json.Marshal(layer3NetworkConf)
	if err != nil {
		return "", fmt.Errorf("marshal %v failed, error: %w", layer3NetworkConf, err)
	}

	return string(outdatedRoute), nil
}

func OutdateNadLayer3NetworkConf(nad *nadv1.NetworkAttachmentDefinition, nadConf *NetConf) (map[string]string, error) {
	if nad == nil {
		return nil, nil
	}
	annotations := nad.Annotations
	routeAnnotation := annotations[KeyNetworkRoute]
	// no route annotation, nothing to do
	if routeAnnotation == "" {
		return annotations, nil
	}

	// both untag and trunk have vlan id 0, just remove route annotation
	if nadConf != nil && nadConf.Vlan == 0 {
		delete(annotations, KeyNetworkRoute)
		return annotations, nil
	}

	// mark the vlan related route as outdated when it is auto mode
	newRouteAnnotation, err := OutdateLayer3NetworkConfPerMode(routeAnnotation)
	if err != nil {
		return nil, err
	}
	annotations[KeyNetworkRoute] = newRouteAnnotation

	return annotations, nil
}

// caller ensurs this function is only called when oldNad and newNad have same vlan id
// the first return is not nil only when real change happens
func OutdateNadLayer3NetworkConfWhenRouteModeChanges(oldNad, newNad *nadv1.NetworkAttachmentDefinition) (map[string]string, error) {
	if newNad == nil || oldNad == nil {
		return nil, nil
	}
	annotations := newNad.Annotations
	newRouteAnnotation := annotations[KeyNetworkRoute]
	oldRouteAnnotation := oldNad.Annotations[KeyNetworkRoute]
	// no route annotation, nothing to do
	if newRouteAnnotation == "" || oldRouteAnnotation == "" {
		return nil, nil
	}

	newL3Conf := Layer3NetworkConf{}
	if err := json.Unmarshal([]byte(newRouteAnnotation), &newL3Conf); err != nil {
		return nil, fmt.Errorf("unmarshal new nad route %v failed, error: %w", newL3Conf, err)
	}

	oldL3Conf := Layer3NetworkConf{}
	if err := json.Unmarshal([]byte(oldRouteAnnotation), &oldL3Conf); err != nil {
		return nil, fmt.Errorf("unmarshal old nad route %v failed, error: %w", oldL3Conf, err)
	}

	// e.g. when helper updates route
	if newL3Conf.Mode == oldL3Conf.Mode {
		return nil, nil
	}

	if newL3Conf.Mode == Auto {
		newL3Conf.Outdated = true
	} else {
		// e.g. both vid and route mode are changed, ensure all legacy fields are wiped out
		newL3Conf.Outdated = false
	}

	outdatedRoute, err := json.Marshal(newL3Conf)
	if err != nil {
		return nil, fmt.Errorf("marshal new route %v failed, error: %w", newL3Conf, err)
	}
	annotations[KeyNetworkRoute] = string(outdatedRoute)

	return annotations, nil
}

func NewLayer3NetworkConfFromNad(nad *nadv1.NetworkAttachmentDefinition) (*Layer3NetworkConf, error) {
	networkConf := &Layer3NetworkConf{}
	if nad == nil {
		return networkConf, nil
	}

	routeStr := nad.Annotations[KeyNetworkRoute]
	if routeStr == "" {
		// return and wait this label
		return networkConf, nil
	}

	if err := json.Unmarshal([]byte(routeStr), networkConf); err != nil {
		return nil, fmt.Errorf("unmarshal nad %v/%v annotation %v %s faield, error: %w", nad.Namespace, nad.Name, KeyNetworkRoute, routeStr, err)
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

func GetNadLabel(nad *nadv1.NetworkAttachmentDefinition, key string) string {
	if nad == nil || nad.Labels == nil {
		return ""
	}
	return nad.Labels[key]
}

func SetNadLabel(nad *nadv1.NetworkAttachmentDefinition, key, value string) {
	if nad == nil {
		return
	}
	if nad.Labels == nil {
		nad.Labels = make(map[string]string)
	}
	nad.Labels[key] = value
}

func GetNadAnnotation(nad *nadv1.NetworkAttachmentDefinition, key string) string {
	if nad == nil || nad.Annotations == nil {
		return ""
	}
	return nad.Annotations[key]
}
func SetNadAnnotation(nad *nadv1.NetworkAttachmentDefinition, key, value string) {
	if nad == nil {
		return
	}
	if nad.Annotations == nil {
		nad.Annotations = make(map[string]string)
	}
	nad.Annotations[key] = value
}

func (c *Layer3NetworkConf) ToString() (string, error) {
	bytes, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func (c *Layer3NetworkConf) GetDHCPServerIPAddr() string {
	if c == nil {
		return ""
	}
	return c.ServerIPAddr
}

func SetDHCPInfo2JobLabels(lb map[string]string, l2netconf *NetConf, l3netconf *Layer3NetworkConf) {
	if lb == nil {
		return
	}
	if l2netconf != nil {
		lb[KeyVlanLabel] = l2netconf.GetVlanString()
	}
	if l3netconf != nil {
		lb[KeyVlanDHCPServerIP] = l3netconf.GetDHCPServerIPAddr()
	}
}

func AreJobLabelsDHCPInfoUnchanged(lb map[string]string, l2netconf *NetConf, l3netconf *Layer3NetworkConf) bool {
	if lb == nil {
		return false
	}
	if l2netconf != nil && l3netconf != nil {
		return lb[KeyVlanLabel] == l2netconf.GetVlanString() && lb[KeyVlanDHCPServerIP] == l3netconf.GetDHCPServerIPAddr()
	}
	return false
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

// L2 mode
type NetConf struct {
	cniv1.NetConf
	BrName       string       `json:"bridge"`
	IsGW         bool         `json:"isGateway"`
	IsDefaultGW  bool         `json:"isDefaultGateway"`
	ForceAddress bool         `json:"forceAddress"`
	IPMasq       bool         `json:"ipMasq"`
	MTU          int          `json:"mtu"`
	HairpinMode  bool         `json:"hairpinMode"`
	PromiscMode  bool         `json:"promiscMode"`
	Vlan         int          `json:"vlan"`
	Provider     string       `json:"provider"`
	VlanTrunk    []*VlanTrunk `json:"vlanTrunk,omitempty"`
}

type VlanTrunk struct {
	MinID *int `json:"minID,omitempty"`
	MaxID *int `json:"maxID,omitempty"`
	ID    *int `json:"id,omitempty"`
}

// honor https://github.com/containernetworking/plugins/blob/48a4ae5ab5f12966c935a793b1316ec13dd05a3e/plugins/main/bridge/bridge.go#L147
// with some updates
func (item *VlanTrunk) IsValid() (bool, error) {
	if item == nil {
		return true, nil
	}

	switch {
	case item.MinID != nil && item.MaxID != nil:
		minID := *item.MinID
		if minID < MinTrunkVlanID || minID > MaxVlanID {
			return false, fmt.Errorf("incorrect trunk minID parameter %v", minID)
		}
		maxID := *item.MaxID
		if maxID < MinTrunkVlanID || maxID > MaxVlanID {
			return false, fmt.Errorf("incorrect trunk maxID parameter %v", maxID)
		}
		if maxID < minID {
			return false, fmt.Errorf("minID %v is greater than maxID %v in trunk parameter", minID, maxID)
		}
	case item.MinID == nil && item.MaxID != nil:
		return false, errors.New("minID and maxID should be configured simultaneously, minID is missing")
	case item.MinID != nil && item.MaxID == nil:
		return false, errors.New("minID and maxID should be configured simultaneously, maxID is missing")
	}

	// single vid
	if item.ID != nil {
		ID := *item.ID
		if ID < MinTrunkVlanID || ID > MaxVlanID {
			return false, fmt.Errorf("incorrect trunk id parameter %v", ID)
		}
	}

	if item.ID == nil && item.MinID == nil && item.MaxID == nil {
		return false, fmt.Errorf("all fields in this VlanTrunk are empty")
	}

	return true, nil
}

// if vlanconfig is valid
func (nc *NetConf) IsVlanConfigValid() (bool, error) {
	if nc.Vlan < MinVlanID || nc.Vlan > MaxVlanID {
		return false, fmt.Errorf("vlan %v is out of range [%v .. %v]", nc.Vlan, MinVlanID, MaxVlanID)
	}

	// when VlanTrunk is set, the Vlan can only 0
	if nc.Vlan != 0 && len(nc.VlanTrunk) > 0 {
		return false, fmt.Errorf("vlan can only be 0 when vlanTrunk is set")
	}

	for _, vt := range nc.VlanTrunk {
		if _, err := vt.IsValid(); err != nil {
			return false, err
		}
	}

	return true, nil
}

func (nc *NetConf) dumpVlanIDSet() (*VlanIDSet, error) {
	// l2 tag mode or untag mode
	if nc.IsVlanAccessMode() {
		return NewVlanIDSetFromSingleVID(nc.Vlan)
	}

	// trunk mode
	vis := NewVlanIDSet()
	var err error
	for _, vt := range nc.VlanTrunk {
		if vt.ID != nil {
			if err = vis.SetVID(*vt.ID); err != nil {
				return nil, err
			}
		}
		if vt.MinID != nil && vt.MaxID != nil {
			for i := *vt.MinID; i <= *vt.MaxID; i++ {
				if err = vis.SetVID(i); err != nil {
					return nil, err
				}
			}
		}
	}
	return vis, nil
}

// return vidsets from all bridge nads
func NewVlanIDSetFromNadList(nads []*nadv1.NetworkAttachmentDefinition) (*VlanIDSet, error) {
	vis := NewVlanIDSet()
	if len(nads) == 0 {
		// do not return nil, but an initialized VlanIDSet for following processings
		return vis, nil
	}

	for _, nad := range nads {
		if nad.DeletionTimestamp != nil {
			continue
		}
		nc, err := DecodeNadConfigToNetConf(nad)
		if err != nil {
			return nil, err
		}
		// skip non-bridge CNI
		if !nc.IsBridgeCNI() {
			continue
		}
		tmpvis, err := nc.dumpVlanIDSet()
		if err != nil {
			return nil, err
		}
		if err := vis.Append(tmpvis); err != nil {
			return nil, err
		}
	}

	return vis, nil
}

// if VlanTrunk is configured
func (nc *NetConf) IsVlanTrunkMode() bool {
	return len(nc.VlanTrunk) > 0
}

// the default mode
func (nc *NetConf) IsVlanAccessMode() bool {
	return len(nc.VlanTrunk) == 0
}

func (nc *NetConf) IsL2VlanNetwork() bool {
	return nc.Vlan != 0
}

func (nc *NetConf) IsUntaggedNetwork() bool {
	return nc.Vlan == 0 && len(nc.VlanTrunk) == 0
}

func (nc *NetConf) IsL2VlanTrunkNetwork() bool {
	return nc.Vlan == 0 && len(nc.VlanTrunk) > 0
}

func (nc *NetConf) IsBridgeCNI() bool {
	return nc.Type == CNITypeBridge || nc.Type == CNITypeDefaultEmpty
}

func (nc *NetConf) IsKubeOVNCNI() bool {
	return nc.Type == CNITypeKubeOVN
}

func (nc *NetConf) GetVlanID() int {
	if nc.IsVlanAccessMode() {
		return nc.Vlan
	}
	return 0
}

func (nc *NetConf) GetNetworkType() (NetworkType, error) {
	switch nc.Type {
	case CNITypeKubeOVN:
		return OverlayNetwork, nil
	case CNITypeBridge, CNITypeDefaultEmpty:
		switch {
		case nc.Vlan != 0:
			return L2VlanNetwork, nil
		case nc.Vlan == 0 && len(nc.VlanTrunk) == 0:
			return UntaggedNetwork, nil
		case nc.Vlan == 0 && len(nc.VlanTrunk) != 0:
			return L2VlanTrunkNetwork, nil
		}
	}
	return "", fmt.Errorf("unknown network type")
}

func (nc *NetConf) GetVlan() int {
	return nc.Vlan
}

func (nc *NetConf) GetVlanString() string {
	// if vlan is unset, it's value is 0
	if nc == nil {
		return "0"
	}
	return fmt.Sprintf("%v", nc.Vlan)
}

func (nc *NetConf) SetNetworkInfoToLabels(label map[string]string) error {
	if label == nil {
		return nil
	}
	delete(label, KeyVlanLabel) // avoid dangling vid label
	if nc.IsKubeOVNCNI() {
		// OVN is only attached to mgmt network for now
		label[KeyClusterNetworkLabel] = ManagementClusterNetworkName
		label[KeyNetworkType] = string(OverlayNetwork)
		return nil
	}

	if !nc.IsBridgeCNI() {
		return fmt.Errorf("cannot determin the network type from netconf type %v", nc.Type)
	}

	// all others are assumed as bridge CNI
	cnname, err := GetBridgeNamePrefix(nc.BrName)
	if err != nil {
		return err
	}
	label[KeyClusterNetworkLabel] = cnname
	if nc.IsL2VlanNetwork() {
		label[KeyNetworkType] = string(L2VlanNetwork)
		label[KeyVlanLabel] = strconv.Itoa(nc.Vlan)
		return nil
	}
	if nc.IsL2VlanTrunkNetwork() {
		label[KeyNetworkType] = string(L2VlanTrunkNetwork)
		return nil
	}
	if nc.IsUntaggedNetwork() {
		label[KeyNetworkType] = string(UntaggedNetwork)
		return nil
	}
	return fmt.Errorf("cannot determin the l2 network type from netconf vid %v length of vlantrunk %v", nc.Vlan, len(nc.VlanTrunk))
}

func (nc *NetConf) GetClusterNetworkName() (string, error) {
	// OVN is only attached to mgmt network for now
	if nc.IsKubeOVNCNI() {
		return ManagementClusterNetworkName, nil
	}

	if !nc.IsBridgeCNI() {
		return "", fmt.Errorf("cannot determin the network type from netconf type %v", nc.Type)
	}

	// all others are assumed as bridge CNI
	cnname, err := GetBridgeNamePrefix(nc.BrName)
	if err != nil {
		return "", err
	}
	return cnname, nil
}

func IsNadNetworkLabelSet(nad *nadv1.NetworkAttachmentDefinition) bool {
	// the label KeyNetworkReady is not included when making decision; cluster network is responsible for updating this label
	if nad.Labels != nil && nad.Labels[KeyNetworkType] != "" && nad.Labels[KeyClusterNetworkLabel] != "" {
		return true
	}

	return false
}

func IsVlanNad(nad *nadv1.NetworkAttachmentDefinition) bool {
	if nad == nil || nad.Spec.Config == "" || nad.Labels == nil || nad.Labels[KeyVlanLabel] == "" ||
		nad.Labels[KeyNetworkType] == "" || nad.Labels[KeyClusterNetworkLabel] == "" {
		return false
	}

	return true
}

func IsOverlayNad(nad *nadv1.NetworkAttachmentDefinition) bool {
	if nad != nil && nad.Labels != nil && nad.Labels[KeyNetworkType] == string(OverlayNetwork) {
		return true
	}

	return false
}

// decode nad config string to a config struct
func DecodeNadConfigToNetConf(nad *nadv1.NetworkAttachmentDefinition) (*NetConf, error) {
	conf := &NetConf{}
	if nad == nil || nad.Spec.Config == "" {
		return conf, nil
	}

	if err := json.Unmarshal([]byte(nad.Spec.Config), conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal nad %v/%v config %s %w", nad.Namespace, nad.Name, nad.Spec.Config, err)
	}

	return conf, nil
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
	if nad == nil || nad.Namespace != HarvesterSystemNamespaceName {
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

// list all nads
func (n *NadGetter) ListAllNads() ([]*nadv1.NetworkAttachmentDefinition, error) {
	nads, err := n.nadCache.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		return nil, err
	}

	if len(nads) == 0 {
		return nil, nil
	}

	return nads, nil
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
	nads, err := n.nadCache.List(HarvesterSystemNamespaceName, labels.Set(map[string]string{
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

func GeVlanIDSetFromClusterNetwork(cnName string, nadCache ctlcniv1.NetworkAttachmentDefinitionCache) (*VlanIDSet, error) {
	ng := NewNadGetter(nadCache)
	nads, err := ng.ListNadsOnClusterNetwork(cnName)
	if err != nil {
		return nil, err
	}
	vis, err := NewVlanIDSetFromNadList(nads)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster network %v vlan id set, error: %w", cnName, err)
	}
	return vis, nil
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
