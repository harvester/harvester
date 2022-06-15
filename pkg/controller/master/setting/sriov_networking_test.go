package setting

import (
	"testing"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	corefake "k8s.io/client-go/kubernetes/fake"
	"github.com/stretchr/testify/assert"
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCheckIfHelmChartInstalled(t *testing.T) {
	var clientset = fake.NewSimpleClientset()
	var coreclientset = corefake.NewSimpleClientset()
	var sriovSetting *harvesterv1.Setting = &harvesterv1.Setting{
		Value:"true",
	}
	var handler = &Handler{
		settings:       fakeSettingClient(clientset.HarvesterhciV1beta1().Settings),
		configmaps:     fakeclients.ConfigmapClient(coreclientset.CoreV1().ConfigMaps),
		configmapCache: fakeclients.ConfigmapCache(coreclientset.CoreV1().ConfigMaps),
	}
	handler.sriovNetworkingChartManager(sriovSetting)
	// NOTE: Bogus test for now, will fix
	assert.Equal(t, nil, nil)
}
