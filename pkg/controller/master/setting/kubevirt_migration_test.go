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

func newKubeVirtMigrationSetting(value string) *harvesterv1.Setting {
	return &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.KubeVirtMigrationSettingName,
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

// defaultMigrationConfiguration returns the setting default decoded into a struct, which is what
// the KubeVirt object is expected to hold once a setting without a value is applied.
func defaultMigrationConfiguration(t *testing.T) *kubevirtv1.MigrationConfiguration {
	t.Helper()
	migrationConfiguration := &kubevirtv1.MigrationConfiguration{}
	require.NoError(t, json.Unmarshal([]byte(settings.KubeVirtMigration.Default), migrationConfiguration))
	return migrationConfiguration
}

func TestSyncKubeVirtMigration(t *testing.T) {
	var (
		trueValue    = true
		four         = uint32(4)
		fortyGiB     = resource.MustParse("40Gi")
		drainTaint   = "acme.io/drain"
		migrationNet = "default/migration"
	)

	type input struct {
		setting  *harvesterv1.Setting
		kubevirt *kubevirtv1.KubeVirt
	}

	var testCases = []struct {
		name     string
		given    input
		expected *kubevirtv1.MigrationConfiguration
	}{
		{
			name: "the value is applied to the KubeVirt object",
			given: input{
				setting:  newKubeVirtMigrationSetting(`{"parallelMigrationsPerCluster":4,"allowAutoConverge":true}`),
				kubevirt: newKubeVirt(nil),
			},
			expected: &kubevirtv1.MigrationConfiguration{
				ParallelMigrationsPerCluster: &four,
				AllowAutoConverge:            &trueValue,
			},
		},
		{
			// The whole migrations block is replaced, so allowPostCopy is dropped. Only the
			// network is carried over, it belongs to the vm-migration-network setting.
			name: "the value replaces the existing configuration but keeps the network",
			given: input{
				setting: newKubeVirtMigrationSetting(`{"allowAutoConverge":true}`),
				kubevirt: newKubeVirt(&kubevirtv1.MigrationConfiguration{
					AllowPostCopy: &trueValue,
					Network:       &migrationNet,
				}),
			},
			expected: &kubevirtv1.MigrationConfiguration{
				AllowAutoConverge: &trueValue,
				Network:           &migrationNet,
			},
		},
		{
			// nodeDrainTaintKey is cleared so KubeVirt falls back to kubevirt.io/drain, which
			// the upgrade scripts depend on.
			name: "nodeDrainTaintKey is cleared",
			given: input{
				setting: newKubeVirtMigrationSetting(`{"allowAutoConverge":true}`),
				kubevirt: newKubeVirt(&kubevirtv1.MigrationConfiguration{
					NodeDrainTaintKey: &drainTaint,
				}),
			},
			expected: &kubevirtv1.MigrationConfiguration{
				AllowAutoConverge: &trueValue,
			},
		},
		{
			name: "a quantity value is applied",
			given: input{
				setting:  newKubeVirtMigrationSetting(`{"bandwidthPerMigration":"40Gi"}`),
				kubevirt: newKubeVirt(nil),
			},
			expected: &kubevirtv1.MigrationConfiguration{
				BandwidthPerMigration: &fortyGiB,
			},
		},
		{
			// Clearing the value resets the KubeVirt object to the setting default.
			name: "an empty value falls back to the default",
			given: input{
				setting: newKubeVirtMigrationSetting(""),
				kubevirt: newKubeVirt(&kubevirtv1.MigrationConfiguration{
					AllowAutoConverge: &trueValue,
				}),
			},
			expected: defaultMigrationConfiguration(t),
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

			kubevirt, err := clientset.KubevirtV1().KubeVirts(util.HarvesterSystemNamespaceName).
				Get(context.TODO(), util.KubeVirtObjectName, metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, tc.expected, kubevirt.Spec.Configuration.MigrationConfiguration)
		})
	}
}

// TestSyncKubeVirtMigrationNoop covers the guard that keeps the syncer from writing to the
// KubeVirt object on every resync.
func TestSyncKubeVirtMigrationNoop(t *testing.T) {
	setting := newKubeVirtMigrationSetting(settings.KubeVirtMigration.Default)
	kubevirt := newKubeVirt(defaultMigrationConfiguration(t))

	clientset := fake.NewSimpleClientset()
	require.NoError(t, clientset.Tracker().Add(setting))
	require.NoError(t, clientset.Tracker().Add(kubevirt))

	handler := &Handler{
		settings:            fakeclients.HarvesterSettingClient(clientset.HarvesterhciV1beta1().Settings),
		kubeVirtConfig:      fakeclients.KubeVirtClient(clientset.KubevirtV1().KubeVirts),
		kubeVirtConfigCache: fakeclients.KubeVirtCache(clientset.KubevirtV1().KubeVirts),
	}

	clientset.ClearActions()
	require.NoError(t, handler.syncKubeVirtMigration(setting))

	for _, action := range clientset.Actions() {
		assert.NotEqual(t, "update", action.GetVerb(), "the KubeVirt object should not be updated when already in sync")
	}
}
