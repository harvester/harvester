package rwxnetwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	networkutils "github.com/harvester/harvester-network-controller/pkg/utils"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhtypes "github.com/longhorn/longhorn-manager/types"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctldiscoveryv1 "github.com/harvester/harvester/pkg/generated/controllers/discovery.k8s.io/v1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlnetworkv1 "github.com/harvester/harvester/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	networkutil "github.com/harvester/harvester/pkg/util/network"
)

const (
	ShareManagerVIPControllerName = "harvester-rwx-share-manager-vip-controller"

	ReasonVIPRangeExhausted  = "VIPRangeExhausted"
	ReasonVIPServiceConflict = "VIPServiceConflict"

	vipServicePrefix                    = "rwx-vip-"
	kubeVIPLoadBalancerIPsAnnotation    = "kube-vip.io/loadbalancerIPs"
	kubeVIPServiceInterfaceAnnotation   = "kube-vip.io/serviceInterface"
	networkStatusAnnotation             = nadv1.NetworkStatusAnnot
	longhornEndpointNetworkForRWXVolume = "endpoint-network-for-rwx-volume"
	endpointSliceManagedBy              = ShareManagerVIPControllerName
	nfsPortName                         = "nfs"
	nfsPort                             = 2049
)

// ShareManagerVIPHandler exposes the Share Manager of every RWX volume through a stable
// VIP from the vipRange of the rwx-network setting: a selectorless LoadBalancer Service
// announced by kube-vip, and an EndpointSlice that follows the Share Manager pod's
// address on the RWX network.
type ShareManagerVIPHandler struct {
	settingCache       ctlharvesterv1.SettingCache
	lhSettingCache     ctllhv1.SettingCache
	volumes            ctllhv1.VolumeController
	volumeCache        ctllhv1.VolumeCache
	services           ctlcorev1.ServiceClient
	serviceCache       ctlcorev1.ServiceCache
	endpointSlices     ctldiscoveryv1.EndpointSliceClient
	endpointSliceCache ctldiscoveryv1.EndpointSliceCache
	podCache           ctlcorev1.PodCache
	hncCache           ctlnetworkv1.HostNetworkConfigCache
	recorder           record.EventRecorder

	// allocateLock serializes VIP allocation, whose ledger is the set of VIP Services.
	allocateLock sync.Mutex
}

func registerShareManagerVIP(ctx context.Context, management *config.Management) {
	settingController := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	lhSettings := management.LonghornFactory.Longhorn().V1beta2().Setting()
	volumes := management.LonghornFactory.Longhorn().V1beta2().Volume()
	services := management.CoreFactory.Core().V1().Service()
	endpointSlices := management.DiscoveryFactory.Discovery().V1().EndpointSlice()
	pods := management.CoreFactory.Core().V1().Pod()
	hncs := management.HarvesterNetworkFactory.Network().V1beta1().HostNetworkConfig()

	h := &ShareManagerVIPHandler{
		settingCache:       settingController.Cache(),
		lhSettingCache:     lhSettings.Cache(),
		volumes:            volumes,
		volumeCache:        volumes.Cache(),
		services:           services,
		serviceCache:       services.Cache(),
		endpointSlices:     endpointSlices,
		endpointSliceCache: endpointSlices.Cache(),
		podCache:           pods.Cache(),
		hncCache:           hncs.Cache(),
		recorder:           management.NewRecorder(ShareManagerVIPControllerName, "", ""),
	}

	volumes.OnChange(ctx, ShareManagerVIPControllerName, h.OnVolumeChange)
	settingController.OnChange(ctx, ShareManagerVIPControllerName, func(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
		if setting != nil && setting.Name == settings.RWXNetworkSettingName {
			h.enqueueAll()
		}
		return setting, nil
	})
	lhSettings.OnChange(ctx, ShareManagerVIPControllerName, func(_ string, setting *lhv1beta2.Setting) (*lhv1beta2.Setting, error) {
		if setting != nil && setting.Name == longhornEndpointNetworkForRWXVolume {
			h.enqueueAll()
		}
		return setting, nil
	})
	hncs.OnChange(ctx, ShareManagerVIPControllerName, func(_ string, hnc *networkv1.HostNetworkConfig) (*networkv1.HostNetworkConfig, error) {
		if hnc == nil || hnc.Labels[util.RWXNetworkManagedLabel] == "true" {
			h.enqueueAll()
		}
		return hnc, nil
	})
	pods.OnChange(ctx, ShareManagerVIPControllerName, func(_ string, pod *corev1.Pod) (*corev1.Pod, error) {
		if pod != nil && pod.Namespace == util.LonghornSystemNamespaceName &&
			pod.Labels[lhtypes.GetLonghornLabelComponentKey()] == lhtypes.LonghornLabelShareManager {
			h.enqueue(pod.Labels[lhtypes.GetLonghornLabelKey(lhtypes.LonghornLabelShareManager)])
		}
		return pod, nil
	})
	services.OnChange(ctx, ShareManagerVIPControllerName, func(_ string, svc *corev1.Service) (*corev1.Service, error) {
		if svc != nil && svc.Namespace == util.LonghornSystemNamespaceName && svc.Labels[util.RWXVolServiceLabel] != "" {
			h.enqueue(svc.Labels[util.RWXVolServiceLabel])
		}
		return svc, nil
	})
	services.OnRemove(ctx, ShareManagerVIPControllerName, func(_ string, svc *corev1.Service) (*corev1.Service, error) {
		if svc == nil || svc.Namespace != util.LonghornSystemNamespaceName {
			return svc, nil
		}
		if svc.Labels[util.RWXVolServiceLabel] != "" {
			// A released VIP may unblock volumes waiting on an exhausted range.
			h.enqueueAll()
		} else if volumeName, ok := strings.CutPrefix(svc.Name, vipServicePrefix); ok {
			// A removed foreign Service may unblock the volume whose VIP Service name it took.
			h.enqueue(volumeName)
		}
		return svc, nil
	})
	endpointSlices.OnChange(ctx, ShareManagerVIPControllerName, func(_ string, eps *discoveryv1.EndpointSlice) (*discoveryv1.EndpointSlice, error) {
		if eps != nil && eps.Namespace == util.LonghornSystemNamespaceName && eps.Labels[util.RWXVolServiceLabel] != "" {
			h.enqueue(eps.Labels[util.RWXVolServiceLabel])
		}
		return eps, nil
	})
}

