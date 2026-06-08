package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func newAdditionalCAHandler(clientset *fake.Clientset) *Handler {
	return &Handler{
		namespace:      util.HarvesterSystemNamespaceName,
		configmaps:     fakeclients.ConfigmapClient(clientset.CoreV1().ConfigMaps),
		configmapCache: fakeclients.ConfigmapCache(clientset.CoreV1().ConfigMaps),
		cdiClient:      fakeclients.CDIClient(clientset.CdiV1beta1().CDIs),
		cdiCache:       fakeclients.CDICache(clientset.CdiV1beta1().CDIs),
	}
}

func TestSyncCDIAdditionalCAConfigMap(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	handler := newAdditionalCAHandler(clientset)
	setting := &harvesterv1.Setting{Value: "cert-v1"}

	err := handler.syncCDIAdditionalCAConfigMap(setting)
	assert.NoError(t, err)

	cm, err := handler.configmaps.Get(util.HarvesterSystemNamespaceName, util.CDIAdditionalCAConfigMapName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{cdiAdditionalCAConfigMapKey: "cert-v1"}, cm.Data)

	configMapUpdateCount := 0
	clientset.PrependReactor("update", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		configMapUpdateCount++
		return false, nil, nil
	})

	err = handler.syncCDIAdditionalCAConfigMap(setting)
	assert.NoError(t, err)
	assert.Equal(t, 0, configMapUpdateCount)

	setting.Value = "cert-v2"
	err = handler.syncCDIAdditionalCAConfigMap(setting)
	assert.NoError(t, err)
	assert.Equal(t, 1, configMapUpdateCount)

	cm, err = handler.configmaps.Get(util.HarvesterSystemNamespaceName, util.CDIAdditionalCAConfigMapName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{cdiAdditionalCAConfigMapKey: "cert-v2"}, cm.Data)
}

func TestSyncCDITrustedCAProxy(t *testing.T) {
	t.Run("set trusted CA proxy", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(&cdiv1.CDI{ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName}})
		handler := newAdditionalCAHandler(clientset)

		err := handler.syncCDITrustedCAProxy(&harvesterv1.Setting{Value: "cert"})
		assert.NoError(t, err)

		updatedCDI, err := handler.cdiCache.Get(util.CDIObjectName)
		assert.NoError(t, err)
		if assert.NotNil(t, updatedCDI.Spec.Config) && assert.NotNil(t, updatedCDI.Spec.Config.ImportProxy) && assert.NotNil(t, updatedCDI.Spec.Config.ImportProxy.TrustedCAProxy) {
			assert.Equal(t, util.CDIAdditionalCAConfigMapName, *updatedCDI.Spec.Config.ImportProxy.TrustedCAProxy)
		}
	})

	t.Run("clear trusted CA proxy and keep other proxy fields", func(t *testing.T) {
		httpProxy := "http://proxy.local:3128"
		trusted := util.CDIAdditionalCAConfigMapName
		clientset := fake.NewSimpleClientset(&cdiv1.CDI{
			ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName},
			Spec: cdiv1.CDISpec{Config: &cdiv1.CDIConfigSpec{ImportProxy: &cdiv1.ImportProxy{
				HTTPProxy:      &httpProxy,
				TrustedCAProxy: &trusted,
			}}},
		})
		handler := newAdditionalCAHandler(clientset)

		err := handler.syncCDITrustedCAProxy(&harvesterv1.Setting{Value: ""})
		assert.NoError(t, err)

		updatedCDI, err := handler.cdiCache.Get(util.CDIObjectName)
		assert.NoError(t, err)
		if assert.NotNil(t, updatedCDI.Spec.Config) && assert.NotNil(t, updatedCDI.Spec.Config.ImportProxy) {
			assert.Nil(t, updatedCDI.Spec.Config.ImportProxy.TrustedCAProxy)
			if assert.NotNil(t, updatedCDI.Spec.Config.ImportProxy.HTTPProxy) {
				assert.Equal(t, httpProxy, *updatedCDI.Spec.Config.ImportProxy.HTTPProxy)
			}
		}
	})

	t.Run("CDI not found", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		handler := newAdditionalCAHandler(clientset)
		err := handler.syncCDITrustedCAProxy(&harvesterv1.Setting{Value: "cert"})
		assert.NoError(t, err)
	})

	t.Run("skip update when already configured", func(t *testing.T) {
		trusted := util.CDIAdditionalCAConfigMapName
		clientset := fake.NewSimpleClientset(&cdiv1.CDI{
			ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName},
			Spec: cdiv1.CDISpec{Config: &cdiv1.CDIConfigSpec{ImportProxy: &cdiv1.ImportProxy{
				TrustedCAProxy: &trusted,
			}}},
		})
		handler := newAdditionalCAHandler(clientset)

		cdiUpdateCount := 0
		clientset.PrependReactor("update", "cdis", func(k8stesting.Action) (bool, runtime.Object, error) {
			cdiUpdateCount++
			return false, nil, nil
		})

		err := handler.syncCDITrustedCAProxy(&harvesterv1.Setting{Value: "cert"})
		assert.NoError(t, err)
		assert.Equal(t, 0, cdiUpdateCount)
	})
}
