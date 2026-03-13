package sharemanager

import (
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strings"
	"time"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/sirupsen/logrus"
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

	shareManagerStaticIPStatusNoSourceNAD       = "storage network setting does not reference a source NAD"
	shareManagerStaticIPStatusSourceNADNotFound = "source NAD is not available"
	shareManagerStaticIPStatusNonStorageNAD     = "source NAD is not marked as a storage network"
	shareManagerStaticIPStatusNoExcludeRange    = "source NAD has no exclude range for static IP allocation"
	shareManagerStaticIPStatusStaticNADPending  = "waiting for the derived static NAD"
	shareManagerStaticIPStatusPoolPending       = "waiting for the derived static IP pool usage resource"
)

type Handler struct {
	shareManagers          ctllonghornv1.ShareManagerClient
	shareManagerController ctllonghornv1.ShareManagerController
	shareManagerCache      ctllonghornv1.ShareManagerCache
	settingsCache          ctlharvesterv1.SettingCache
	nads                   ctlcniv1.NetworkAttachmentDefinitionClient
	nadCache               ctlcniv1.NetworkAttachmentDefinitionCache
	ipPoolUsages           ctlharvesterv1.IPPoolUsageClient
	ipPoolUsageCache       ctlharvesterv1.IPPoolUsageCache
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

	if err := h.syncSourceNetworkAnnotations(mCpy); err != nil {
		return m, err
	}
	if err := h.syncStaticNetworkAnnotations(mCpy); err != nil {
		return m, err
	}
	if !shareManagerHasUsableStaticNetwork(mCpy) {
		delete(mCpy.Annotations, util.ShareManagerIPAnnotation)
	} else if err := h.ensureExcludePoolAllocation(mCpy); err != nil {
		return m, err
	}

	if reflect.DeepEqual(mCpy.Annotations, m.Annotations) {
		return m, nil
	}

	annotations := mCpy.Annotations
	iface := annotations[util.ShareManagerIfaceAnnotation]
	ipCIDR := annotations[util.ShareManagerIPAnnotation]
	nadName := annotations[util.ShareManagerNADAnnotation]
	staticNADName := annotations[util.ShareManagerStaticNADAnnotation]

	logrus.Infof("Updating ShareManager %s/%s with requested IP %s on %s via source NAD %s and static NAD %s",
		m.Namespace, m.Name, ipCIDR, iface, nadName, staticNADName)
	return h.shareManagers.Update(mCpy)

}

func (h *Handler) syncSourceNetworkAnnotations(m *longhornv1beta2.ShareManager) error {
	if m == nil || h.settingsCache == nil {
		return nil
	}

	storageNetworkSetting, err := h.settingsCache.Get(settings.StorageNetworkName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			delete(m.Annotations, util.ShareManagerNADAnnotation)
			return nil
		}
		return err
	}

	sourceNADName := storageNetworkSetting.Annotations[util.NadStorageNetworkAnnotation]
	if sourceNADName == "" {
		clearNetworkAnnotationsWithReason(m, shareManagerStaticIPStatusNoSourceNAD)
		return nil
	}

	sourceNAD, err := h.getSourceNAD(sourceNADName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			clearNetworkAnnotationsWithReason(m, shareManagerStaticIPStatusSourceNADNotFound)
			return nil
		}
		return err
	}
	if sourceNAD.Annotations[util.StorageNetworkAnnotation] != "true" {
		clearNetworkAnnotationsWithReason(m, shareManagerStaticIPStatusNonStorageNAD)
		return nil
	}

	m.Annotations[util.ShareManagerNADAnnotation] = sourceNADName
	return nil
}

func (h *Handler) OnShareManagerRemove(_ string, m *longhornv1beta2.ShareManager) (*longhornv1beta2.ShareManager, error) {
	if m == nil || m.Namespace != util.LonghornSystemNamespaceName {
		return m, nil
	}

	sourceNADName := m.Annotations[util.ShareManagerNADAnnotation]
	if sourceNADName == "" || h.ipPoolUsages == nil {
		return m, nil
	}

	sourceNAD, err := h.getSourceNAD(sourceNADName)
	if err != nil {
		return m, err
	}
	if err := h.releaseExcludePoolAllocation(m, sourceNAD); err != nil {
		return m, err
	}

	return m, nil
}

