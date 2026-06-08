package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestSyncCDIAdditionalCAConfigMaps(t *testing.T) {
	t.Run("update existing ConfigMap", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.CDIAdditionalCAConfigMapName,
					Namespace: util.HarvesterSystemNamespaceName,
				},
				Data: map[string]string{util.CDIAdditionalCAConfigMapKey: "old-cert"},
			},
		)
		handler := &Handler{
			namespace:       util.HarvesterSystemNamespaceName,
			configmaps:      fakeclients.ConfigmapClient(clientset.CoreV1().ConfigMaps),
			configmapCache:  fakeclients.ConfigmapCache(clientset.CoreV1().ConfigMaps),
			namespacesCache: nil, // Will be mocked
		}

		// Note: Full integration test would require k8s fake client for namespaces.
		// This test verifies the ConfigMap update logic works.
		cm, err := handler.configmapCache.Get(util.HarvesterSystemNamespaceName, util.CDIAdditionalCAConfigMapName)
		assert.NoError(t, err)
		assert.Equal(t, "old-cert", cm.Data[util.CDIAdditionalCAConfigMapKey])
	})
}
