package settings

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"github.com/sirupsen/logrus"
)

type TargetType string

const (
	S3BackupType  TargetType = "s3"
	NFSBackupType TargetType = "nfs"
)

type BackupTarget struct {
	Type                     TargetType `json:"type"`
	Endpoint                 string     `json:"endpoint"`
	AccessKeyID              string     `json:"accessKeyId"`
	SecretAccessKey          string     `json:"secretAccessKey"`
	BucketName               string     `json:"bucketName"`
	BucketRegion             string     `json:"bucketRegion"`
	Cert                     string     `json:"cert"`
	VirtualHostedStyle       bool       `json:"virtualHostedStyle"`
	RefreshIntervalInSeconds int64      `json:"refreshIntervalInSeconds"`
}

type VMForceResetPolicy struct {
	Enable bool `json:"enable"`
	// Period means how many seconds to wait for a node get back.
	Period int64 `json:"period"`
}

func InitBackupTargetToString() string {
	target := &BackupTarget{}
	targetStr, err := json.Marshal(target)
	if err != nil {
		logrus.Errorf("failed to init %s, error: %s", BackupTargetSettingName, err.Error())
	}
	return string(targetStr)
}

func DecodeBackupTarget(value string) (*BackupTarget, error) {
	target := &BackupTarget{}

	if value != "" {
		if err := json.Unmarshal([]byte(value), target); err != nil {
			return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, value)
		}
	}

	return target, nil
}

func (target *BackupTarget) IsDefaultBackupTarget() bool {
	if target == nil || target.Type != "" {
		return false
	}

	defaultTarget := &BackupTarget{}
	return reflect.DeepEqual(target, defaultTarget)
}

func InitVMForceResetPolicy() string {
	policy := &VMForceResetPolicy{
		Enable: true,
		Period: 5 * 60, // 5 minutes
	}
	policyStr, err := json.Marshal(policy)
	if err != nil {
		logrus.Errorf("failed to init %s, error: %s", VMForceResetPolicySettingName, err.Error())
	}
	return string(policyStr)
}

func DecodeVMForceResetPolicy(value string) (*VMForceResetPolicy, error) {
	policy := &VMForceResetPolicy{}
	if err := json.Unmarshal([]byte(value), policy); err != nil {
		return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, value)
	}

	if policy.Period <= 0 {
		return nil, fmt.Errorf("period value should be greater than 0, value: %d", policy.Period)
	}

	return policy, nil
}

type Overcommit struct {
	CPU     int `json:"cpu"`
	Memory  int `json:"memory"`
	Storage int `json:"storage"`
}

type SSLCertificate struct {
	CA                string `json:"ca"`
	PublicCertificate string `json:"publicCertificate"`
	PrivateKey        string `json:"privateKey"`
}

type SSLParameter struct {
	Protocols string `json:"protocols"`
	Ciphers   string `json:"ciphers"`
}

type CSIDriverInfo struct {
	VolumeSnapshotClassName       string `json:"volumeSnapshotClassName"`
	BackupVolumeSnapshotClassName string `json:"backupVolumeSnapshotClassName"`
}

type AutoRotateRKE2Certs struct {
	Enable          bool `json:"enable"`
	ExpiringInHours int  `json:"expiringInHours"`
}

func InitAutoRotateRKE2Certs() string {
	autoRotateRKE2Certs := &AutoRotateRKE2Certs{
		Enable:          false,
		ExpiringInHours: 240, // 7 days
	}
	autoRotateRKE2CertsStr, err := json.Marshal(autoRotateRKE2Certs)
	if err != nil {
		logrus.WithField("name", AutoRotateRKE2CertsSettingName).WithError(err).Error("failed to init setting")
	}
	return string(autoRotateRKE2CertsStr)
}