func (h *Handler) getSourceNAD(name string) (*networkv1.NetworkAttachmentDefinition, error) {
	nadNamespace, nadName, err := parseNADNamespacedName(name)
	if err != nil {
		return nil, err
	}
	if nadNamespace == "" {
		nadNamespace = util.StorageNetworkNetAttachDefNamespace
	}
	return h.nadCache.Get(nadNamespace, nadName)
}

func (h *Handler) syncStaticNetworkAnnotations(m *longhornv1beta2.ShareManager) error {
	if m == nil {
		return nil
	}

	sourceNADName := m.Annotations[util.ShareManagerNADAnnotation]
	sourceNAD, err := h.getSourceNAD(sourceNADName)
	if err != nil {
		return err
	}

	excludeCIDR, err := getSingleExcludeRange(sourceNAD)
	if err != nil {
		return err
	}
	if excludeCIDR == "" {
		clearNetworkAnnotationsWithReason(m, shareManagerStaticIPStatusNoExcludeRange)
		return nil
	}

	staticNADName := staticNADName(sourceNAD)
	if _, err := h.nadCache.Get(sourceNAD.Namespace, staticNADName); err != nil {
		if apierrors.IsNotFound(err) {
			clearNetworkAnnotationsWithReason(m, shareManagerStaticIPStatusStaticNADPending)
			h.enqueueShareManager(m.Namespace, m.Name)
			return nil
		}
		return err
	}
	if _, err := h.ipPoolUsageCache.Get(excludeIPPoolUsageName(sourceNAD)); err != nil {
		if apierrors.IsNotFound(err) {
			clearNetworkAnnotationsWithReason(m, shareManagerStaticIPStatusPoolPending)
			h.enqueueShareManager(m.Namespace, m.Name)
			return nil
		}
		return err
	}

	delete(m.Annotations, util.ShareManagerStaticIPStatusAnnotation)
	m.Annotations[util.ShareManagerStaticNADAnnotation] = fmt.Sprintf("%s/%s", sourceNAD.Namespace, staticNADName)
	return nil
}

func (h *Handler) ensureExcludePoolAllocation(m *longhornv1beta2.ShareManager) error {
	if h.ipPoolUsages == nil || h.ipPoolUsageCache == nil {
		return nil
	}

	sourceNADName := m.Annotations[util.ShareManagerNADAnnotation]
	if sourceNADName == "" {
		return nil
	}

	sourceNAD, err := h.getSourceNAD(sourceNADName)
	if err != nil {
		return err
	}
	excludeCIDR, err := getSingleExcludeRange(sourceNAD)
	if err != nil {
		return err
	}
	if excludeCIDR == "" {
		return nil
	}

	ipAddress, err := h.allocateIPFromPool(excludeIPPoolUsageName(sourceNAD), m)
	if err != nil {
		return err
	}

	ipCIDR, err := ipToCIDR(ipAddress, sourceNAD)
	if err != nil {
		return err
	}
	m.Annotations[util.ShareManagerIPAnnotation] = ipCIDR
	return nil
}

func shareManagerWantsStaticIP(m *longhornv1beta2.ShareManager) bool {
	annotations := m.Annotations
	return strings.EqualFold(annotations[util.ShareManagerStaticIPAnnotation], "true") &&
		annotations[util.ShareManagerIfaceAnnotation] != ""
}

func shareManagerHasUsableStaticNetwork(m *longhornv1beta2.ShareManager) bool {
	annotations := m.Annotations
	return annotations[util.ShareManagerNADAnnotation] != "" && annotations[util.ShareManagerStaticNADAnnotation] != ""
}

func clearNetworkAnnotations(m *longhornv1beta2.ShareManager) {
	delete(m.Annotations, util.ShareManagerIPAnnotation)
	delete(m.Annotations, util.ShareManagerNADAnnotation)
	delete(m.Annotations, util.ShareManagerStaticNADAnnotation)
	delete(m.Annotations, util.ShareManagerStaticIPStatusAnnotation)
}