func (h *ShareManagerVIPHandler) enqueue(volumeName string) {
	if volumeName != "" {
		h.volumes.Enqueue(util.LonghornSystemNamespaceName, volumeName)
	}
}

func (h *ShareManagerVIPHandler) enqueueAll() {
	volumes, err := h.volumeCache.List(util.LonghornSystemNamespaceName, labels.Everything())
	if err != nil {
		logrus.WithError(err).Error("Failed to list Longhorn volumes")
		return
	}
	for _, volume := range volumes {
		if isRWXFilesystemVolume(volume) {
			h.enqueue(volume.Name)
		}
	}
}

func (h *ShareManagerVIPHandler) OnVolumeChange(_ string, volume *lhv1beta2.Volume) (*lhv1beta2.Volume, error) {
	if volume == nil || volume.DeletionTimestamp != nil || volume.Namespace != util.LonghornSystemNamespaceName || !isRWXFilesystemVolume(volume) {
		// The VIP Service is owned by the volume and garbage collected with it.
		return volume, nil
	}

	setting, err := h.settingCache.Get(settings.RWXNetworkSettingName)
	if err != nil {
		return volume, fmt.Errorf("failed to get %s setting: %w", settings.RWXNetworkSettingName, err)
	}
	rwxConfig, err := settings.DecodeConfig[settings.RWXNetworkConfig](setting.EffectiveValue())
	if err != nil {
		return volume, err
	}

	svc, err := h.serviceCache.Get(util.LonghornSystemNamespaceName, vipServiceName(volume.Name))
	if apierrors.IsNotFound(err) {
		svc = nil
	} else if err != nil {
		return volume, err
	}
	if svc != nil && svc.Labels[util.RWXVolServiceLabel] != volume.Name {
		// The Service watch requeues the volume once the foreign one is removed.
		h.recorder.Eventf(volume, corev1.EventTypeWarning, ReasonVIPServiceConflict,
			"Service %s/%s exists but is not managed by Harvester, the Share Manager VIP cannot be set up", svc.Namespace, svc.Name)
		return volume, nil
	}

	if rwxConfig == nil || rwxConfig.HostIPRange == "" || rwxConfig.VIPRange == "" {
		if svc != nil && svc.DeletionTimestamp == nil {
			// The EndpointSlice is owned by the Service and garbage collected with it.
			if err := h.services.Delete(svc.Namespace, svc.Name, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return volume, err
			}
		}
		return volume, nil
	}
	if svc != nil && svc.DeletionTimestamp != nil {
		// The Service watch requeues once the deletion completes.
		return volume, nil
	}

	svc, err = h.syncService(volume, svc, rwxConfig)
	if err != nil || svc == nil {
		return volume, err
	}
	return volume, h.syncEndpointSlice(volume, svc)
}

