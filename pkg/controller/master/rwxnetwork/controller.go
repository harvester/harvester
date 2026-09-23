package rwxnetwork

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"reflect"
	"slices"
	"sort"
	"strings"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	networkutils "github.com/harvester/harvester-network-controller/pkg/utils"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlnetworkv1 "github.com/harvester/harvester/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	networkutil "github.com/harvester/harvester/pkg/util/network"
)

const (
	ControllerName        = "harvester-rwx-host-network-controller"
	HostNetworkConfigName = "rwx-network"

	ReasonHostIPRangeExhausted = "HostIPRangeExhausted"

	hostNetworkConfigModeStatic = "static"
)

// Handler prepares the Harvester hosts for the RWX network: it reserves the hostIPRange
// and vipRange of the rwx-network setting in the source NAD, and gives every eligible
// node an address on the RWX network through a HostNetworkConfig.
type Handler struct {
	settings          ctlharvesterv1.SettingClient
	settingCache      ctlharvesterv1.SettingCache
	settingController ctlharvesterv1.SettingController
	nads              ctlcniv1.NetworkAttachmentDefinitionClient
	nadCache          ctlcniv1.NetworkAttachmentDefinitionCache
	hncs              ctlnetworkv1.HostNetworkConfigClient
	hncCache          ctlnetworkv1.HostNetworkConfigCache
	vlanConfigCache   ctlnetworkv1.VlanConfigCache
	nodeCache         ctlcorev1.NodeCache
	recorder          record.EventRecorder
}

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	settingController := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	nads := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()
	hncs := management.HarvesterNetworkFactory.Network().V1beta1().HostNetworkConfig()
	vlanConfigs := management.HarvesterNetworkFactory.Network().V1beta1().VlanConfig()
	nodes := management.CoreFactory.Core().V1().Node()

	h := &Handler{
		settings:          settingController,
		settingCache:      settingController.Cache(),
		settingController: settingController,
		nads:              nads,
		nadCache:          nads.Cache(),
		hncs:              hncs,
		hncCache:          hncs.Cache(),
		vlanConfigCache:   vlanConfigs.Cache(),
		nodeCache:         nodes.Cache(),
		recorder:          management.NewRecorder(ControllerName, "", ""),
	}

	settingController.OnChange(ctx, ControllerName, h.OnSettingChange)
	nodes.OnChange(ctx, ControllerName, func(_ string, node *corev1.Node) (*corev1.Node, error) {
		h.enqueue()
		return node, nil
	})
	vlanConfigs.OnChange(ctx, ControllerName, func(_ string, vc *networkv1.VlanConfig) (*networkv1.VlanConfig, error) {
		h.enqueue()
		return vc, nil
	})
	hncs.OnChange(ctx, ControllerName, func(_ string, hnc *networkv1.HostNetworkConfig) (*networkv1.HostNetworkConfig, error) {
		if hnc == nil || hnc.Labels[util.RWXNetworkManagedLabel] == "true" {
			h.enqueue()
		}
		return hnc, nil
	})
	return nil
}

func (h *Handler) enqueue() {
	h.settingController.Enqueue(settings.RWXNetworkSettingName)
}

func (h *Handler) OnSettingChange(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return setting, nil
	}

	switch setting.Name {
	case settings.StorageNetworkName:
		// The storage network NAD is the source NAD in share mode.
		h.enqueue()
		return setting, nil
	case settings.RWXNetworkSettingName:
		return h.reconcile(setting)
	}
	return setting, nil
}

