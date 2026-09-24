package rwxnetwork

import (
	"context"
	"fmt"
	"testing"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const testNetworkStatus = `[{
  "name": "k8s-pod-network",
  "interface": "eth0",
  "ips": ["10.52.0.30"],
  "default": true
},{
  "name": "harvester-system/rwx-network-abcde",
  "interface": "lhnet2",
  "ips": ["172.16.0.21"]
}]`

func newShareManagerPod(ready bool, networkStatus string) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "share-manager-pvc-1",
			Namespace: util.LonghornSystemNamespaceName,
		},
		Spec: corev1.PodSpec{NodeName: "node-a"},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			PodIP:      "10.52.0.30",
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
		},
	}
	if networkStatus != "" {
		pod.Annotations = map[string]string{networkStatusAnnotation: networkStatus}
	}
	return pod
}

func TestShareManagerEndpointAddr(t *testing.T) {
	const network = "harvester-system/rwx-network-abcde"

	deleting := newShareManagerPod(true, testNetworkStatus)
	deleting.DeletionTimestamp = &metav1.Time{}

	tests := []struct {
		name    string
		pod     *corev1.Pod
		network string
		want    string
	}{
		{
			name:    "returns the RWX network address, not the pod IP",
			pod:     newShareManagerPod(true, testNetworkStatus),
			network: network,
			want:    "172.16.0.21",
		},
		{
			name:    "no pod",
			network: network,
		},
		{
			name:    "pod not ready",
			pod:     newShareManagerPod(false, testNetworkStatus),
			network: network,
		},
		{
			name:    "pod terminating",
			pod:     deleting,
			network: network,
		},
		{
			name:    "no network status",
			pod:     newShareManagerPod(true, ""),
			network: network,
		},
		{
			name:    "malformed network status",
			pod:     newShareManagerPod(true, "{"),
			network: network,
		},
		{
			name:    "pod not attached to the RWX network",
			pod:     newShareManagerPod(true, testNetworkStatus),
			network: "harvester-system/storagenetwork-xyz",
		},
		{
			name: "RWX network not configured in Longhorn",
			pod:  newShareManagerPod(true, testNetworkStatus),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shareManagerEndpointAddr(tt.pod, tt.network))
		})
	}
}

func TestNewVIPService(t *testing.T) {
	volume := &lhv1beta2.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-1",
			Namespace: util.LonghornSystemNamespaceName,
			UID:       types.UID("volume-uid"),
		},
	}

	svc := newVIPService(volume, "172.16.0.250")

	assert.Equal(t, "rwx-vip-pvc-1", svc.Name)
	assert.Equal(t, util.LonghornSystemNamespaceName, svc.Namespace)
	assert.Equal(t, map[string]string{util.RWXVolServiceLabel: "pvc-1"}, svc.Labels)
	assert.Equal(t, map[string]string{
		kubeVIPLoadBalancerIPsAnnotation:  "172.16.0.250",
		kubeVIPServiceInterfaceAnnotation: "auto",
	}, svc.Annotations)
	require.Len(t, svc.OwnerReferences, 1)
	assert.Equal(t, metav1.OwnerReference{
		APIVersion: "longhorn.io/v1beta2",
		Kind:       "Volume",
		Name:       "pvc-1",
		UID:        types.UID("volume-uid"),
	}, svc.OwnerReferences[0])

	assert.Equal(t, corev1.ServiceTypeLoadBalancer, svc.Spec.Type)
	assert.Equal(t, "172.16.0.250", svc.Spec.LoadBalancerIP)
	assert.Nil(t, svc.Spec.Selector)
	assert.Equal(t, ptr.To(false), svc.Spec.AllocateLoadBalancerNodePorts)
	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, "nfs", svc.Spec.Ports[0].Name)
	assert.Equal(t, int32(2049), svc.Spec.Ports[0].Port)
	assert.Equal(t, int32(2049), svc.Spec.Ports[0].TargetPort.IntVal)
	assert.Equal(t, corev1.ProtocolTCP, svc.Spec.Ports[0].Protocol)

	assert.Equal(t, "172.16.0.250", serviceVIP(svc))
}