// syncService reconciles the VIP Service of a volume. It returns nil when the volume
// cannot get a VIP yet.
func (h *ShareManagerVIPHandler) syncService(volume *lhv1beta2.Volume, svc *corev1.Service, rwxConfig *settings.RWXNetworkConfig) (*corev1.Service, error) {
	vipRange := rwxConfig.VIPRange
	inUse := svc != nil && inRange(serviceVIP(svc), vipRange)

	// kube-vip announces a VIP only if its interface exists when the Service is
	// created or updated, and does not retry by itself, so VIPs are only handed out
	// once the host network is ready. Existing ones keep serving meanwhile.
	iface, err := h.hostNetworkInterface(rwxConfig)
	if err != nil {
		return nil, err
	}
	if iface == "" {
		if inUse {
			return svc, nil
		}
		return nil, nil
	}
	if inUse {
		return h.applyService(svc, newVIPService(volume, serviceVIP(svc), iface))
	}

	h.allocateLock.Lock()
	defer h.allocateLock.Unlock()

	// List from the API server rather than the cache, so that a VIP handed out by a
	// previous allocation is always seen.
	list, err := h.services.List(util.LonghornSystemNamespaceName, metav1.ListOptions{LabelSelector: util.RWXVolServiceLabel})
	if err != nil {
		return nil, err
	}
	var used []string
	for _, s := range list.Items {
		if s.Labels[util.RWXVolServiceLabel] != volume.Name {
			used = append(used, serviceVIP(&s))
		}
	}

	vip, err := networkutil.AllocateVIP(vipRange, used)
	if errors.Is(err, networkutil.ErrRangeExhausted) {
		h.recorder.Eventf(volume, corev1.EventTypeWarning, ReasonVIPRangeExhausted,
			"no address left in vipRange %s of the %s setting for the Share Manager VIP", vipRange, settings.RWXNetworkSettingName)
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return h.applyService(svc, newVIPService(volume, vip, iface))
}

func (h *ShareManagerVIPHandler) applyService(current, desired *corev1.Service) (*corev1.Service, error) {
	if current == nil {
		return h.services.Create(desired)
	}
	if serviceUpToDate(current, desired) {
		return current, nil
	}

	svcCopy := current.DeepCopy()
	svcCopy.Labels = mergeMaps(svcCopy.Labels, desired.Labels)
	svcCopy.Annotations = mergeMaps(svcCopy.Annotations, desired.Annotations)
	svcCopy.OwnerReferences = desired.OwnerReferences
	svcCopy.Spec.Type = desired.Spec.Type
	svcCopy.Spec.LoadBalancerIP = desired.Spec.LoadBalancerIP
	svcCopy.Spec.AllocateLoadBalancerNodePorts = desired.Spec.AllocateLoadBalancerNodePorts
	svcCopy.Spec.Selector = nil
	svcCopy.Spec.Ports = desired.Spec.Ports
	return h.services.Update(svcCopy)
}

func (h *ShareManagerVIPHandler) syncEndpointSlice(volume *lhv1beta2.Volume, svc *corev1.Service) error {
	network, err := h.rwxEndpointNetwork()
	if err != nil {
		return err
	}

	pod, err := h.podCache.Get(util.LonghornSystemNamespaceName, lhtypes.GetShareManagerPodNameFromShareManagerName(volume.Name))
	if apierrors.IsNotFound(err) {
		pod = nil
	} else if err != nil {
		return err
	}

	var addr, nodeName string
	if addr = shareManagerEndpointAddr(pod, network); addr != "" {
		nodeName = pod.Spec.NodeName
	}
	desired := newVIPEndpointSlice(svc, addr, nodeName)

	current, err := h.endpointSliceCache.Get(desired.Namespace, desired.Name)
	if apierrors.IsNotFound(err) {
		_, err = h.endpointSlices.Create(desired)
		return err
	} else if err != nil {
		return err
	}

	if equality.Semantic.DeepEqual(current.Endpoints, desired.Endpoints) &&
		equality.Semantic.DeepEqual(current.Ports, desired.Ports) &&
		equality.Semantic.DeepEqual(current.OwnerReferences, desired.OwnerReferences) &&
		current.AddressType == desired.AddressType &&
		equality.Semantic.DeepEqual(mergeMaps(current.Labels, desired.Labels), current.Labels) {
		return nil
	}
	if current.AddressType != desired.AddressType {
		// The address type of an EndpointSlice is immutable.
		return h.endpointSlices.Delete(current.Namespace, current.Name, &metav1.DeleteOptions{})
	}

	epsCopy := current.DeepCopy()
	epsCopy.Labels = mergeMaps(epsCopy.Labels, desired.Labels)
	epsCopy.OwnerReferences = desired.OwnerReferences
	epsCopy.Endpoints = desired.Endpoints
	epsCopy.Ports = desired.Ports
	_, err = h.endpointSlices.Update(epsCopy)
	return err
}

// hostNetworkInterface returns the host interface on the RWX network that announces the
// VIPs, or an empty string while the managed HostNetworkConfig for the current setting
// is not ready.
func (h *ShareManagerVIPHandler) hostNetworkInterface(rwxConfig *settings.RWXNetworkConfig) (string, error) {
	network, err := h.rwxSourceNetwork(rwxConfig)
	if err != nil || network == nil {
		return "", err
	}

	hnc, err := h.hncCache.Get(HostNetworkConfigName)
	if apierrors.IsNotFound(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	if hnc.Spec.ClusterNetwork != network.ClusterNetwork || hnc.Spec.VlanID != network.Vlan || !hostNetworkConfigReady(hnc) {
		return "", nil
	}
	return networkutils.GetClusterNetworkVlanDevice(network.ClusterNetwork, network.Vlan), nil
}

// rwxSourceNetwork returns the network carrying RWX traffic, which is the storage
// network in share mode.
func (h *ShareManagerVIPHandler) rwxSourceNetwork(rwxConfig *settings.RWXNetworkConfig) (*networkutil.Config, error) {
	if !rwxConfig.ShareStorageNetwork {
		return rwxConfig.Network, nil
	}
	setting, err := h.settingCache.Get(settings.StorageNetworkName)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s setting: %w", settings.StorageNetworkName, err)
	}
	return settings.DecodeConfig[networkutil.Config](setting.EffectiveValue())
}

// hostNetworkConfigReady reports whether every node of a managed HostNetworkConfig has
// its address set up. The network controller only reports readiness per node.
func hostNetworkConfigReady(hnc *networkv1.HostNetworkConfig) bool {
	if hnc.DeletionTimestamp != nil || hnc.Labels[util.RWXNetworkManagedLabel] != "true" || len(hnc.Spec.HostIPs) == 0 {
		return false
	}
	for node := range hnc.Spec.HostIPs {
		status, ok := hnc.Status.NodeStatus[node]
		if !ok || status.ClusterNetwork != hnc.Spec.ClusterNetwork || status.VlanID != hnc.Spec.VlanID {
			return false
		}
		ready := false
		for _, c := range status.Conditions {
			if c.Type == networkv1.Ready {
				ready = c.Status == corev1.ConditionTrue
				break
			}
		}
		if !ready {
			return false
		}
	}
	return true
}

// rwxEndpointNetwork returns the NAD Longhorn attaches the Share Manager pods to, as
// namespace/name.
func (h *ShareManagerVIPHandler) rwxEndpointNetwork() (string, error) {
	setting, err := h.lhSettingCache.Get(util.LonghornSystemNamespaceName, longhornEndpointNetworkForRWXVolume)
	if apierrors.IsNotFound(err) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("failed to get longhorn %s setting: %w", longhornEndpointNetworkForRWXVolume, err)
	}
	return setting.Value, nil
}

