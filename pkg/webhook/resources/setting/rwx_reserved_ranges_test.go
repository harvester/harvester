package setting

import (
	"fmt"
	"testing"

	whereaboutsv1alpha1 "github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	networkutil "github.com/harvester/harvester/pkg/util/network"
)

func newRWXRangesValidator(objects ...runtime.Object) *settingValidator {
	objects = append(objects,
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-3"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "witness", Labels: map[string]string{util.HarvesterWitnessNodeLabelKey: "true"}}},
	)
	clientset := fake.NewSimpleClientset(objects...)
	return &settingValidator{
		settingCache: fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		nodeCache:    fakeclients.NodeCache(clientset.CoreV1().Nodes),
		lhNodeCache:  fakeclients.LonghornNodeCache(clientset.LonghornV1beta2().Nodes),
		hncCache:     fakeclients.HostNetworkConfigCache(clientset.NetworkV1beta1().HostNetworkConfigs),
		ipPoolCache:  fakeclients.WhereaboutsIPPoolCache(clientset.WhereaboutsV1alpha1().IPPools),
	}
}

func storageNetworkSetting(value string) *v1beta1.Setting {
	return &v1beta1.Setting{ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName}, Value: value}
}

func hostNetworkConfig(name, clusterNetwork string, vlan uint16, managed bool) *networkv1.HostNetworkConfig {
	hnc := &networkv1.HostNetworkConfig{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       networkv1.HostNetworkConfigSpec{ClusterNetwork: clusterNetwork, VlanID: vlan, Mode: "static"},
	}
	if managed {
		hnc.Labels = map[string]string{util.RWXNetworkManagedLabel: "true"}
	}
	return hnc
}

func ipPool(cidr string, offsets ...int) *whereaboutsv1alpha1.IPPool {
	name, _ := networkutil.WhereaboutsIPPoolName(cidr)
	allocations := map[string]whereaboutsv1alpha1.IPAllocation{}
	for _, o := range offsets {
		allocations[fmt.Sprint(o)] = whereaboutsv1alpha1.IPAllocation{PodRef: "longhorn-system/pod"}
	}
	return &whereaboutsv1alpha1.IPPool{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       whereaboutsv1alpha1.IPPoolSpec{Range: cidr, Allocations: allocations},
	}
}

