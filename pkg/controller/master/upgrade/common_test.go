package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade/repoinfo"
	"github.com/harvester/harvester/pkg/settings"
)

func TestApplyNodeJobShutdownVMOnMigrationFailure(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		expected    string
	}{
		{name: "enabled", annotations: map[string]string{shutdownVMOnMigrationFailureAnnotation: "true"}, expected: "true"},
		{name: "disabled", annotations: map[string]string{shutdownVMOnMigrationFailureAnnotation: "false"}, expected: "false"},
		{name: "no annotation", expected: "false"},
		{name: "invalid annotation", annotations: map[string]string{shutdownVMOnMigrationFailureAnnotation: "yes"}, expected: "false"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			upgrade := &harvesterv1.Upgrade{}
			upgrade.Name = "test-upgrade"
			upgrade.Annotations = tc.annotations
			job := applyNodeJob(upgrade, &repoinfo.RepoInfo{}, "test-node", upgradeJobTypePreDrain)

			found := false
			for _, env := range job.Spec.Template.Spec.Containers[0].Env {
				if env.Name == "HARVESTER_SHUTDOWN_VM_ON_MIGRATION_FAILURE" {
					found = true
					assert.Equal(t, tc.expected, env.Value)
				}
			}
			assert.True(t, found, "env var not found")
		})
	}
}

func TestSaveShutdownVMOnMigrationFailureToUpgradeAnnotation(t *testing.T) {
	defaultValue := settings.UpgradeConfigSet.Get()
	t.Cleanup(func() { _ = settings.UpgradeConfigSet.Set(defaultValue) })

	testCases := []struct {
		name        string
		config      string
		annotations map[string]string
		expected    string
	}{
		{name: "default", config: defaultValue, expected: "false"},
		{name: "enabled", config: `{"shutdownVMOnMigrationFailure": true}`, expected: "true"},
		{name: "disabled", config: `{"shutdownVMOnMigrationFailure": false}`, expected: "false"},
		{name: "missing field", config: `{"restoreVM": true}`, expected: "false"},
		{name: "non-boolean value", config: `{"shutdownVMOnMigrationFailure": "true"}`, expected: "false"},
		{name: "invalid config", config: `not json`, expected: "false"},
		{
			name:        "saved value isn't changed by enabling the setting",
			config:      `{"shutdownVMOnMigrationFailure": true}`,
			annotations: map[string]string{shutdownVMOnMigrationFailureAnnotation: "false"},
			expected:    "false",
		},
		{
			name:        "saved value isn't changed by disabling the setting",
			config:      `{"shutdownVMOnMigrationFailure": false}`,
			annotations: map[string]string{shutdownVMOnMigrationFailureAnnotation: "true"},
			expected:    "true",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, settings.UpgradeConfigSet.Set(tc.config))

			upgrade := &harvesterv1.Upgrade{}
			upgrade.Annotations = tc.annotations
			saveShutdownVMOnMigrationFailureToUpgradeAnnotation(upgrade)

			assert.Equal(t, tc.expected, upgrade.Annotations[shutdownVMOnMigrationFailureAnnotation])
		})
	}
}
