package settings

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ThirdPartyBackupConfig struct {
	Restic ResticConfig    `json:"restic"`
	Kopia  BackupJobConfig `json:"kopia"`
}

type ResticConfig struct {
	BackupJobConfig
	NoCache bool `json:"noCache"`
}

type BackupJobConfig struct {
	MemoryRequest     string `json:"memoryRequest"`
	MemoryLimit       string `json:"memoryLimit"`
	CacheSize         string `json:"cacheSize"`
	MaxConcurrentJobs int    `json:"maxConcurrentJobs"`
	EphemeralRequest  string `json:"ephemeralRequest"`
	EphemeralLimit    string `json:"ephemeralLimit"`
}

func defaultThirdPartyBackupConfig() ThirdPartyBackupConfig {
	job := BackupJobConfig{
		MemoryRequest:     "1Gi",
		MemoryLimit:       "4Gi",
		CacheSize:         "2Gi",
		MaxConcurrentJobs: 5,
		EphemeralRequest:  "512Mi",
		EphemeralLimit:    "3Gi",
	}
	return ThirdPartyBackupConfig{
		Restic: ResticConfig{BackupJobConfig: job},
		Kopia:  job,
	}
}

func InitThirdPartyBackupConfig() string {
	value, err := json.Marshal(defaultThirdPartyBackupConfig())
	if err != nil {
		logrus.Errorf("failed to init %s: %v", ThirdPartyBackupConfigSettingName, err)
	}
	return string(value)
}

func GetThirdPartyBackupConfig() (*ThirdPartyBackupConfig, error) {
	return DecodeThirdPartyBackupConfig(ThirdPartyBackupConfigSet.Get())
}

// DecodeThirdPartyBackupConfig fills omitted fields with defaults and validates both
// engines. An empty setting resets the entire configuration to its default.
func DecodeThirdPartyBackupConfig(value string) (*ThirdPartyBackupConfig, error) {
	if value == "" {
		value = ThirdPartyBackupConfigSet.GetDefault()
	}
	if !strings.HasPrefix(strings.TrimSpace(value), "{") {
		return nil, fmt.Errorf("%s must be a JSON object", ThirdPartyBackupConfigSettingName)
	}
	config := defaultThirdPartyBackupConfig()
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", ThirdPartyBackupConfigSettingName, err)
	}
	for engine, job := range map[string]BackupJobConfig{
		"restic": config.Restic.BackupJobConfig,
		"kopia":  config.Kopia,
	} {
		if err := job.validate(engine); err != nil {
			return nil, err
		}
	}
	return &config, nil
}

// BackupResources applies to every engine Job, including checks and maintenance.
func BackupResources(engine string) (corev1.ResourceRequirements, error) {
	config, err := GetThirdPartyBackupConfig()
	if err != nil {
		return corev1.ResourceRequirements{}, err
	}
	switch engine {
	case "restic":
		return config.Restic.resources(engine)
	case "kopia":
		return config.Kopia.resources(engine)
	default:
		return corev1.ResourceRequirements{}, fmt.Errorf("unknown backup engine %q", engine)
	}
}

func (c BackupJobConfig) resources(engine string) (corev1.ResourceRequirements, error) {
	result := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}
	for _, pair := range []struct {
		resource             corev1.ResourceName
		request, limit       string
		requestKey, limitKey string
	}{
		{corev1.ResourceMemory, c.MemoryRequest, c.MemoryLimit, "memoryRequest", "memoryLimit"},
		{corev1.ResourceEphemeralStorage, c.EphemeralRequest, c.EphemeralLimit,
			"ephemeralRequest", "ephemeralLimit"},
	} {
		request, err := backupQuantity(engine+"."+pair.requestKey, pair.request)
		if err != nil {
			return result, err
		}
		limit, err := backupQuantity(engine+"."+pair.limitKey, pair.limit)
		if err != nil {
			return result, err
		}
		if request.Cmp(limit) > 0 {
			return result, fmt.Errorf("%s.%s must not exceed %s", engine, pair.requestKey, pair.limitKey)
		}
		result.Requests[pair.resource] = request
		result.Limits[pair.resource] = limit
	}
	return result, nil
}

func (c BackupJobConfig) validate(engine string) error {
	if c.MaxConcurrentJobs < 0 {
		return fmt.Errorf("%s.maxConcurrentJobs must not be negative (0 disables the limit)", engine)
	}
	resources, err := c.resources(engine)
	if err != nil {
		return err
	}
	cache, err := backupQuantity(engine+".cacheSize", c.CacheSize)
	if err != nil {
		return err
	}
	if engine == "kopia" && cache.Cmp(resource.MustParse("8Mi")) < 0 {
		return fmt.Errorf("kopia.cacheSize must be at least 8Mi")
	}
	if cache.Cmp(resources.Limits[corev1.ResourceEphemeralStorage]) >= 0 {
		return fmt.Errorf("%s.cacheSize must be less than ephemeralLimit to leave room for "+
			"container logs and writable layers", engine)
	}
	return nil
}

func backupQuantity(name, value string) (resource.Quantity, error) {
	quantity, err := resource.ParseQuantity(value)
	if err != nil || quantity.Sign() <= 0 {
		return resource.Quantity{}, fmt.Errorf("%s must be a positive resource quantity, got %q", name, value)
	}
	return quantity, nil
}
