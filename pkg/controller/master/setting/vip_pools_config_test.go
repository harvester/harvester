package setting

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	corefake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	typeharv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestSyncVipPoolsConfig(t *testing.T) {
	const vipPools = "vip-pools"
	type input struct {
		key     string
		setting *harvesterv1.Setting
	}
	type output struct {
		pools map[string]string
		err   error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "correct ip pools input, should pass",
			given: input{
				key: vipPools,
				setting: &harvesterv1.Setting{
					ObjectMeta: v1.ObjectMeta{
						Name: vipPools,
					},
					Value: `{"default":"172.16.1.0/24","test":"172.16.2.0/24"}`,
				},
			},
			expected: output{
				pools: map[string]string{
					"cidr-default": "172.16.1.0/24",
					"cidr-test":    "172.16.2.0/24",
				},
				err: nil,
			},
		},
		{
			name: "multiple CIDRs per ns pools, should pass",
			given: input{
				key: vipPools,
				setting: &harvesterv1.Setting{
					ObjectMeta: v1.ObjectMeta{
						Name: vipPools,
					},
					Value: `{"default":"172.16.1.0/24,192.168.10.0/24","test":"172.16.2.0/24,192.168.20.0/24"}`,
				},
			},
			expected: output{
				pools: map[string]string{
					"cidr-default": "172.16.1.0/24,192.168.10.0/24",
					"cidr-test":    "172.16.2.0/24,192.168.20.0/24",
				},
				err: nil,
			},
		},
		{
			name: "incorrect ip pools input, should fail",
			given: input{
				setting: &harvesterv1.Setting{
					Value: `{"default": "172.16.1.0/242", "test": "1000.16.2.0/24"}`,
				},
			},
			expected: output{
				pools: nil,
				err:   errors.New("invalid CIDR value"),
			},
		},
	}

	for _, tc := range testCases {

		pools := map[string]string{}
		err := json.Unmarshal([]byte(tc.given.setting.Value), &pools)
		assert.Nil(t, err)

		var actual output
		actual.err = ValidateCIDRs(pools)
		if strings.Contains(tc.name, "fail") {
			assert.Errorf(t, actual.err, "invalid CIDR value")
			continue
		} else {
			assert.NoError(t, actual.err)
		}

		var clientset = fake.NewSimpleClientset()
		if tc.given.setting != nil {
			var err = clientset.Tracker().Add(tc.given.setting)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		// validate syncVipPoolsConfig func
		var coreclientset = corefake.NewSimpleClientset()
		var handler = &Handler{
			settings:       fakeSettingClient(clientset.HarvesterhciV1beta1().Settings),
			configmaps:     fakeclients.ConfigmapClient(coreclientset.CoreV1().ConfigMaps),
			configmapCache: fakeclients.ConfigmapCache(coreclientset.CoreV1().ConfigMaps),
		}
		syncers = map[string]syncerFunc{
			"vip-pools": handler.syncVipPoolsConfig,
		}

		var syncActual output
		_, syncActual.err = handler.settingOnChanged(tc.given.key, tc.given.setting)
		assert.Nil(t, syncActual.err)

		// check if the kube-vip configmap is configured properly
		cnf, err := handler.configmaps.Get(util.KubeSystemNamespace, KubevipConfigmapName, v1.GetOptions{})
		assert.NoError(t, err)
		assert.Equal(t, tc.expected.pools, cnf.Data)
	}
}

type fakeSettingClient func() typeharv1.SettingInterface

func (c fakeSettingClient) Create(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	return c().Create(context.TODO(), setting, metav1.CreateOptions{})
}

func (c fakeSettingClient) Update(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	return c().Update(context.TODO(), setting, metav1.UpdateOptions{})
}

func (c fakeSettingClient) UpdateStatus(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	return c().UpdateStatus(context.TODO(), setting, metav1.UpdateOptions{})
}

func (c fakeSettingClient) Delete(name string, opts *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *opts)
}

func (c fakeSettingClient) Get(name string, opts metav1.GetOptions) (*harvesterv1.Setting, error) {
	return c().Get(context.TODO(), name, opts)
}

func (c fakeSettingClient) List(opts metav1.ListOptions) (*harvesterv1.SettingList, error) {
	return c().List(context.TODO(), opts)
}

func (c fakeSettingClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c fakeSettingClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1.Setting, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}
