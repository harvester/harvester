package setting

import (
	"encoding/json"
	"strings"
	"testing"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorn "github.com/longhorn/longhorn-manager/types"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestHandler_syncLHIMResources(t *testing.T) {
	createHandler := func(clientset *fake.Clientset) *Handler {
		return &Handler{
			longhornSettings:     fakeclients.LonghornSettingClient(clientset.LonghornV1beta2().Settings),
			longhornSettingCache: fakeclients.LonghornSettingCache(clientset.LonghornV1beta2().Settings),
			longhornVolumeCache:  fakeclients.LonghornVolumeCache(clientset.LonghornV1beta2().Volumes),
		}
	}

	t.Run("updates longhorn setting when all volumes are detached", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()

		err := clientset.Tracker().Add(&lhv1beta2.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: util.LonghornSystemNamespaceName,
				Name:      string(longhorn.SettingNameGuaranteedInstanceManagerCPU),
			},
			Value: `{"v1":"12","v2":"12"}`,
		})
		assert.NoError(t, err)
		err = clientset.Tracker().Add(&lhv1beta2.Volume{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: util.LonghornSystemNamespaceName,
				Name:      "vol-detached",
			},
			Status: lhv1beta2.VolumeStatus{
				State: lhv1beta2.VolumeStateDetached,
			},
		})
		assert.NoError(t, err)
	})

	t.Run("rejects update while there are attached volumes", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		handler := createHandler(clientset)

		err := clientset.Tracker().Add(&lhv1beta2.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: util.LonghornSystemNamespaceName,
				Name:      string(longhorn.SettingNameGuaranteedInstanceManagerCPU),
			},
			Value: `{"v1":"12","v2":"12"}`,
		})
		assert.NoError(t, err)
		err = clientset.Tracker().Add(&lhv1beta2.Volume{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: util.LonghornSystemNamespaceName,
				Name:      "vol-attached",
			},
			Status: lhv1beta2.VolumeStatus{
				State: lhv1beta2.VolumeStateAttached,
			},
		})
		assert.NoError(t, err)

		input := &harvesterv1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.LHIMResourcesSettingName},
			Value:      `{"cpu":{"v1":"20","v2":"20"}}`,
		}

		err = handler.syncLHIMResources(input)
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "waiting for all volumes detached"))

		lhSetting, getErr := handler.longhornSettings.Get(util.LonghornSystemNamespaceName, string(longhorn.SettingNameGuaranteedInstanceManagerCPU), metav1.GetOptions{})
		assert.NoError(t, getErr)
		actual := map[string]string{}
		assert.NoError(t, json.Unmarshal([]byte(lhSetting.Value), &actual))
		assert.Equal(t, map[string]string{"v1": "12", "v2": "12"}, actual)
	})
}