// isRWXFilesystemVolume reports whether a Longhorn volume is served by a Share Manager.
// Migratable RWX volumes are block devices for VM live migration.
func isRWXFilesystemVolume(volume *lhv1beta2.Volume) bool {
	return volume.Spec.AccessMode == lhv1beta2.AccessModeReadWriteMany && !volume.Spec.Migratable
}

// shareManagerEndpointAddr returns the address of a ready Share Manager pod on the given
// network, or an empty string.
func shareManagerEndpointAddr(pod *corev1.Pod, network string) string {
	if pod == nil || network == "" || pod.DeletionTimestamp != nil || !podReady(pod) {
		return ""
	}

	var statuses []nadv1.NetworkStatus
	if err := json.Unmarshal([]byte(pod.Annotations[networkStatusAnnotation]), &statuses); err != nil {
		return ""
	}
	for _, status := range statuses {
		if status.Name != network {
			continue
		}
		for _, ip := range status.IPs {
			if addr, err := netip.ParseAddr(ip); err == nil && addr.Is4() {
				return addr.String()
			}
		}
	}
	return ""
}

func podReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func vipServiceName(volumeName string) string {
	return vipServicePrefix + volumeName
}

func serviceVIP(svc *corev1.Service) string {
	return svc.Annotations[kubeVIPLoadBalancerIPsAnnotation]
}

