package sharemanager

import (
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strings"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	utilnetwork "github.com/harvester/harvester/pkg/util/network"
)

const (
	shareManagerKind = "ShareManager"

	statusNoSourceNAD       = "rwx network configuration does not reference a source NAD"
	statusSourceNADNotFound = "source NAD is not available"
	statusInvalidSourceNAD  = "source NAD is not marked as a RWX network"
	statusStaticNADPending  = "waiting for the derived static NAD"
	statusWaitingForPod     = "waiting for the share manager pod network identity"
)

type Handler struct {
	shareManagers          ctllonghornv1.ShareManagerClient
	shareManagerController ctllonghornv1.ShareManagerController
	shareManagerCache      ctllonghornv1.ShareManagerCache
	podCache               ctlcorev1.PodCache
	settingsController     ctlharvesterv1.SettingController
	settingsCache          ctlharvesterv1.SettingCache
	nads                   ctlcniv1.NetworkAttachmentDefinitionClient
	nadCache               ctlcniv1.NetworkAttachmentDefinitionCache
}

type shareManagerNetworkPlan struct {
	name                string
	namespace           string
	annotations         map[string]string
	enqueueRWXSetting   bool
	enqueueShareManager bool
}

// newShareManagerNetworkPlan snapshots the current annotations so reconciliation
// can compute the final desired state before mutating the ShareManager object.
func newShareManagerNetworkPlan(m *longhornv1beta2.ShareManager) *shareManagerNetworkPlan {
	annotations := map[string]string{}
	if m == nil {
		return &shareManagerNetworkPlan{annotations: annotations}
	}
	if m.Annotations != nil {
		for key, value := range m.Annotations {
			annotations[key] = value
		}
	}

	return &shareManagerNetworkPlan{
		name:        m.Name,
		namespace:   m.Namespace,
		annotations: annotations,
	}
}

func (p *shareManagerNetworkPlan) apply(m *longhornv1beta2.ShareManager) {
	m.Annotations = p.annotations
}

func (p *shareManagerNetworkPlan) runPostActions(h *Handler, m *longhornv1beta2.ShareManager) {
	if p.enqueueRWXSetting {
		h.enqueueRWXNetworkSetting()
	}
	if p.enqueueShareManager {
		h.enqueueShareManager(m.Namespace, m.Name)
	}
}

func (p *shareManagerNetworkPlan) get(key string) string {
	return p.annotations[key]
}

func (p *shareManagerNetworkPlan) set(key, value string) {
	p.annotations[key] = value
}

func (p *shareManagerNetworkPlan) delete(keys ...string) {
	for _, key := range keys {
		delete(p.annotations, key)
	}
}

func (p *shareManagerNetworkPlan) hasUsableStaticNetwork() bool {
	return p.get(util.ShareManagerNADAnno) != "" && p.get(util.ShareManagerStaticNADAnno) != ""
}

// resetNetwork clears the full static-network state when the source NAD is not usable.
func (p *shareManagerNetworkPlan) resetNetwork(reason string) {
	p.delete(
		util.ShareManagerNADAnno,
		util.ShareManagerStaticNADAnno,
		util.ShareManagerIPAnno,
		util.ShareManagerMACAnno,
	)
	p.set(util.ShareManagerStaticIPStatusAnno, reason)
}

func (p *shareManagerNetworkPlan) clearLearnedNetworkIdentity() {
	p.delete(util.ShareManagerIPAnno, util.ShareManagerMACAnno)
}

// resetIdentity clears learned pod identity while keeping the NAD selections intact.
func (p *shareManagerNetworkPlan) resetIdentity(status string) {
	p.clearLearnedNetworkIdentity()
	p.set(util.ShareManagerStaticIPStatusAnno, status)
}

func (p *shareManagerNetworkPlan) setIdentity(ipCIDR, mac string) {
	p.set(util.ShareManagerIPAnno, ipCIDR)
	p.set(util.ShareManagerMACAnno, mac)
	p.delete(util.ShareManagerStaticIPStatusAnno)
}

// handleMissingStaticNAD keeps the source NAD but drops the derived static NAD
// and learned identity until the RWX setting controller recreates it.
func (p *shareManagerNetworkPlan) handleMissingStaticNAD() {
	p.delete(util.ShareManagerStaticNADAnno)
	p.resetIdentity(statusStaticNADPending)
	p.enqueueRWXSetting = true
	p.enqueueShareManager = true
}

