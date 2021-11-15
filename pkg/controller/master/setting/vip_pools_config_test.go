package setting

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
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