func TestNewVIPEndpointSlice(t *testing.T) {
	svc := newVIPService(&lhv1beta2.Volume{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"}}, "172.16.0.250")
	svc.UID = types.UID("service-uid")

	t.Run("points at the Share Manager address", func(t *testing.T) {
		eps := newVIPEndpointSlice(svc, "172.16.0.21", "node-a")

		assert.Equal(t, "rwx-vip-pvc-1", eps.Name)
		assert.Equal(t, util.LonghornSystemNamespaceName, eps.Namespace)
		assert.Equal(t, "rwx-vip-pvc-1", eps.Labels[discoveryv1.LabelServiceName])
		assert.Equal(t, endpointSliceManagedBy, eps.Labels[discoveryv1.LabelManagedBy])
		assert.Equal(t, "pvc-1", eps.Labels[util.RWXVolServiceLabel])
		assert.Equal(t, discoveryv1.AddressTypeIPv4, eps.AddressType)
		require.Len(t, eps.OwnerReferences, 1)
		assert.Equal(t, types.UID("service-uid"), eps.OwnerReferences[0].UID)
		assert.Equal(t, "Service", eps.OwnerReferences[0].Kind)

		require.Len(t, eps.Ports, 1)
		assert.Equal(t, "nfs", *eps.Ports[0].Name)
		assert.Equal(t, int32(2049), *eps.Ports[0].Port)
		assert.Equal(t, corev1.ProtocolTCP, *eps.Ports[0].Protocol)

		require.Len(t, eps.Endpoints, 1)
		assert.Equal(t, []string{"172.16.0.21"}, eps.Endpoints[0].Addresses)
		assert.Equal(t, ptr.To(true), eps.Endpoints[0].Conditions.Ready)
		assert.Equal(t, ptr.To("node-a"), eps.Endpoints[0].NodeName)
	})

	t.Run("has no endpoint without a Share Manager address", func(t *testing.T) {
		eps := newVIPEndpointSlice(svc, "", "")
		assert.Empty(t, eps.Endpoints)
		assert.Len(t, eps.Ports, 1)
	})
}

const (
	testRWXNAD      = "harvester-system/rwx-network-abcde"
	testRWXSetting  = `{"share-storage-network":false,"network":{"vlan":2017,"clusterNetwork":"rwx","range":"172.16.0.0/24"},"hostIPRange":"172.16.0.240/29","vipRange":"172.16.0.248/30"}`
	testRWXDisabled = `{"share-storage-network":false}`
)

type vipTestEnv struct {
	clientset *fake.Clientset
	handler   *ShareManagerVIPHandler
	recorder  *record.FakeRecorder
}

func newVIPTestEnv(rwxSetting string, hncReady bool, objs ...runtime.Object) *vipTestEnv {
	readyStatus := corev1.ConditionFalse
	if hncReady {
		readyStatus = corev1.ConditionTrue
	}
	objs = append(objs,
		&harvesterv1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
			Value:      rwxSetting,
		},
		&lhv1beta2.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: longhornEndpointNetworkForRWXVolume, Namespace: util.LonghornSystemNamespaceName},
			Value:      testRWXNAD,
		},
		newTestHostNetworkConfig(readyStatus),
	)

	clientset := fake.NewSimpleClientset(objs...)
	recorder := record.NewFakeRecorder(10)
	return &vipTestEnv{
		clientset: clientset,
		recorder:  recorder,
		handler: &ShareManagerVIPHandler{
			settingCache:       fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
			lhSettingCache:     fakeclients.LonghornSettingCache(clientset.LonghornV1beta2().Settings),
			services:           fakeclients.ServiceClient(clientset.CoreV1().Services),
			serviceCache:       fakeclients.ServiceCache(clientset.CoreV1().Services),
			endpointSlices:     fakeclients.EndpointSliceClient(clientset.DiscoveryV1().EndpointSlices),
			endpointSliceCache: fakeclients.EndpointSliceCache(clientset.DiscoveryV1().EndpointSlices),
			podCache:           fakeclients.PodCache(clientset.CoreV1().Pods),
			hncCache:           fakeclients.HostNetworkConfigCache(clientset.NetworkV1beta1().HostNetworkConfigs),
			recorder:           recorder,
		},
	}
}

func (e *vipTestEnv) reconcile(t *testing.T, volume *lhv1beta2.Volume) {
	t.Helper()
	_, err := e.handler.OnVolumeChange(volume.Namespace+"/"+volume.Name, volume)
	require.NoError(t, err)
}

func (e *vipTestEnv) service(t *testing.T, volume string) *corev1.Service {
	t.Helper()
	svc, err := e.clientset.CoreV1().Services(util.LonghornSystemNamespaceName).Get(context.TODO(), vipServiceName(volume), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)
	return svc
}