func GetCSIDriverInfo(provisioner string) (*CSIDriverInfo, error) {
	csiDriverConfig := make(map[string]*CSIDriverInfo)
	if err := json.Unmarshal([]byte(CSIDriverConfig.Get()), &csiDriverConfig); err != nil {
		return nil, err
	}
	csiDriverInfo, ok := csiDriverConfig[provisioner]
	if !ok {
		return nil, fmt.Errorf("can not find csi driver info for %s", provisioner)
	}
	return csiDriverInfo, nil
}

type StrategyType string

const (
	// Do no preloading
	SkipType StrategyType = "skip"

	// Preloading one node at a time
	SequentialType StrategyType = "sequential"

	// Preloading multiple nodes starts at the same time
	ParallelType StrategyType = "parallel"
)

type PreloadStrategy struct {
	Type StrategyType `json:"type,omitempty"`

	// Concurrency only takes effect when ParallelType is specified. Default to
	// 0, which means "full scale." Any value higher than the number of the
	// cluster nodes will be treated as 0; values lower than 0 will be rejected
	// by the validator.
	Concurrency int `json:"concurrency,omitempty"`
}

type ImagePreloadOption struct {
	// PreloadStrategy tweaks the way images are preloaded.
	Strategy PreloadStrategy `json:"strategy,omitempty"`
}

type UpgradeConfig struct {
	// Options for the Image Preload phase of Harvester Upgrade
	PreloadOption ImagePreloadOption `json:"imagePreloadOption,omitempty"`
	// set true to restore vm to the pre-upgrade state, this option only works under single node.
	RestoreVM bool `json:"restoreVM,omitempty"`
}

func DecodeConfig[T any](value string) (*T, error) {
	target := new(T)

	if value != "" {
		if err := json.Unmarshal([]byte(value), target); err != nil {
			return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, value)
		}
	}

	return target, nil
}

const (
	AdditionalGuestMemoryOverheadRatioMinValue = 1.0
	AdditionalGuestMemoryOverheadRatioMaxValue = 10.0 // when ratio is 10.0, the overhead is about 2.5 Gi, should be enough for most cases

	AdditionalGuestMemoryOverheadRatioDefault = "1.5" // After kubevirt computes the overhead, it will further multiple with this factor
)

type AdditionalGuestMemoryOverheadRatioConfig struct {
	value string  `json:"value"`
	ratio float64 `json:"ratio"` // converted from configured string
}

func NewAdditionalGuestMemoryOverheadRatioConfig(value string) (*AdditionalGuestMemoryOverheadRatioConfig, error) {
	agmorc := AdditionalGuestMemoryOverheadRatioConfig{
		value: value,
	}
	if agmorc.value == "" {
		return &agmorc, nil // the ratio is 0.0
	}

	ratio, err := strconv.ParseFloat(agmorc.value, 64)
	if err != nil {
		return nil, fmt.Errorf("value %v can't be converted to float64, error:%w", agmorc.value, err)
	}
	if ratio < AdditionalGuestMemoryOverheadRatioMinValue && ratio != 0 {
		return nil, fmt.Errorf("value %v can't be less than %v", agmorc.value, AdditionalGuestMemoryOverheadRatioMinValue)
	}
	if ratio > AdditionalGuestMemoryOverheadRatioMaxValue {
		return nil, fmt.Errorf("value %v can't be greater than %v", agmorc.value, AdditionalGuestMemoryOverheadRatioMaxValue)
	}
	agmorc.ratio = ratio
	return &agmorc, nil
}

func (agmorc *AdditionalGuestMemoryOverheadRatioConfig) Ratio() float64 {
	return agmorc.ratio
}

func (agmorc *AdditionalGuestMemoryOverheadRatioConfig) Value() string {
	return agmorc.value
}

// this setting is cleared when:
// value field "0" or "0.0"
// value & default fields are both empty
func (agmorc *AdditionalGuestMemoryOverheadRatioConfig) IsEmpty() bool {
	return agmorc.value == "" || agmorc.ratio == 0.0
}

func ValidateAdditionalGuestMemoryOverheadRatioHelper(value string) error {
	_, err := NewAdditionalGuestMemoryOverheadRatioConfig(value)
	return err
}