func (h *Handler) reconcile(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	rwxConfig, err := settings.DecodeConfig[settings.RWXNetworkConfig](setting.EffectiveValue())
	if err != nil {
		return setting, err
	}
	if rwxConfig == nil || rwxConfig.HostIPRange == "" || rwxConfig.VIPRange == "" {
		return h.teardown(setting)
	}

	nadKey, err := h.sourceNAD(setting, rwxConfig)
	if err != nil {
		return setting, err
	}
	if nadKey == "" {
		// The setting is updated again once the storage network controller has the NAD ready.
		return setting, nil
	}

	namespace, name, err := cache.SplitMetaNamespaceKey(nadKey)
	if err != nil {
		return setting, err
	}
	nad, err := h.nadCache.Get(namespace, name)
	if err != nil {
		return setting, fmt.Errorf("failed to get RWX source NAD %s: %w", nadKey, err)
	}
	network, err := networkutil.ParseBridgeNADConfig(nad.Spec.Config)
	if err != nil {
		return setting, fmt.Errorf("failed to parse RWX source NAD %s: %w", nadKey, err)
	}

	if err := h.syncNADExcludes(nadKey, []string{rwxConfig.HostIPRange, rwxConfig.VIPRange}); err != nil {
		return setting, err
	}

	unassigned, err := h.syncHostNetworkConfig(network, rwxConfig.HostIPRange)
	if err != nil {
		return setting, err
	}

	return h.setHostIPsAssigned(setting, rwxConfig.HostIPRange, unassigned)
}

func (h *Handler) teardown(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if err := h.syncNADExcludes("", nil); err != nil {
		return setting, err
	}

	hnc, err := h.managedHostNetworkConfig()
	if err != nil {
		return setting, err
	}
	if hnc != nil && hnc.DeletionTimestamp == nil {
		if err := h.hncs.Delete(hnc.Name, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return setting, err
		}
	}

	settingCopy := setting.DeepCopy()
	settingCopy.Status.Conditions = slices.DeleteFunc(settingCopy.Status.Conditions, func(c harvesterv1.Condition) bool {
		return c.Type == harvesterv1.SettingHostIPsAssigned
	})
	if len(settingCopy.Status.Conditions) == len(setting.Status.Conditions) {
		return setting, nil
	}
	return h.settings.Update(settingCopy)
}

// sourceNAD returns the namespaced name of the NAD carrying RWX traffic.
func (h *Handler) sourceNAD(setting *harvesterv1.Setting, rwxConfig *settings.RWXNetworkConfig) (string, error) {
	if !rwxConfig.ShareStorageNetwork {
		return setting.Annotations[util.RWXNadNetworkAnnotation], nil
	}

	storageNetwork, err := h.settingCache.Get(settings.StorageNetworkName)
	if err != nil {
		return "", fmt.Errorf("failed to get %s setting: %w", settings.StorageNetworkName, err)
	}
	return storageNetwork.Annotations[util.NadStorageNetworkAnnotation], nil
}