func (e *vipTestEnv) endpointSlice(t *testing.T, volume string) *discoveryv1.EndpointSlice {
	t.Helper()
	eps, err := e.clientset.DiscoveryV1().EndpointSlices(util.LonghornSystemNamespaceName).Get(context.TODO(), vipServiceName(volume), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)
	return eps
}

func (e *vipTestEnv) setPod(t *testing.T, pod *corev1.Pod) {
	t.Helper()
	pods := e.clientset.CoreV1().Pods(pod.Namespace)
	_ = pods.Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
	_, err := pods.Create(context.TODO(), pod, metav1.CreateOptions{})
	require.NoError(t, err)
}

func newTestHostNetworkConfig(readyStatus corev1.ConditionStatus) *networkv1.HostNetworkConfig {
	return &networkv1.HostNetworkConfig{
		ObjectMeta: metav1.ObjectMeta{Name: HostNetworkConfigName, Labels: map[string]string{util.RWXNetworkManagedLabel: "true"}},
		Spec: networkv1.HostNetworkConfigSpec{
			ClusterNetwork: "rwx",
			VlanID:         2017,
			Mode:           hostNetworkConfigModeStatic,
			HostIPs:        map[string]networkv1.IPAddr{"node-1": "172.16.0.240/24", "node-2": "172.16.0.241/24"},
		},
		Status: networkv1.HostNetworkConfigStatus{
			NodeStatus: map[string]networkv1.HostNetworkConfigNodeStatus{
				"node-1": {ClusterNetwork: "rwx", VlanID: 2017, Conditions: []networkv1.Condition{{Type: networkv1.Ready, Status: corev1.ConditionTrue}}},
				"node-2": {ClusterNetwork: "rwx", VlanID: 2017, Conditions: []networkv1.Condition{{Type: networkv1.Ready, Status: readyStatus}}},
			},
		},
	}
}

func TestHostNetworkConfigReady(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*networkv1.HostNetworkConfig)
		want   bool
	}{
		{name: "every node ready", want: true},
		{name: "a node not ready", mutate: func(h *networkv1.HostNetworkConfig) {
			h.Status.NodeStatus["node-2"].Conditions[0].Status = corev1.ConditionFalse
		}},
		{name: "a node without status", mutate: func(h *networkv1.HostNetworkConfig) {
			delete(h.Status.NodeStatus, "node-2")
		}},
		{name: "a node status for another VLAN", mutate: func(h *networkv1.HostNetworkConfig) {
			s := h.Status.NodeStatus["node-2"]
			s.VlanID = 2018
			h.Status.NodeStatus["node-2"] = s
		}},
		{name: "no host IPs", mutate: func(h *networkv1.HostNetworkConfig) { h.Spec.HostIPs = nil }},
		{name: "being deleted", mutate: func(h *networkv1.HostNetworkConfig) { h.DeletionTimestamp = &metav1.Time{} }},
		{name: "not managed", mutate: func(h *networkv1.HostNetworkConfig) { h.Labels = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hnc := newTestHostNetworkConfig(corev1.ConditionTrue)
			if tt.mutate != nil {
				tt.mutate(hnc)
			}
			assert.Equal(t, tt.want, hostNetworkConfigReady(hnc))
		})
	}
}

func newRWXVolume(name string) *lhv1beta2.Volume {
	return &lhv1beta2.Volume{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: util.LonghornSystemNamespaceName, UID: types.UID(name + "-uid")},
		Spec:       lhv1beta2.VolumeSpec{AccessMode: lhv1beta2.AccessModeReadWriteMany},
	}
}

func rwxNetworkStatus(ip string) string {
	return `[{"name":"k8s-pod-network","ips":["10.52.0.30"],"default":true},{"name":"` + testRWXNAD + `","ips":["` + ip + `"]}]`
}

func endpointAddrs(eps *discoveryv1.EndpointSlice) []string {
	var addrs []string
	for _, e := range eps.Endpoints {
		addrs = append(addrs, e.Addresses...)
	}
	return addrs
}

