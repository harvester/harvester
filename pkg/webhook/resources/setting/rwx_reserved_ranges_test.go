package setting

import (
	"fmt"
	"strings"
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

func Test_checkStorageNetworkNotLockedByRWX(t *testing.T) {
	rwxSetting := func(value string) *v1beta1.Setting {
		return &v1beta1.Setting{ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName}, Value: value}
	}
	const storageNetwork = `{"vlan":2017,"clusterNetwork":"mgmt","range":"172.16.0.0/24"}`

	tests := []struct {
		name        string
		rwx         string
		newValue    string
		errContains string
	}{
		{
			name:        "shared with ranges set",
			rwx:         `{"share-storage-network":true,"hostIPRange":"172.16.0.224/28","vipRange":"172.16.0.192/27"}`,
			newValue:    `{"vlan":2017,"clusterNetwork":"mgmt","range":"172.16.0.0/24","exclude":["172.16.0.128/28"]}`,
			errContains: "remove them from rwx-network first",
		},
		{
			name:        "shared with ranges set, storage network cleared",
			rwx:         `{"share-storage-network":true,"hostIPRange":"172.16.0.224/28","vipRange":"172.16.0.192/27"}`,
			errContains: "remove them from rwx-network first",
		},
		{
			name:     "shared with ranges set, same config reformatted",
			rwx:      `{"share-storage-network":true,"hostIPRange":"172.16.0.224/28","vipRange":"172.16.0.192/27"}`,
			newValue: `{"range":"172.16.0.0/24","clusterNetwork":"mgmt","vlan":2017}`,
		},
		{
			name:     "shared without ranges",
			rwx:      `{"share-storage-network":true}`,
			newValue: `{"vlan":2018,"clusterNetwork":"mgmt","range":"172.16.0.0/24"}`,
		},
		{
			name:     "ranges set on a dedicated network",
			rwx:      `{"share-storage-network":false,"network":{"vlan":2019,"clusterNetwork":"mgmt","range":"10.10.0.0/24"},"hostIPRange":"10.10.0.224/28","vipRange":"10.10.0.192/27"}`,
			newValue: `{"vlan":2018,"clusterNetwork":"mgmt","range":"172.16.0.0/24"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := newRWXRangesValidator(rwxSetting(tc.rwx))
			err := v.checkStorageNetworkNotLockedByRWX(storageNetworkSetting(storageNetwork), storageNetworkSetting(tc.newValue))
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

func Test_checkRWXNetworkNotLocked(t *testing.T) {
	const (
		network  = `"network":{"vlan":2017,"clusterNetwork":"mgmt","range":"172.16.0.0/24"}`
		disabled = `{"share-storage-network":false,` + network + `}`
		enabled  = `{"share-storage-network":false,` + network + `,"hostIPRange":"172.16.0.224/28","vipRange":"172.16.0.192/27"}`
	)
	rwxSetting := func(value string) *v1beta1.Setting {
		return &v1beta1.Setting{ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName}, Value: value}
	}

	tests := []struct {
		name     string
		oldValue string
		newValue string
		locked   bool
	}{
		{name: "enable", oldValue: disabled, newValue: enabled},
		{name: "enable together with a new network", oldValue: `{"share-storage-network":false}`, newValue: enabled},
		{name: "disable", oldValue: enabled, newValue: disabled},
		{name: "reset to default", oldValue: enabled, newValue: ""},
		{name: "change the network while disabled", oldValue: disabled, newValue: strings.Replace(disabled, "2017", "2018", 1)},
		{name: "change a range", oldValue: enabled, newValue: strings.Replace(enabled, "172.16.0.224/28", "172.16.0.160/27", 1), locked: true},
		{name: "change the VLAN", oldValue: enabled, newValue: strings.Replace(enabled, "2017", "2018", 1), locked: true},
		{name: "change the excludes", oldValue: enabled, newValue: strings.Replace(enabled, `"range":"172.16.0.0/24"`, `"range":"172.16.0.0/24","exclude":["172.16.0.10/32"]`, 1), locked: true},
		{name: "switch to share mode", oldValue: enabled, newValue: `{"share-storage-network":true,"hostIPRange":"172.16.0.224/28","vipRange":"172.16.0.192/27"}`, locked: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkRWXNetworkNotLocked(rwxSetting(tc.oldValue), rwxSetting(tc.newValue))
			if !tc.locked {
				assert.NoError(t, err)
				return
			}
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), "remove them first")
			}
		})
	}
}