func (h *Handler) OnShareManagerChange(_ string, m *longhornv1beta2.ShareManager) (*longhornv1beta2.ShareManager, error) {
	if m == nil || m.DeletionTimestamp != nil || m.Namespace != util.LonghornSystemNamespaceName {
		return m, nil
	}
	if !shareManagerWantsStaticIP(m) {
		return m, nil
	}

	mCpy := m.DeepCopy()
	if mCpy.Annotations == nil {
		mCpy.Annotations = map[string]string{}
	}
	if err := h.syncShareManagerNetwork(m, mCpy); err != nil {
		return m, err
	}

	if reflect.DeepEqual(mCpy.Annotations, m.Annotations) {
		return m, nil
	}

	logrus.Infof("Updating ShareManager %s/%s network settings: iface=%s ip=%s mac=%s staticNAD=%s",
		m.Namespace,
		m.Name,
		mCpy.Annotations[util.ShareManagerIfaceAnno],
		mCpy.Annotations[util.ShareManagerIPAnno],
		mCpy.Annotations[util.ShareManagerMACAnno],
		mCpy.Annotations[util.ShareManagerStaticNADAnno],
	)
	return h.shareManagers.Update(mCpy)
}

func (h *Handler) syncShareManagerNetwork(oldSM, newSM *longhornv1beta2.ShareManager) error {
	plan := newShareManagerNetworkPlan(newSM)

	if err := h.syncSourceNADAnnotations(plan); err != nil {
		return err
	}
	if err := h.handleSourceNADChange(oldSM, plan); err != nil {
		return err
	}
	if err := h.syncStaticNADAnnotations(plan); err != nil {
		return err
	}
	if !plan.hasUsableStaticNetwork() {
		plan.clearLearnedNetworkIdentity()
	} else if err := h.syncNetworkIdentity(plan); err != nil {
		return err
	}

	plan.apply(newSM)
	plan.runPostActions(h, newSM)
	return nil
}

func (h *Handler) syncSourceNADAnnotations(plan *shareManagerNetworkPlan) error {
	if plan == nil || h.settingsCache == nil {
		return nil
	}

	sourceNADName, err := h.getShareManagerSourceNADRef()
	if err != nil {
		return err
	}
	if sourceNADName == "" {
		plan.resetNetwork(statusNoSourceNAD)
		return nil
	}

	sourceNAD, err := h.getNAD(sourceNADName)
	if apierrors.IsNotFound(err) {
		plan.resetNetwork(statusSourceNADNotFound)
		return nil
	}
	if err != nil {
		return err
	}

	if !isShareManagerSourceNAD(sourceNAD) {
		plan.resetNetwork(statusInvalidSourceNAD)
		return nil
	}

	plan.set(util.ShareManagerNADAnno, sourceNADName)
	return nil
}

func (h *Handler) getShareManagerSourceNADRef() (string, error) {
	rwxSetting, err := h.getRWXNetworkSetting()
	if err != nil {
		return "", err
	}
	return h.getCurrentSourceNADRef(rwxSetting)
}

func (h *Handler) isShareManagerNetworkConfigured() (bool, error) {
	sourceNADRef, err := h.getShareManagerSourceNADRef()
	if err != nil {
		return false, err
	}
	return sourceNADRef != "", nil
}

func (h *Handler) getRWXNetworkSetting() (*harvesterv1beta1.Setting, error) {
	if h.settingsCache == nil {
		return nil, nil
	}

	setting, err := h.settingsCache.Get(settings.RWXNetworkSettingName)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return setting, nil
}

func isShareManagerSourceNAD(sourceNAD *networkv1.NetworkAttachmentDefinition) bool {
	if sourceNAD == nil {
		return false
	}
	return sourceNAD.Annotations[util.StorageNetworkAnnotation] == "true" ||
		sourceNAD.Annotations[util.RWXNetworkAnnotation] == "true"
}

func (h *Handler) OnShareManagerRemove(_ string, m *longhornv1beta2.ShareManager) (*longhornv1beta2.ShareManager, error) {
	if m == nil || m.Namespace != util.LonghornSystemNamespaceName {
		return m, nil
	}

	if err := h.removeSrcNADExclude(
		m.Annotations[util.ShareManagerNADAnno],
		m.Annotations[util.ShareManagerIPAnno],
	); err != nil {
		return m, err
	}

	return m, nil
}