func inRange(ip, cidr string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	prefix, err := netip.ParsePrefix(cidr)
	return err == nil && prefix.Masked().Contains(addr)
}

func newVIPService(volume *lhv1beta2.Volume, vip, iface string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vipServiceName(volume.Name),
			Namespace: util.LonghornSystemNamespaceName,
			Labels:    map[string]string{util.RWXVolServiceLabel: volume.Name},
			Annotations: map[string]string{
				kubeVIPLoadBalancerIPsAnnotation:  vip,
				kubeVIPServiceInterfaceAnnotation: iface,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: lhv1beta2.SchemeGroupVersion.String(),
				Kind:       "Volume",
				Name:       volume.Name,
				UID:        volume.UID,
			}},
		},
		Spec: corev1.ServiceSpec{
			Type:           corev1.ServiceTypeLoadBalancer,
			LoadBalancerIP: vip,
			// The Share Manager is reached through the VIP only.
			AllocateLoadBalancerNodePorts: ptr.To(false),
			Ports: []corev1.ServicePort{{
				Name:       nfsPortName,
				Protocol:   corev1.ProtocolTCP,
				Port:       nfsPort,
				TargetPort: intstr.FromInt32(nfsPort),
			}},
		},
	}
}

func serviceUpToDate(current, desired *corev1.Service) bool {
	if !equality.Semantic.DeepEqual(mergeMaps(current.Labels, desired.Labels), current.Labels) ||
		!equality.Semantic.DeepEqual(mergeMaps(current.Annotations, desired.Annotations), current.Annotations) ||
		!equality.Semantic.DeepEqual(current.OwnerReferences, desired.OwnerReferences) {
		return false
	}
	if current.Spec.Type != desired.Spec.Type ||
		current.Spec.LoadBalancerIP != desired.Spec.LoadBalancerIP ||
		!equality.Semantic.DeepEqual(current.Spec.AllocateLoadBalancerNodePorts, desired.Spec.AllocateLoadBalancerNodePorts) ||
		len(current.Spec.Selector) > 0 ||
		len(current.Spec.Ports) != len(desired.Spec.Ports) {
		return false
	}
	for i, p := range desired.Spec.Ports {
		c := current.Spec.Ports[i]
		if c.Name != p.Name || c.Protocol != p.Protocol || c.Port != p.Port || c.TargetPort != p.TargetPort {
			return false
		}
	}
	return true
}

func newVIPEndpointSlice(svc *corev1.Service, addr, nodeName string) *discoveryv1.EndpointSlice {
	endpoints := []discoveryv1.Endpoint{}
	if addr != "" {
		endpoint := discoveryv1.Endpoint{
			Addresses:  []string{addr},
			Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
		}
		if nodeName != "" {
			endpoint.NodeName = ptr.To(nodeName)
		}
		endpoints = append(endpoints, endpoint)
	}

	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Labels: map[string]string{
				discoveryv1.LabelServiceName: svc.Name,
				discoveryv1.LabelManagedBy:   endpointSliceManagedBy,
				util.RWXVolServiceLabel:      svc.Labels[util.RWXVolServiceLabel],
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       svc.Name,
				UID:        svc.UID,
			}},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints:   endpoints,
		Ports: []discoveryv1.EndpointPort{{
			Name:     ptr.To(nfsPortName),
			Protocol: ptr.To(corev1.ProtocolTCP),
			Port:     ptr.To(int32(nfsPort)),
		}},
	}
}

// mergeMaps returns a copy of dst with every entry of src set on it.
func mergeMaps(dst, src map[string]string) map[string]string {
	merged := make(map[string]string, len(dst)+len(src))
	for k, v := range dst {
		merged[k] = v
	}
	for k, v := range src {
		merged[k] = v
	}
	return merged
}
