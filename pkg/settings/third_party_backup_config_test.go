package settings

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestThirdPartyBackupConfigDefaults(t *testing.T) {
	for _, value := range []string{"", "{}", InitThirdPartyBackupConfig()} {
		config, err := DecodeThirdPartyBackupConfig(value)
		require.NoError(t, err)
		require.Equal(t, defaultThirdPartyBackupConfig(), *config)
	}
}

func TestThirdPartyBackupConfigOverrides(t *testing.T) {
	config, err := DecodeThirdPartyBackupConfig(`{
		"restic":{"noCache":true,"memoryRequest":"512Mi","maxConcurrentJobs":0},
		"kopia":{"memoryLimit":"8Gi","cacheSize":"1Gi","maxConcurrentJobs":2}
	}`)
	require.NoError(t, err)
	require.True(t, config.Restic.NoCache)
	require.Equal(t, "512Mi", config.Restic.MemoryRequest)
	require.Equal(t, "4Gi", config.Restic.MemoryLimit)
	require.Zero(t, config.Restic.MaxConcurrentJobs)
	require.Equal(t, "1Gi", config.Kopia.MemoryRequest)
	require.Equal(t, "8Gi", config.Kopia.MemoryLimit)
	require.Equal(t, "1Gi", config.Kopia.CacheSize)
	require.Equal(t, 2, config.Kopia.MaxConcurrentJobs)
	value, err := json.Marshal(config)
	require.NoError(t, err)
	decoded, err := DecodeThirdPartyBackupConfig(string(value))
	require.NoError(t, err)
	require.Equal(t, config, decoded)
}

func TestThirdPartyBackupConfigValidation(t *testing.T) {
	for _, value := range []string{
		`null`, `[]`, `true`, `{"restic":`, `{} {}`,
		`{"restic":{"memoryRequest":1}}`,
		`{"restic":{"noCache":"true"}}`,
		`{"kopia":{"maxConcurrentJobs":1.5}}`,
		`{"restic":{"memoryRequest":"bad"}}`,
		`{"restic":{"memoryRequest":"-1Gi"}}`,
		`{"kopia":{"memoryLimit":"0"}}`,
		`{"kopia":{"memoryRequest":""}}`,
		`{"restic":{"memoryRequest":"5Gi"}}`,
		`{"kopia":{"ephemeralRequest":"4Gi"}}`,
		`{"restic":{"cacheSize":"0"}}`,
		`{"kopia":{"cacheSize":"4Mi"}}`,
		`{"kopia":{"cacheSize":"3Gi"}}`,
		`{"restic":{"ephemeralLimit":"2Gi"}}`,
		`{"restic":{"maxConcurrentJobs":-1}}`,
	} {
		t.Run(value, func(t *testing.T) {
			_, err := DecodeThirdPartyBackupConfig(value)
			require.Error(t, err)
		})
	}
	// Related values can be changed together without an invalid intermediate setting.
	_, err := DecodeThirdPartyBackupConfig(`{
		"restic":{"memoryRequest":"5Gi","memoryLimit":"8Gi", "cacheSize":"4Gi","ephemeralLimit":"6Gi"}
	}`)
	require.NoError(t, err)
}

func TestBackupResources(t *testing.T) {
	old := ThirdPartyBackupConfigSet.Get()
	t.Cleanup(func() { _ = ThirdPartyBackupConfigSet.Set(old) })
	for _, engine := range []string{"restic", "kopia"} {
		t.Run(engine, func(t *testing.T) {
			require.NoError(t, ThirdPartyBackupConfigSet.Set(""))
			resources, err := BackupResources(engine)
			require.NoError(t, err)
			require.Equal(t, resource.MustParse("1Gi"), resources.Requests[corev1.ResourceMemory])
			require.Equal(t, resource.MustParse("4Gi"), resources.Limits[corev1.ResourceMemory])
			require.Equal(t, resource.MustParse("512Mi"), resources.Requests[corev1.ResourceEphemeralStorage])
			require.Equal(t, resource.MustParse("3Gi"), resources.Limits[corev1.ResourceEphemeralStorage])

			config, err := GetThirdPartyBackupConfig()
			require.NoError(t, err)
			if engine == "restic" {
				config.Restic.MemoryRequest, config.Restic.MemoryLimit = "512Mi", "2Gi"
			} else {
				config.Kopia.MemoryRequest, config.Kopia.MemoryLimit = "512Mi", "2Gi"
			}
			value, err := json.Marshal(config)
			require.NoError(t, err)
			require.NoError(t, ThirdPartyBackupConfigSet.Set(string(value)))
			resources, err = BackupResources(engine)
			require.NoError(t, err)
			require.Equal(t, resource.MustParse("512Mi"), resources.Requests[corev1.ResourceMemory])
			require.Equal(t, resource.MustParse("2Gi"), resources.Limits[corev1.ResourceMemory])

			// Job construction must reject invalid persisted data that bypasses admission.
			require.NoError(t, ThirdPartyBackupConfigSet.Set(`{"`+engine+`":{"memoryLimit":"128Mi"}}`))
			_, err = BackupResources(engine)
			require.Error(t, err)
		})
	}
	_, err := BackupResources("unknown")
	require.Error(t, err)
}