func (h *Handler) OnPodChange(_ string, pod *corev1.Pod) (*corev1.Pod, error) {
	if pod == nil || pod.Namespace != util.LonghornSystemNamespaceName {
		return pod, nil
	}

	shareManagerName, ok := shareManagerOwnerNameFromRefs(pod.OwnerReferences)
	if !ok {
		return pod, nil
	}

	h.enqueueShareManager(pod.Namespace, shareManagerName)
	return pod, nil
}

func (h *Handler) OnPodRemove(_ string, pod *corev1.Pod) (*corev1.Pod, error) {
	if pod == nil || pod.Namespace != util.LonghornSystemNamespaceName {
		return pod, nil
	}

	shareManagerName, ok := shareManagerOwnerNameFromRefs(pod.OwnerReferences)
	if !ok {
		return pod, nil
	}

	manager, err := h.shareManagerCache.Get(pod.Namespace, shareManagerName)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return pod, err
	}

	configured, err := h.isShareManagerNetworkConfigured()
	if err != nil {
		return pod, err
	}
	if !configured {
		return nil, nil
	}

	if !shareManagerWantsStaticIP(manager) || !hasCompleteStaticNetwork(manager) {
		h.enqueueShareManager(manager.Namespace, manager.Name)
		return pod, fmt.Errorf("waiting for ShareManager %s/%s static IP configuration before removing pod %s",
			manager.Namespace, manager.Name, pod.Name)
	}

	return nil, nil
}

func (h *Handler) getNAD(name string) (*networkv1.NetworkAttachmentDefinition, error) {
	nadNamespace, nadName, err := parseNADNamespacedName(name)
	if err != nil {
		return nil, err
	}
	if nadNamespace == "" {
		nadNamespace = util.StorageNetworkNetAttachDefNamespace
	}
	return h.nadCache.Get(nadNamespace, nadName)
}

func (h *Handler) syncStaticNADAnnotations(plan *shareManagerNetworkPlan) error {
	if plan == nil {
		return nil
	}

	sourceNADName := plan.get(util.ShareManagerNADAnno)
	if sourceNADName == "" {
		plan.delete(util.ShareManagerStaticNADAnno)
		return nil
	}

	sourceNAD, err := h.getNAD(sourceNADName)
	if err != nil {
		return err
	}

	staticNADName := staticNADName(sourceNAD)

	_, err = h.nadCache.Get(sourceNAD.Namespace, staticNADName)
	if apierrors.IsNotFound(err) {
		plan.handleMissingStaticNAD()
		return nil
	}

	if err != nil {
		return err
	}

	plan.set(util.ShareManagerStaticNADAnno, fmt.Sprintf("%s/%s", sourceNAD.Namespace, staticNADName))
	return nil
}

func shareManagerWantsStaticIP(m *longhornv1beta2.ShareManager) bool {
	annotations := m.Annotations
	return strings.EqualFold(annotations[util.ShareManagerStaticIPAnno], "true") &&
		annotations[util.ShareManagerIfaceAnno] != ""
}

func hasUsableStaticNetwork(m *longhornv1beta2.ShareManager) bool {
	annotations := m.Annotations
	return annotations[util.ShareManagerNADAnno] != "" && annotations[util.ShareManagerStaticNADAnno] != ""
}

func hasCompleteStaticNetwork(m *longhornv1beta2.ShareManager) bool {
	if !hasUsableStaticNetwork(m) {
		return false
	}

	annotations := m.Annotations
	return annotations[util.ShareManagerIPAnno] != "" &&
		annotations[util.ShareManagerMACAnno] != "" &&
		annotations[util.ShareManagerStaticIPStatusAnno] == ""
}

func (p *shareManagerNetworkPlan) hasIdentity() bool {
	return p.get(util.ShareManagerIPAnno) != "" &&
		p.get(util.ShareManagerMACAnno) != ""
}

func (h *Handler) syncNetworkIdentity(plan *shareManagerNetworkPlan) error {
	sourceNADName := plan.get(util.ShareManagerNADAnno)
	if sourceNADName == "" {
		return nil
	}

	sourceNAD, err := h.getNAD(sourceNADName)
	if err != nil {
		return err
	}

	shareManager := &longhornv1beta2.ShareManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:        plan.name,
			Namespace:   plan.namespace,
			Annotations: plan.annotations,
		},
	}
	ipCIDR, mac, found, err := h.findShareManagerNetworkIdentity(shareManager, sourceNAD)
	if err != nil {
		return err
	}
	if !found {
		if plan.hasIdentity() {
			return nil
		}
		plan.resetIdentity(statusWaitingForPod)
		return nil
	}

	plan.setIdentity(ipCIDR, mac)
	return h.ensureSrcNADExclude(sourceNAD, ipCIDR)
}

