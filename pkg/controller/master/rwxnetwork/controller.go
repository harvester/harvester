package rwxnetwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"slices"
	"sort"
	"strings"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	networkutils "github.com/harvester/harvester-network-controller/pkg/utils"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	whereaboutsv1alpha1 "github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
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
	ctlwhereaboutsv1 "github.com/harvester/harvester/pkg/generated/controllers/whereabouts.cni.cncf.io/v1alpha1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	networkutil "github.com/harvester/harvester/pkg/util/network"
)

const (
	ControllerName        = "harvester-rwx-host-network-controller"
	HostNetworkConfigName = "rwx-network"

	ReasonHostIPRangeExhausted      = "HostIPRangeExhausted"
	ReasonHostNetworkConfigConflict = "HostNetworkConfigConflict"
	ReasonAddressInUse              = "AddressInUse"
	ReasonHostNetworkConfigMismatch = "HostNetworkConfigMismatch"

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
	ipPoolCache       ctlwhereaboutsv1.IPPoolCache
	recorder          record.EventRecorder
}

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	settingController := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	nads := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()
	hncs := management.HarvesterNetworkFactory.Network().V1beta1().HostNetworkConfig()
	vlanConfigs := management.HarvesterNetworkFactory.Network().V1beta1().VlanConfig()
	nodes := management.CoreFactory.Core().V1().Node()
	ipPools := management.WhereaboutsCNIFactory.Whereabouts().V1alpha1().IPPool()

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
		ipPoolCache:       ipPools.Cache(),
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
	// A released address may unblock the reserved ranges.
	ipPools.OnChange(ctx, ControllerName, func(_ string, pool *whereaboutsv1alpha1.IPPool) (*whereaboutsv1alpha1.IPPool, error) {
		h.enqueue()
		return pool, nil
	})

	registerShareManagerVIP(ctx, management)
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

	hnc, err := h.hncCache.Get(HostNetworkConfigName)
	if err != nil && !apierrors.IsNotFound(err) {
		return setting, err
	}
	if err == nil && hnc.Labels[util.RWXNetworkManagedLabel] != "true" {
		// The HostNetworkConfig watch requeues once the foreign one is removed.
		return h.setHostIPsAssignedCondition(setting, false, ReasonHostNetworkConfigConflict,
			fmt.Sprintf("HostNetworkConfig %s exists but is not managed by Harvester", HostNetworkConfigName))
	}

	// Pods may have got an address in the ranges before they were excluded. Hold the
	// host network until those addresses are released rather than hand them out twice.
	inUse, err := h.reservedAddrsInUse(network.Range, rwxConfig.HostIPRange, rwxConfig.VIPRange)
	if err != nil {
		return setting, err
	}
	if len(inUse) > 0 {
		return h.setHostIPsAssignedCondition(setting, false, ReasonAddressInUse,
			fmt.Sprintf("address(es) in hostIPRange or vipRange still allocated: %s", strings.Join(inUse, ", ")))
	}

	unassigned, err := h.syncHostNetworkConfig(network, rwxConfig.HostIPRange)
	if errors.Is(err, errHostNetworkConfigMismatch) {
		return h.setHostIPsAssignedCondition(setting, false, ReasonHostNetworkConfigMismatch,
			fmt.Sprintf("HostNetworkConfig %s no longer matches the %s setting, remove hostIPRange and vipRange and set them again",
				HostNetworkConfigName, settings.RWXNetworkSettingName))
	} else if err != nil {
		return setting, err
	}

	if len(unassigned) > 0 {
		return h.setHostIPsAssignedCondition(setting, false, ReasonHostIPRangeExhausted,
			fmt.Sprintf("no address left in hostIPRange %s for node(s) %s", rwxConfig.HostIPRange, strings.Join(unassigned, ", ")))
	}
	return h.setHostIPsAssignedCondition(setting, true, "", "")
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

// reservedAddrsInUse returns the Whereabouts allocations of the subnet that fall in the
// given ranges, as "address (pod)".
func (h *Handler) reservedAddrsInUse(subnet string, ranges ...string) ([]string, error) {
	poolName, err := networkutil.WhereaboutsIPPoolName(subnet)
	if err != nil {
		return nil, err
	}
	pool, err := h.ipPoolCache.Get(util.KubeSystemNamespace, poolName)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	allocations, err := networkutil.IPPoolAllocations(pool)
	if err != nil {
		return nil, err
	}

	var inUse []string
	for addr, podRef := range allocations {
		for _, r := range ranges {
			if prefix, err := netip.ParsePrefix(r); err == nil && prefix.Contains(addr) {
				inUse = append(inUse, fmt.Sprintf("%s (%s)", addr, podRef))
				break
			}
		}
	}
	sort.Strings(inUse)
	return inUse, nil
}

// errHostNetworkConfigMismatch reports a managed HostNetworkConfig that cannot be updated
// in place to match the setting.
var errHostNetworkConfigMismatch = errors.New("managed HostNetworkConfig does not match the setting")

// syncHostNetworkConfig reconciles the managed HostNetworkConfig and returns the eligible
// nodes left without an address.
//
// The HostNetworkConfig is only removed on teardown. Removing it tears down the host
// interface together with every VIP kube-vip announces on it, and kube-vip does not
// recover those VIPs once the interface comes back.
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
	case desired == nil:
		// Keep the current one: the node and VlanConfig caches may not have caught up
		// yet, e.g. right after a leader change.
	case !updatableInPlace(hnc, desired):
		return nil, errHostNetworkConfigMismatch
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

// setHostIPsAssignedCondition updates the hostIPsAssigned condition, and records a
// Warning event whenever it turns false with a new message.
func (h *Handler) setHostIPsAssignedCondition(setting *harvesterv1.Setting, assigned bool, reason, message string) (*harvesterv1.Setting, error) {
	cond := harvesterv1.SettingHostIPsAssigned
	settingCopy := setting.DeepCopy()
	if assigned {
		cond.True(settingCopy)
	} else {
		cond.False(settingCopy)
	}
	cond.Reason(settingCopy, reason)
	cond.Message(settingCopy, message)

	if reflect.DeepEqual(settingCopy.Status, setting.Status) {
		return setting, nil
	}
	if !assigned {
		h.recorder.Event(settingCopy, corev1.EventTypeWarning, reason, message)
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

// updatableInPlace reports whether the HostNetworkConfig can be updated to the desired
// spec. Its cluster network and VLAN are immutable, and the network controller agent
// does not reapply the address of an interface it has already set up.
func updatableInPlace(current, desired *networkv1.HostNetworkConfig) bool {
	if current.Spec.ClusterNetwork != desired.Spec.ClusterNetwork || current.Spec.VlanID != desired.Spec.VlanID {
		return false
	}
	for node, ip := range desired.Spec.HostIPs {
		if currentIP, ok := current.Spec.HostIPs[node]; ok && currentIP != ip {
			return false
		}
	}
	return true
}