func clearNetworkAnnotationsWithReason(m *longhornv1beta2.ShareManager, reason string) {
	clearNetworkAnnotations(m)
	m.Annotations[util.ShareManagerStaticIPStatusAnnotation] = reason
}

func (h *Handler) enqueueShareManager(namespace, name string) {
	if h.shareManagerController == nil {
		return
	}
	h.shareManagerController.Enqueue(namespace, name)
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

func getSingleExcludeRange(sourceNAD *networkv1.NetworkAttachmentDefinition) (string, error) {
	bridgeConfig := utilnetwork.BridgeConfig{}
	if err := json.Unmarshal([]byte(sourceNAD.Spec.Config), &bridgeConfig); err != nil {
		return "", fmt.Errorf("failed to decode NAD %s/%s config: %w", sourceNAD.Namespace, sourceNAD.Name, err)
	}
	if len(bridgeConfig.IPAM.Exclude) == 0 {
		return "", nil
	}
	if len(bridgeConfig.IPAM.Exclude) > 1 {
		return "", fmt.Errorf("network attachment definition %s/%s has more than one exclude range", sourceNAD.Namespace, sourceNAD.Name)
	}
	return bridgeConfig.IPAM.Exclude[0], nil
}

func staticNADName(sourceNAD *networkv1.NetworkAttachmentDefinition) string {
	return sourceNAD.Name + "-static"
}

func excludeIPPoolUsageName(sourceNAD *networkv1.NetworkAttachmentDefinition) string {
	return fmt.Sprintf("%s-%s-exclude", sourceNAD.Namespace, sourceNAD.Name)
}

func (h *Handler) allocateIPFromPool(poolName string, m *longhornv1beta2.ShareManager) (string, error) {
	owner := shareManagerResourceRef(m)
	for attempt := 0; attempt < 10; attempt++ {
		pool, err := h.ipPoolUsageCache.Get(poolName)
		if err != nil {
			return "", err
		}

		if ipAddress, found := findAllocationByOwner(pool.Status.Allocations, owner); found {
			return ipAddress, nil
		}

		ipAddress, err := findFreeIP(pool)
		if err != nil {
			return "", err
		}

		poolCopy := pool.DeepCopy()
		if poolCopy.Status.Allocations == nil {
			poolCopy.Status.Allocations = map[string]harvesterv1beta1.IPPoolAllocation{}
		}

		now := metav1.NewTime(time.Now().UTC())
		poolCopy.Status.Allocations[ipAddress] = harvesterv1beta1.IPPoolAllocation{
			Resource:       owner,
			AllocatedAt:    &now,
			LastUpdateTime: &now,
		}

		if _, err := h.ipPoolUsages.UpdateStatus(poolCopy); err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			return "", err
		}
		return ipAddress, nil
	}

	return "", fmt.Errorf("failed to allocate IP from pool %s after retries", poolName)
}

func (h *Handler) releaseExcludePoolAllocation(m *longhornv1beta2.ShareManager, sourceNAD *networkv1.NetworkAttachmentDefinition) error {
	if h.ipPoolUsages == nil || h.ipPoolUsageCache == nil {
		return nil
	}

	poolName := excludeIPPoolUsageName(sourceNAD)
	owner := shareManagerResourceRef(m)
	for attempt := 0; attempt < 10; attempt++ {
		pool, err := h.ipPoolUsageCache.Get(poolName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		ipAddress, found := findAllocationByOwner(pool.Status.Allocations, owner)
		if !found {
			return nil
		}

		poolCopy := pool.DeepCopy()
		delete(poolCopy.Status.Allocations, ipAddress)
		if _, err := h.ipPoolUsages.UpdateStatus(poolCopy); err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			return err
		}
		return nil
	}

	return fmt.Errorf("failed to release IP allocation from pool %s after retries", poolName)
}

func shareManagerResourceRef(m *longhornv1beta2.ShareManager) harvesterv1beta1.IPPoolUsageResourceRef {
	return harvesterv1beta1.IPPoolUsageResourceRef{
		APIVersion: longhornv1beta2.SchemeGroupVersion.String(),
		Kind:       shareManagerKind,
		Namespace:  m.Namespace,
		Name:       m.Name,
		UID:        m.UID,
	}
}