// syncNADExcludes reserves the desired ranges in the source NAD and releases them from
// any other NAD, e.g. the storage network NAD after leaving share mode.
func (h *Handler) syncNADExcludes(sourceNADKey string, desired []string) error {
	nads, err := h.nadCache.List(util.HarvesterSystemNamespaceName, labels.Everything())
	if err != nil {
		return err
	}

	for _, nad := range nads {
		var want []string
		if nad.Namespace+"/"+nad.Name == sourceNADKey {
			want = desired
		}
		if err := h.setNADManagedExcludes(nad, want); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) setNADManagedExcludes(nad *nadv1.NetworkAttachmentDefinition, want []string) error {
	if nad.DeletionTimestamp != nil {
		return nil
	}

	var previous []string
	annotation, annotated := nad.Annotations[util.RWXManagedExcludeAnnotation]
	if annotated {
		if err := json.Unmarshal([]byte(annotation), &previous); err != nil {
			return fmt.Errorf("failed to decode annotation %s of NAD %s/%s: %w", util.RWXManagedExcludeAnnotation, nad.Namespace, nad.Name, err)
		}
	}
	if !annotated && len(want) == 0 {
		return nil
	}

	config, configChanged, err := networkutil.SetManagedExcludes(nad.Spec.Config, previous, want)
	if err != nil {
		return fmt.Errorf("failed to update excludes of NAD %s/%s: %w", nad.Namespace, nad.Name, err)
	}

	nadCopy := nad.DeepCopy()
	nadCopy.Spec.Config = config
	if len(want) == 0 {
		delete(nadCopy.Annotations, util.RWXManagedExcludeAnnotation)
	} else {
		wantJSON, err := json.Marshal(want)
		if err != nil {
			return err
		}
		if nadCopy.Annotations == nil {
			nadCopy.Annotations = map[string]string{}
		}
		nadCopy.Annotations[util.RWXManagedExcludeAnnotation] = string(wantJSON)
	}

	if !configChanged && reflect.DeepEqual(nad.Annotations, nadCopy.Annotations) {
		return nil
	}
	if _, err := h.nads.Update(nadCopy); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// syncHostNetworkConfig reconciles the managed HostNetworkConfig and returns the eligible
// nodes left without an address.
func (h *Handler) syncHostNetworkConfig(network networkutil.BridgeNAD, hostIPRange string) ([]string, error) {
	subnet, err := netip.ParsePrefix(network.Range)
	if err != nil {
		return nil, err
	}

	nodes, err := h.eligibleNodes(network.ClusterNetwork)
	if err != nil {
		return nil, err
	}

	hnc, err := h.hncCache.Get(HostNetworkConfigName)
	if apierrors.IsNotFound(err) {
		hnc = nil
	} else if err != nil {
		return nil, err
	}
	if hnc != nil && hnc.Labels[util.RWXNetworkManagedLabel] != "true" {
		return nil, fmt.Errorf("HostNetworkConfig %s is not managed by Harvester", hnc.Name)
	}

	current := map[string]string{}
	if hnc != nil && hnc.Spec.ClusterNetwork == network.ClusterNetwork && hnc.Spec.VlanID == network.Vlan {
		for node, ip := range hnc.Spec.HostIPs {
			if prefix, err := netip.ParsePrefix(string(ip)); err == nil && prefix.Bits() == subnet.Bits() {
				current[node] = prefix.Addr().String()
			}
		}
	}

	assigned, unassigned, err := networkutil.AssignHostIPs(hostIPRange, current, nodes)
	if err != nil {
		return nil, err
	}
	desired := newHostNetworkConfig(network, subnet.Bits(), assigned)

	switch {
	case hnc == nil:
		if desired != nil {
			if _, err := h.hncs.Create(desired); err != nil {
				return nil, err
			}
		}
	case hnc.DeletionTimestamp != nil:
		// The HostNetworkConfig watch requeues once the deletion completes.
	case desired == nil || needsRecreate(hnc, desired):
		if err := h.hncs.Delete(hnc.Name, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
	case !reflect.DeepEqual(hnc.Spec, desired.Spec):
		hncCopy := hnc.DeepCopy()
		hncCopy.Spec = desired.Spec
		if _, err := h.hncs.Update(hncCopy); err != nil {
			return nil, err
		}
	}

	return unassigned, nil
}

// eligibleNodes returns the non-witness nodes that the cluster network spans.
func (h *Handler) eligibleNodes(clusterNetwork string) ([]string, error) {
	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var spanned map[string]bool
	if clusterNetwork != networkutils.ManagementClusterNetworkName {
		spanned, err = h.nodesSpannedByVlanConfigs(clusterNetwork)
		if err != nil {
			return nil, err
		}
	}

	var eligible []string
	for _, node := range nodes {
		if node.DeletionTimestamp != nil || util.IsWitnessNodeWithoutPromotionStatus(node) {
			continue
		}
		if spanned != nil && !spanned[node.Name] {
			continue
		}
		eligible = append(eligible, node.Name)
	}
	return eligible, nil
}

func (h *Handler) nodesSpannedByVlanConfigs(clusterNetwork string) (map[string]bool, error) {
	vcs, err := h.vlanConfigCache.List(labels.Set{networkutils.KeyClusterNetworkLabel: clusterNetwork}.AsSelector())
	if err != nil {
		return nil, err
	}

	spanned := map[string]bool{}
	for _, vc := range vcs {
		matched := vc.Annotations[networkutils.KeyMatchedNodes]
		if matched == "" {
			continue
		}
		var nodes []string
		if err := json.Unmarshal([]byte(matched), &nodes); err != nil {
			return nil, fmt.Errorf("failed to decode matched nodes of VlanConfig %s: %w", vc.Name, err)
		}
		for _, node := range nodes {
			spanned[node] = true
		}
	}
	return spanned, nil
}

func (h *Handler) managedHostNetworkConfig() (*networkv1.HostNetworkConfig, error) {
	hnc, err := h.hncCache.Get(HostNetworkConfigName)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if hnc.Labels[util.RWXNetworkManagedLabel] != "true" {
		return nil, nil
	}
	return hnc, nil
}

func (h *Handler) setHostIPsAssigned(setting *harvesterv1.Setting, hostIPRange string, unassigned []string) (*harvesterv1.Setting, error) {
	cond := harvesterv1.SettingHostIPsAssigned
	settingCopy := setting.DeepCopy()

	var message string
	if len(unassigned) == 0 {
		cond.True(settingCopy)
		cond.Reason(settingCopy, "")
	} else {
		message = fmt.Sprintf("no address left in hostIPRange %s for node(s) %s", hostIPRange, strings.Join(unassigned, ", "))
		cond.False(settingCopy)
		cond.Reason(settingCopy, ReasonHostIPRangeExhausted)
	}
	cond.Message(settingCopy, message)

	if reflect.DeepEqual(settingCopy.Status, setting.Status) {
		return setting, nil
	}
	if len(unassigned) > 0 {
		h.recorder.Event(settingCopy, corev1.EventTypeWarning, ReasonHostIPRangeExhausted, message)
	}
	return h.settings.Update(settingCopy)
}

func newHostNetworkConfig(network networkutil.BridgeNAD, prefixBits int, assigned map[string]string) *networkv1.HostNetworkConfig {
	if len(assigned) == 0 {
		return nil
	}

	nodes := make([]string, 0, len(assigned))
	ips := make(map[string]networkv1.IPAddr, len(assigned))
	for node, ip := range assigned {
		nodes = append(nodes, node)
		ips[node] = networkv1.IPAddr(fmt.Sprintf("%s/%d", ip, prefixBits))
	}
	sort.Strings(nodes)

	return &networkv1.HostNetworkConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:   HostNetworkConfigName,
			Labels: map[string]string{util.RWXNetworkManagedLabel: "true"},
		},
		Spec: networkv1.HostNetworkConfigSpec{
			Description:    fmt.Sprintf("Managed by Harvester for the %s setting", settings.RWXNetworkSettingName),
			ClusterNetwork: network.ClusterNetwork,
			VlanID:         network.Vlan,
			Mode:           hostNetworkConfigModeStatic,
			HostIPs:        ips,
			// The network controller requires a static IP for every node the selector matches.
			NodeSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      corev1.LabelHostname,
					Operator: metav1.LabelSelectorOpIn,
					Values:   nodes,
				}},
			},
		},
	}
}

// needsRecreate reports whether the HostNetworkConfig must be replaced rather than
// updated. Its cluster network and VLAN are immutable, and the network controller
// agent does not reapply the address of an interface it has already set up.
func needsRecreate(current, desired *networkv1.HostNetworkConfig) bool {
	if current.Spec.ClusterNetwork != desired.Spec.ClusterNetwork || current.Spec.VlanID != desired.Spec.VlanID {
		return true
	}
	for node, ip := range desired.Spec.HostIPs {
		if currentIP, ok := current.Spec.HostIPs[node]; ok && currentIP != ip {
			return true
		}
	}
	return false
}