func TestShareManagerVIPKeepsVIPAcrossPodRecreation(t *testing.T) {
	volume := newRWXVolume("pvc-1")
	env := newVIPTestEnv(testRWXSetting, true, newShareManagerPod(true, rwxNetworkStatus("172.16.0.21")))

	env.reconcile(t, volume)
	svc := env.service(t, "pvc-1")
	require.NotNil(t, svc)
	assert.Equal(t, "172.16.0.248", serviceVIP(svc))
	eps := env.endpointSlice(t, "pvc-1")
	require.NotNil(t, eps)
	assert.Equal(t, []string{"172.16.0.21"}, endpointAddrs(eps))

	// The Share Manager pod is being replaced: no endpoint, same VIP.
	env.setPod(t, newShareManagerPod(false, ""))
	env.reconcile(t, volume)
	assert.Equal(t, "172.16.0.248", serviceVIP(env.service(t, "pvc-1")))
	assert.Empty(t, env.endpointSlice(t, "pvc-1").Endpoints)

	// The new pod comes up with a different address on the RWX network.
	env.setPod(t, newShareManagerPod(true, rwxNetworkStatus("172.16.0.37")))
	env.reconcile(t, volume)
	assert.Equal(t, "172.16.0.248", serviceVIP(env.service(t, "pvc-1")))
	assert.Equal(t, []string{"172.16.0.37"}, endpointAddrs(env.endpointSlice(t, "pvc-1")))
}

func TestShareManagerVIPAllocation(t *testing.T) {
	t.Run("skips VIPs held by other volumes", func(t *testing.T) {
		other := newVIPService(newRWXVolume("pvc-0"), "172.16.0.248")
		env := newVIPTestEnv(testRWXSetting, true, other)

		env.reconcile(t, newRWXVolume("pvc-1"))
		assert.Equal(t, "172.16.0.249", serviceVIP(env.service(t, "pvc-1")))
	})

	t.Run("reallocates a VIP left outside a changed vipRange", func(t *testing.T) {
		stale := newVIPService(newRWXVolume("pvc-1"), "172.16.0.200")
		env := newVIPTestEnv(testRWXSetting, true, stale)

		env.reconcile(t, newRWXVolume("pvc-1"))
		svc := env.service(t, "pvc-1")
		assert.Equal(t, "172.16.0.248", serviceVIP(svc))
		assert.Equal(t, "172.16.0.248", svc.Spec.LoadBalancerIP)
	})

	t.Run("reports an exhausted vipRange", func(t *testing.T) {
		vips := []string{"172.16.0.248", "172.16.0.249", "172.16.0.250", "172.16.0.251"}
		objs := make([]runtime.Object, 0, len(vips))
		for i, vip := range vips {
			objs = append(objs, newVIPService(newRWXVolume(fmt.Sprintf("pvc-%d", i+10)), vip))
		}
		env := newVIPTestEnv(testRWXSetting, true, objs...)

		env.reconcile(t, newRWXVolume("pvc-1"))
		assert.Nil(t, env.service(t, "pvc-1"))
		require.Len(t, env.recorder.Events, 1)
		assert.Contains(t, <-env.recorder.Events, ReasonVIPRangeExhausted)
	})

	t.Run("waits for the host network before handing out a VIP", func(t *testing.T) {
		env := newVIPTestEnv(testRWXSetting, false)

		env.reconcile(t, newRWXVolume("pvc-1"))
		assert.Nil(t, env.service(t, "pvc-1"))
	})

	t.Run("keeps serving an existing VIP while the host network is not ready", func(t *testing.T) {
		existing := newVIPService(newRWXVolume("pvc-1"), "172.16.0.249")
		env := newVIPTestEnv(testRWXSetting, false, existing)

		env.reconcile(t, newRWXVolume("pvc-1"))
		assert.Equal(t, "172.16.0.249", serviceVIP(env.service(t, "pvc-1")))
		assert.NotNil(t, env.endpointSlice(t, "pvc-1"))
	})
}

func TestShareManagerVIPIgnoresOtherVolumes(t *testing.T) {
	migratable := newRWXVolume("pvc-migratable")
	migratable.Spec.Migratable = true
	rwo := newRWXVolume("pvc-rwo")
	rwo.Spec.AccessMode = lhv1beta2.AccessModeReadWriteOnce

	env := newVIPTestEnv(testRWXSetting, true)
	for _, volume := range []*lhv1beta2.Volume{migratable, rwo} {
		env.reconcile(t, volume)
		assert.Nil(t, env.service(t, volume.Name))
	}
}

func TestShareManagerVIPTeardown(t *testing.T) {
	existing := newVIPService(newRWXVolume("pvc-1"), "172.16.0.248")
	env := newVIPTestEnv(testRWXDisabled, true, existing)

	env.reconcile(t, newRWXVolume("pvc-1"))
	assert.Nil(t, env.service(t, "pvc-1"))
}