func findAllocationByOwner(
	allocations map[string]harvesterv1beta1.IPPoolAllocation,
	owner harvesterv1beta1.IPPoolUsageResourceRef,
) (string, bool) {
	for ipAddress, allocation := range allocations {
		if allocation.Resource == owner {
			return ipAddress, true
		}
	}
	return "", false
}

func findFreeIP(pool *harvesterv1beta1.IPPoolUsage) (string, error) {
	_, networkCIDR, err := net.ParseCIDR(pool.Spec.CIDR)
	if err != nil {
		return "", fmt.Errorf("failed to parse pool CIDR %q: %w", pool.Spec.CIDR, err)
	}

	startIP, endIP, err := allocatableIPRange(networkCIDR)
	if err != nil {
		return "", err
	}

	allocated := map[string]struct{}{}
	for ipAddress := range pool.Status.Allocations {
		allocated[ipAddress] = struct{}{}
	}
	for _, ipAddress := range reservedIPsForPool(pool) {
		allocated[ipAddress] = struct{}{}
	}

	for ip := append(net.IP(nil), startIP...); compareIPs(ip, endIP) <= 0; ip = nextIP(ip) {
		if _, exists := allocated[ip.String()]; exists {
			continue
		}
		return ip.String(), nil
	}

	return "", fmt.Errorf("no free IPs left in pool %s", pool.Name)
}

func reservedIPsForPool(pool *harvesterv1beta1.IPPoolUsage) []string {
	_, networkCIDR, err := net.ParseCIDR(pool.Spec.CIDR)
	if err != nil {
		return nil
	}
	startIP, endIP, err := allocatableIPRange(networkCIDR)
	if err != nil {
		return nil
	}

	reservedCount := effectiveReservedIPCount(pool.Spec.ReservedIPCount)
	if reservedCount == 0 {
		return nil
	}

	reservedIPs := make([]string, 0, reservedCount)
	for ip := append(net.IP(nil), startIP...); compareIPs(ip, endIP) <= 0 && len(reservedIPs) < reservedCount; ip = nextIP(ip) {
		reservedIPs = append(reservedIPs, ip.String())
	}
	return reservedIPs
}

func effectiveReservedIPCount(count int) int {
	if count == 0 {
		return util.IPPoolUsageDefaultReservedIPCount
	}
	if count < 0 {
		return 0
	}
	return count
}

func allocatableIPRange(cidr *net.IPNet) (net.IP, net.IP, error) {
	networkIP := cidr.IP.Mask(cidr.Mask)
	if networkIP == nil {
		return nil, nil, fmt.Errorf("failed to mask CIDR %s", cidr.String())
	}

	maskSize, totalBits := cidr.Mask.Size()
	startIP := append(net.IP(nil), networkIP...)
	endIP := lastIPInSubnet(cidr)

	if startIP.To4() != nil && totalBits == 32 && maskSize < 31 {
		startIP = nextIP(startIP)
		endIP = previousIP(endIP)
	}
	if compareIPs(startIP, endIP) > 0 {
		return nil, nil, fmt.Errorf("CIDR %s has no allocatable IPs", cidr.String())
	}

	return startIP, endIP, nil
}

func lastIPInSubnet(cidr *net.IPNet) net.IP {
	endIP := append(net.IP(nil), cidr.IP.Mask(cidr.Mask)...)
	for i := range endIP {
		endIP[i] |= ^cidr.Mask[i]
	}
	return endIP
}

func nextIP(ip net.IP) net.IP {
	next := append(net.IP(nil), ip...)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	return next
}

func previousIP(ip net.IP) net.IP {
	prev := append(net.IP(nil), ip...)
	for i := len(prev) - 1; i >= 0; i-- {
		if prev[i] == 0 {
			prev[i] = 255
			continue
		}
		prev[i]--
		break
	}
	return prev
}

func compareIPs(left, right net.IP) int {
	left16 := left.To16()
	right16 := right.To16()
	for i := 0; i < len(left16) && i < len(right16); i++ {
		if left16[i] < right16[i] {
			return -1
		}
		if left16[i] > right16[i] {
			return 1
		}
	}
	return 0
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
