package setting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestHandler_syncPersistentReservation(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name     string
		current  *kubevirtv1.PersistentReservationConfiguration
		value    string
		expected *kubevirtv1.PersistentReservationConfiguration
	}{
		{
			name:     "default value keeps the field unset",
			current:  nil,
			value:    "",
			expected: nil,
		},
		{
			name:     "enable sets the field",
			current:  nil,
			value:    "true",
			expected: &kubevirtv1.PersistentReservationConfiguration{Enabled: &enabled},
		},
		{
			name:     "enable overrides an explicit false",
			current:  &kubevirtv1.PersistentReservationConfiguration{Enabled: &disabled},
			value:    "true",
			expected: &kubevirtv1.PersistentReservationConfiguration{Enabled: &enabled},
		},
		{
			name:     "already enabled is unchanged",
			current:  &kubevirtv1.PersistentReservationConfiguration{Enabled: &enabled},
			value:    "true",
			expected: &kubevirtv1.PersistentReservationConfiguration{Enabled: &enabled},
		},
		{
			name:     "disable removes the field",
			current:  &kubevirtv1.PersistentReservationConfiguration{Enabled: &enabled},
			value:    "false",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			kubevirt := &kubevirtv1.KubeVirt{
				ObjectMeta: metav1.ObjectMeta{Namespace: util.HarvesterSystemNamespaceName, Name: util.KubeVirtObjectName},
				Spec: kubevirtv1.KubeVirtSpec{
					Configuration: kubevirtv1.KubeVirtConfiguration{PersistentReservationConfiguration: tc.current},
				},
			}
			assert.NoError(t, clientset.Tracker().Add(kubevirt))

			handler := &Handler{
				kubeVirtConfig:      fakeclients.KubeVirtClient(clientset.KubevirtV1().KubeVirts),
				kubeVirtConfigCache: fakeclients.KubeVirtCache(clientset.KubevirtV1().KubeVirts),
			}
			setting := &harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.PersistentReservationEnabledSettingName},
				Default:    "false",
				Value:      tc.value,
			}

			assert.NoError(t, handler.syncPersistentReservation(setting))

			got, err := clientset.KubevirtV1().KubeVirts(util.HarvesterSystemNamespaceName).Get(context.TODO(), util.KubeVirtObjectName, metav1.GetOptions{})
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, got.Spec.Configuration.PersistentReservationConfiguration)
		})
	}

	t.Run("invalid value returns an error", func(t *testing.T) {
		handler := &Handler{}
		setting := &harvesterv1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.PersistentReservationEnabledSettingName},
			Value:      "yes please",
		}
		assert.Error(t, handler.syncPersistentReservation(setting))
	})
}
