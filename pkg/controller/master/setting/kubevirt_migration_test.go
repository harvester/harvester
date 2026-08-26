package setting

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func newKubeVirtMigrationSetting(value string, annotations map[string]string) *harvesterv1.Setting {
	return &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name:        settings.KubeVirtMigrationSettingName,
			Annotations: annotations,
		},
		Default: settings.KubeVirtMigration.Default,
		Value:   value,
	}
}

func newKubeVirt(migrationConfiguration *kubevirtv1.MigrationConfiguration) *kubevirtv1.KubeVirt {
	return &kubevirtv1.KubeVirt{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: util.HarvesterSystemNamespaceName,
			Name:      util.KubeVirtObjectName,
		},
		Spec: kubevirtv1.KubeVirtSpec{
			Configuration: kubevirtv1.KubeVirtConfiguration{
				MigrationConfiguration: migrationConfiguration,
			},
		},
	}
}

// defaultMigrationConfiguration returns the setting default decoded into a struct, which is
// what the KubeVirt object is expected to end up with once the setting is applied.
func defaultMigrationConfiguration(t *testing.T) *kubevirtv1.MigrationConfiguration {
	t.Helper()
	migrationConfiguration := &kubevirtv1.MigrationConfiguration{}
	require.NoError(t, json.Unmarshal([]byte(settings.KubeVirtMigration.Default), migrationConfiguration))
	return migrationConfiguration
}

func TestSyncKubeVirtMigration(t *testing.T) {
	var (
		trueValue    = true
		fortyGiB     = resource.MustParse("40Gi")
		drainTaint   = "acme.io/drain"
		migrationNet = "default/migration"
	)

	type input struct {
		setting  *harvesterv1.Setting
		kubevirt *kubevirtv1.KubeVirt
	}
	type output struct {
		// settingValue is the expected setting value after the sync. An empty string means
		// the setting is expected to be left untouched.
		settingValue *kubevirtv1.MigrationConfiguration
		// kubevirtConfig is the expected KubeVirt migration configuration after the sync.
		kubevirtConfig *kubevirtv1.MigrationConfiguration
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			// The regression this guards: on upgrade the setting is created empty while the
			// KubeVirt object already carries a hand-made configuration. The setting must
			// adopt it instead of reporting the defaults.
			name: "unconfigured setting adopts the existing KubeVirt configuration",
			given: input{
				setting: newKubeVirtMigrationSetting("", nil),
				kubevirt: newKubeVirt(&kubevirtv1.MigrationConfiguration{
					AllowAutoConverge: &trueValue,
				}),
			},
			expected: output{
				settingValue: func() *kubevirtv1.MigrationConfiguration {
					c := defaultMigrationConfiguration(t)
					c.AllowAutoConverge = &trueValue
					return c
				}(),
				kubevirtConfig: &kubevirtv1.MigrationConfiguration{
					AllowAutoConverge: &trueValue,
				},
			},
		},
		{
			name: "adoption drops the fields owned elsewhere",
			given: input{
				setting: newKubeVirtMigrationSetting("", nil),
				kubevirt: newKubeVirt(&kubevirtv1.MigrationConfiguration{
					AllowAutoConverge:     &trueValue,
					BandwidthPerMigration: &fortyGiB,
					NodeDrainTaintKey:     &drainTaint,
					Network:               &migrationNet,
				}),
			},
			expected: output{
				settingValue: func() *kubevirtv1.MigrationConfiguration {
					c := defaultMigrationConfiguration(t)
					c.AllowAutoConverge = &trueValue
					c.BandwidthPerMigration = &fortyGiB
					return c
				}(),
				kubevirtConfig: &kubevirtv1.MigrationConfiguration{
					AllowAutoConverge:     &trueValue,
					BandwidthPerMigration: &fortyGiB,
					NodeDrainTaintKey:     &drainTaint,
					Network:               &migrationNet,
				},
			},
		},
		{
			// A pristine cluster runs on the KubeVirt defaults, which the setting default
			// already mirrors, so there is nothing to adopt and nothing to write.
			name: "unconfigured setting leaves an unconfigured KubeVirt object alone",
			given: input{
				setting:  newKubeVirtMigrationSetting("", nil),
				kubevirt: newKubeVirt(nil),
			},
			expected: output{
				settingValue:   nil,
				kubevirtConfig: nil,
			},
		},
		{
			name: "configured setting is applied to the KubeVirt object",
			given: input{
				setting:  newKubeVirtMigrationSetting(`{"parallelMigrationsPerCluster":4,"allowAutoConverge":true}`, nil),
				kubevirt: newKubeVirt(nil),
			},
			expected: output{
				settingValue: nil,
				kubevirtConfig: func() *kubevirtv1.MigrationConfiguration {
					four := uint32(4)
					return &kubevirtv1.MigrationConfiguration{
						ParallelMigrationsPerCluster: &four,
						AllowAutoConverge:            &trueValue,
					}
				}(),
			},
		},
		{
			name: "configured setting overrides the existing KubeVirt configuration",
			given: input{
				setting: newKubeVirtMigrationSetting(`{"allowAutoConverge":true}`, nil),
				kubevirt: newKubeVirt(&kubevirtv1.MigrationConfiguration{
					AllowPostCopy: &trueValue,
					Network:       &migrationNet,
				}),
			},
			expected: output{
				settingValue: nil,
				kubevirtConfig: &kubevirtv1.MigrationConfiguration{
					AllowAutoConverge: &trueValue,
					Network:           &migrationNet,
				},
			},
		},
		{
			// Clearing the value of a setting that was configured before resets the KubeVirt
			// object to the default, it does not re-adopt what is on the object.
			name: "cleared value of a previously synced setting resets to the default",
			given: input{
				setting: newKubeVirtMigrationSetting("", map[string]string{util.AnnotationHash: "stale"}),
				kubevirt: newKubeVirt(&kubevirtv1.MigrationConfiguration{
					AllowAutoConverge: &trueValue,
				}),
			},
			expected: output{
				settingValue:   nil,
				kubevirtConfig: defaultMigrationConfiguration(t),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			require.NoError(t, clientset.Tracker().Add(tc.given.setting))
			require.NoError(t, clientset.Tracker().Add(tc.given.kubevirt))

			handler := &Handler{
				settings:            fakeclients.HarvesterSettingClient(clientset.HarvesterhciV1beta1().Settings),
				kubeVirtConfig:      fakeclients.KubeVirtClient(clientset.KubevirtV1().KubeVirts),
				kubeVirtConfigCache: fakeclients.KubeVirtCache(clientset.KubevirtV1().KubeVirts),
			}

			require.NoError(t, handler.syncKubeVirtMigration(tc.given.setting))

			setting, err := clientset.HarvesterhciV1beta1().Settings().Get(context.TODO(), tc.given.setting.Name, metav1.GetOptions{})
			require.NoError(t, err)
			if tc.expected.settingValue == nil {
				assert.Equal(t, tc.given.setting.Value, setting.Value, "setting value should not change")
			} else {
				actual := &kubevirtv1.MigrationConfiguration{}
				require.NoError(t, json.Unmarshal([]byte(setting.Value), actual))
				assert.Equal(t, tc.expected.settingValue, actual)
			}

			kubevirt, err := clientset.KubevirtV1().KubeVirts(util.HarvesterSystemNamespaceName).Get(context.TODO(), util.KubeVirtObjectName, metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, tc.expected.kubevirtConfig, kubevirt.Spec.Configuration.MigrationConfiguration)
		})
	}
}