func (h *Handler) handleSourceNADChange(oldSM *longhornv1beta2.ShareManager, plan *shareManagerNetworkPlan) error {
	previousSrcNAD := oldSM.Annotations[util.ShareManagerNADAnno]
	currentSrcNAD := plan.get(util.ShareManagerNADAnno)
	if previousSrcNAD == "" || previousSrcNAD == currentSrcNAD {
		return nil
	}

	if err := h.removeSrcNADExclude(previousSrcNAD, oldSM.Annotations[util.ShareManagerIPAnno]); err != nil {
		return err
	}
	plan.clearLearnedNetworkIdentity()
	return nil
}

func (h *Handler) enqueueShareManager(namespace, name string) {
	if h.shareManagerController == nil {
		return
	}
	h.shareManagerController.Enqueue(namespace, name)
}

func (h *Handler) enqueueRWXNetworkSetting() {
	if h.settingsController == nil {
		return
	}
	h.settingsController.Enqueue(settings.RWXNetworkSettingName)
}

func (h *Handler) enqueueStaticIPShareManagers() error {
	if h.shareManagerCache == nil || h.shareManagerController == nil {
		return nil
	}

	shareManagers, err := h.shareManagerCache.List(util.LonghornSystemNamespaceName, labels.Everything())
	if err != nil {
		return err
	}
	for _, manager := range shareManagers {
		if !shareManagerWantsStaticIP(manager) {
			continue
		}
		h.shareManagerController.Enqueue(manager.Namespace, manager.Name)
	}

	return nil
}

func ipToCIDR(ip string, sourceNAD *networkv1.NetworkAttachmentDefinition) (string, error) {
	ipRange, err := getIPAMRange(sourceNAD)
	if err != nil {
		return "", err
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "", fmt.Errorf("failed to parse IP %q", ip)
	}

	_, cidr, err := net.ParseCIDR(ipRange)
	if err != nil {
		return "", fmt.Errorf("failed to parse IPAM range %q: %w", ipRange, err)
	}
	prefixSize, _ := cidr.Mask.Size()
	return fmt.Sprintf("%s/%d", parsedIP.String(), prefixSize), nil
}

func (h *Handler) findShareManagerNetworkIdentity(
	m *longhornv1beta2.ShareManager,
	sourceNAD *networkv1.NetworkAttachmentDefinition,
) (string, string, bool, error) {
	if h.podCache == nil {
		return "", "", false, nil
	}

	pods, err := h.podCache.List(m.Namespace, labels.Everything())
	if err != nil {
		return "", "", false, err
	}

	for _, pod := range pods {
		if pod == nil || pod.DeletionTimestamp != nil {
			continue
		}

		shareManagerName, ok := shareManagerOwnerNameFromRefs(pod.OwnerReferences)
		if !ok || shareManagerName != m.Name {
			continue
		}

		ipCIDR, mac, found, err := findNetworkIdentityForInterface(
			pod.Annotations[networkv1.NetworkStatusAnnot],
			m.Annotations[util.ShareManagerIfaceAnno],
			sourceNAD,
		)
		if err != nil {
			return "", "", false, err
		}
		if found {
			return ipCIDR, mac, true, nil
		}
	}

	return "", "", false, nil
}

func findNetworkIdentityForInterface(
	networkStatusJSON string,
	iface string,
	sourceNAD *networkv1.NetworkAttachmentDefinition,
) (string, string, bool, error) {
	if networkStatusJSON == "" || iface == "" {
		return "", "", false, nil
	}

	statuses := []networkv1.NetworkStatus{}
	if err := json.Unmarshal([]byte(networkStatusJSON), &statuses); err != nil {
		return "", "", false, err
	}

	for _, status := range statuses {
		if status.Interface != iface || len(status.IPs) == 0 || status.Mac == "" {
			continue
		}

		ipCIDR, err := ipToCIDR(status.IPs[0], sourceNAD)
		if err != nil {
			return "", "", false, err
		}
		return ipCIDR, status.Mac, true, nil
	}

	return "", "", false, nil
}

