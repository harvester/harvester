package settings

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	communityReleaseDownloadURL = "https://releases.rancher.com/harvester"
	primeReleaseDownloadURL     = "https://prime.example.invalid/harvester"
)

func TestSetAllDefaultValueFromEnv(t *testing.T) {
	tests := []struct {
		name            string
		existing        *harvesterv1.Setting
		expectedDefault string
		expectedValue   string
	}{
		{
			name:            "new setting is created with default from env",
			expectedDefault: primeReleaseDownloadURL,
		},
		{
			name: "existing default is replaced",
			existing: &harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.ReleaseDownloadURLSettingName},
				Default:    communityReleaseDownloadURL,
			},
			expectedDefault: primeReleaseDownloadURL,
		},
		{
			name: "existing value is kept",
			existing: &harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.ReleaseDownloadURLSettingName},
				Default:    communityReleaseDownloadURL,
				Value:      "https://mirror.example.invalid/harvester",
			},
			expectedDefault: primeReleaseDownloadURL,
			expectedValue:   "https://mirror.example.invalid/harvester",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(settings.GetEnvDefaultValueKey(settings.ReleaseDownloadURLSettingName), primeReleaseDownloadURL)

			client := fake.NewSimpleClientset()
			if tc.existing != nil {
				client = fake.NewSimpleClientset(tc.existing)
			}
			sp := &settingsProvider{
				context:        context.TODO(),
				settings:       fakeclients.HarvesterSettingClient(client.HarvesterhciV1beta1().Settings),
				settingsLister: fakeclients.HarvesterSettingCache(client.HarvesterhciV1beta1().Settings),
			}

			err := sp.SetAll(map[string]settings.Setting{
				settings.ReleaseDownloadURLSettingName: {
					Name:    settings.ReleaseDownloadURLSettingName,
					Default: communityReleaseDownloadURL,
				},
			})
			require.NoError(t, err)

			obj, err := client.HarvesterhciV1beta1().Settings().Get(context.TODO(), settings.ReleaseDownloadURLSettingName, metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, tc.expectedDefault, obj.Default)
			assert.Equal(t, tc.expectedValue, obj.Value)
		})
	}
}