func Test_validateRWXNetworkReservedRanges(t *testing.T) {
	dedicated := func(rangeCIDR, exclude, host, vip string) string {
		value := fmt.Sprintf(`{"share-storage-network":false,"network":{"vlan":2017,"clusterNetwork":"mgmt","range":%q`, rangeCIDR)
		if exclude != "" {
			value += fmt.Sprintf(`,"exclude":[%q]`, exclude)
		}
		value += "}"
		if host != "" {
			value += fmt.Sprintf(`,"hostIPRange":%q`, host)
		}
		if vip != "" {
			value += fmt.Sprintf(`,"vipRange":%q`, vip)
		}
		return value + "}"
	}
	share := func(host, vip string) string {
		return fmt.Sprintf(`{"share-storage-network":true,"hostIPRange":%q,"vipRange":%q}`, host, vip)
	}
	storageNetwork := storageNetworkSetting(`{"vlan":2017,"clusterNetwork":"mgmt","range":"172.16.0.0/24"}`)

	tests := []struct {
		name        string
		value       string
		objects     []runtime.Object
		errContains string
	}{
		{
			name:  "dedicated network without reserved ranges",
			value: dedicated("10.10.0.0/24", "", "", ""),
		},
		{
			name:  "dedicated network with valid reserved ranges",
			value: dedicated("10.10.0.0/24", "10.10.0.1/32", "10.10.0.224/28", "10.10.0.192/27"),
		},
		{
			name:        "ranges must be set together",
			value:       dedicated("10.10.0.0/24", "", "10.10.0.224/28", ""),
			errContains: "must be set together",
		},
		{
			name:        "range must be a subnet CIDR",
			value:       dedicated("10.10.0.0/24", "", "10.10.0.241/28", "10.10.0.192/27"),
			errContains: "should be subnet CIDR",
		},
		{
			name:        "range must be within the network range",
			value:       dedicated("10.10.0.0/24", "", "10.10.1.224/28", "10.10.0.192/27"),
			errContains: "is not within range",
		},
		{
			name:        "range must not include the broadcast address",
			value:       dedicated("10.10.0.0/24", "", "10.10.0.192/28", "10.10.0.224/27"),
			errContains: "broadcast address",
		},
		{
			name:        "range must not overlap user excludes",
			value:       dedicated("10.10.0.0/24", "10.10.0.230/32", "10.10.0.224/28", "10.10.0.192/27"),
			errContains: "overlaps exclude entry",
		},
		{
			name:        "ranges must not overlap each other",
			value:       dedicated("10.10.0.0/24", "", "10.10.0.192/28", "10.10.0.192/27"),
			errContains: "overlaps vipRange",
		},
		{
			name:        "host IP range must fit all non-witness nodes",
			value:       dedicated("10.10.0.0/24", "", "10.10.0.224/31", "10.10.0.192/27"),
			errContains: "fewer than the 3 non-witness nodes",
		},
		{
			name:        "remaining range must still fit RWX workloads",
			value:       dedicated("10.10.0.0/26", "", "10.10.0.32/28", "10.10.0.16/28"),
			errContains: "allocatable IP address range",
		},
		{
			name:        "user HostNetworkConfig on the same VLAN conflicts",
			value:       dedicated("10.10.0.0/24", "", "10.10.0.224/28", "10.10.0.192/27"),
			objects:     []runtime.Object{hostNetworkConfig("user", "mgmt", 2017, false)},
			errContains: "HostNetworkConfig user already configures",
		},
		{
			name:    "managed HostNetworkConfig on the same VLAN is fine",
			value:   dedicated("10.10.0.0/24", "", "10.10.0.224/28", "10.10.0.192/27"),
			objects: []runtime.Object{hostNetworkConfig("rwx", "mgmt", 2017, true), hostNetworkConfig("other", "mgmt", 2018, false)},
		},
		{
			name:        "range must not contain live Whereabouts allocations",
			value:       dedicated("10.10.0.0/24", "", "10.10.0.224/28", "10.10.0.192/27"),
			objects:     []runtime.Object{ipPool("10.10.0.0/24", 1, 200)},
			errContains: "10.10.0.200 in 10.10.0.192/27 is already allocated",
		},
		{
			name:    "share mode validates against the storage network",
			value:   share("172.16.0.224/28", "172.16.0.192/27"),
			objects: []runtime.Object{storageNetwork},
		},
		{
			name:        "share mode requires the storage network",
			value:       share("172.16.0.224/28", "172.16.0.192/27"),
			errContains: "require a dedicated network or share-storage-network",
		},
		{
			name:        "share mode range must fit the storage network",
			value:       share("172.16.1.224/28", "172.16.0.192/27"),
			objects:     []runtime.Object{storageNetwork},
			errContains: "is not within range 172.16.0.0/24",
		},
		{
			name:        "share mode remaining range must fit storage and RWX workloads",
			value:       share("172.16.0.32/28", "172.16.0.16/28"),
			objects:     []runtime.Object{storageNetworkSetting(`{"vlan":2017,"clusterNetwork":"mgmt","range":"172.16.0.0/26"}`)},
			errContains: "allocatable IP address range",
		},
		{
			name:        "untagged network is rejected",
			value:       share("172.16.0.224/28", "172.16.0.192/27"),
			objects:     []runtime.Object{storageNetworkSetting(`{"clusterNetwork":"cn1","range":"172.16.0.0/24"}`)},
			errContains: "require a tagged VLAN",
		},
		{
			name:        "host interface name must fit the Linux limit",
			value:       share("172.16.0.224/28", "172.16.0.192/27"),
			objects:     []runtime.Object{storageNetworkSetting(`{"vlan":2017,"clusterNetwork":"averylongcn","range":"172.16.0.0/24"}`)},
			errContains: "averylongcn-br.2017",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := newRWXRangesValidator(tc.objects...)
			err := v.validateRWXNetworkHelper(&v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      tc.value,
			})
			if tc.errContains == "" {
				assert.NoError(t, err)
				return
			}
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), tc.errContains)
			}
		})
	}
}

func Test_checkStorageNetworkKeepsRWXReservedRanges(t *testing.T) {
	rwxSetting := func(value string) *v1beta1.Setting {
		return &v1beta1.Setting{ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName}, Value: value}
	}
	sharedWithRanges := rwxSetting(`{"share-storage-network":true,"hostIPRange":"172.16.0.224/28","vipRange":"172.16.0.192/27"}`)

	tests := []struct {
		name        string
		rwx         *v1beta1.Setting
		config      *networkutil.Config
		errContains string
	}{
		{
			name:   "new storage network still fits the ranges",
			rwx:    sharedWithRanges,
			config: &networkutil.Config{ClusterNetwork: "mgmt", Vlan: 2018, Range: "172.16.0.0/24"},
		},
		{
			name:        "new storage network no longer contains the ranges",
			rwx:         sharedWithRanges,
			config:      &networkutil.Config{ClusterNetwork: "mgmt", Vlan: 2017, Range: "172.16.1.0/24"},
			errContains: "rwx-network shares this network",
		},
		{
			name:   "ranges are ignored when rwx-network does not share the storage network",
			rwx:    rwxSetting(`{"share-storage-network":false,"network":{"vlan":2017,"clusterNetwork":"mgmt","range":"10.10.0.0/24"},"hostIPRange":"10.10.0.224/28","vipRange":"10.10.0.192/27"}`),
			config: &networkutil.Config{ClusterNetwork: "mgmt", Vlan: 2017, Range: "172.16.1.0/24"},
		},
		{
			name:   "storage network cleared",
			rwx:    sharedWithRanges,
			config: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := newRWXRangesValidator(tc.rwx)
			err := v.checkStorageNetworkKeepsRWXReservedRanges(tc.config)
			if tc.errContains == "" {
				assert.NoError(t, err)
				return
			}
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), tc.errContains)
			}
		})
	}
}