func ensureIPExcludeCIDR(ipOrCIDR string) (string, error) {
	if ipOrCIDR == "" {
		return "", nil
	}

	if parsedIP := net.ParseIP(ipOrCIDR); parsedIP != nil {
		if parsedIP.To4() != nil {
			return parsedIP.String() + "/32", nil
		}
		return parsedIP.String() + "/128", nil
	}

	parsedIP, _, err := net.ParseCIDR(ipOrCIDR)
	if err != nil {
		return "", err
	}
	if parsedIP.To4() != nil {
		return parsedIP.String() + "/32", nil
	}
	return parsedIP.String() + "/128", nil
}

func (h *Handler) ensureSrcNADExclude(sourceNAD *networkv1.NetworkAttachmentDefinition, ipCIDR string) error {
	excludeCIDR, err := ensureIPExcludeCIDR(ipCIDR)
	if err != nil || excludeCIDR == "" {
		return err
	}

	return h.updateSrcNADExcludeList(sourceNAD, func(excludes []string) ([]string, bool) {
		for _, existing := range excludes {
			if existing == excludeCIDR {
				return excludes, false
			}
		}
		return append(excludes, excludeCIDR), true
	})
}

func (h *Handler) removeSrcNADExclude(sourceNADName, ipCIDR string) error {
	if sourceNADName == "" || ipCIDR == "" || h.nadCache == nil || h.nads == nil {
		return nil
	}

	sourceNAD, err := h.getNAD(sourceNADName)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	excludeCIDR, err := ensureIPExcludeCIDR(ipCIDR)
	if err != nil || excludeCIDR == "" {
		return err
	}

	return h.updateSrcNADExcludeList(sourceNAD, func(excludes []string) ([]string, bool) {
		filtered := make([]string, 0, len(excludes))
		changed := false
		for _, existing := range excludes {
			if existing == excludeCIDR {
				changed = true
				continue
			}
			filtered = append(filtered, existing)
		}
		return filtered, changed
	})
}

func (h *Handler) updateSrcNADExcludeList(
	sourceNAD *networkv1.NetworkAttachmentDefinition,
	mutate func([]string) ([]string, bool),
) error {
	if sourceNAD == nil || h.nads == nil {
		return nil
	}

	bridgeConfig := utilnetwork.BridgeConfig{}
	if err := json.Unmarshal([]byte(sourceNAD.Spec.Config), &bridgeConfig); err != nil {
		return fmt.Errorf("failed to decode NAD %s/%s config: %w", sourceNAD.Namespace, sourceNAD.Name, err)
	}

	updatedExcludes, changed := mutate(append([]string{}, bridgeConfig.IPAM.Exclude...))
	if !changed {
		return nil
	}
	bridgeConfig.IPAM.Exclude = updatedExcludes

	configBytes, err := json.Marshal(bridgeConfig)
	if err != nil {
		return fmt.Errorf("failed to encode NAD %s/%s config: %w", sourceNAD.Namespace, sourceNAD.Name, err)
	}

	nadCopy := sourceNAD.DeepCopy()
	nadCopy.Spec.Config = string(configBytes)
	_, err = h.nads.Update(nadCopy)
	return err
}

func shareManagerOwnerNameFromRefs(ownerReferences []metav1.OwnerReference) (string, bool) {
	for _, ownerRef := range ownerReferences {
		if ownerRef.Kind == shareManagerKind && ownerRef.Name != "" {
			return ownerRef.Name, true
		}
	}
	return "", false
}

func getIPAMRange(sourceNAD *networkv1.NetworkAttachmentDefinition) (string, error) {
	bridgeConfig := utilnetwork.BridgeConfig{}
	if err := json.Unmarshal([]byte(sourceNAD.Spec.Config), &bridgeConfig); err != nil {
		return "", fmt.Errorf("failed to decode NAD %s/%s config: %w", sourceNAD.Namespace, sourceNAD.Name, err)
	}
	if bridgeConfig.IPAM.Range == "" {
		return "", fmt.Errorf("network attachment definition %s/%s has no IPAM range", sourceNAD.Namespace, sourceNAD.Name)
	}
	return bridgeConfig.IPAM.Range, nil
}

func staticNADName(sourceNAD *networkv1.NetworkAttachmentDefinition) string {
	return sourceNAD.Name + "-static"
}

func parseNADNamespacedName(nad string) (string, string, error) {
	parts := strings.Split(nad, "/")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return "", "", fmt.Errorf("invalid network attachment definition name %q", nad)
		}
		return "", parts[0], nil
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("invalid network attachment definition name %q", nad)
		}
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("invalid network attachment definition name %q", nad)
	}
}
