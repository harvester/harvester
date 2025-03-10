package types

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/robfig/cron"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/longhorn/longhorn-manager/meta"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	DefaultSettingYAMLFileName  = "default-setting.yaml"
	DefaultResourceYAMLFileName = "default-resource.yaml"

	ValueEmpty   = "none"
	ValueUnknown = "unknown"

	// From `maximumChainLength` in longhorn-engine/pkg/replica/replica.go
	MaxSnapshotNum = 250

	DefaultMinNumberOfCopies = 3

	DefaultBackupstorePollInterval = 300 * time.Second
)

type SettingType string

const (
	SettingTypeString     = SettingType("string")
	SettingTypeInt        = SettingType("int")
	SettingTypeBool       = SettingType("bool")
	SettingTypeDeprecated = SettingType("deprecated")

	ValueIntRangeMinimum = "minimum"
	ValueIntRangeMaximum = "maximum"
)

type SettingName string

const (
	SettingNameAllowRecurringJobWhileVolumeDetached                     = SettingName("allow-recurring-job-while-volume-detached")
	SettingNameCreateDefaultDiskLabeledNodes                            = SettingName("create-default-disk-labeled-nodes")
	SettingNameDefaultDataPath                                          = SettingName("default-data-path")
	SettingNameDefaultEngineImage                                       = SettingName("default-engine-image")
	SettingNameDefaultInstanceManagerImage                              = SettingName("default-instance-manager-image")
	SettingNameDefaultBackingImageManagerImage                          = SettingName("default-backing-image-manager-image")
	SettingNameSupportBundleManagerImage                                = SettingName("support-bundle-manager-image")
	SettingNameReplicaSoftAntiAffinity                                  = SettingName("replica-soft-anti-affinity")
	SettingNameReplicaAutoBalance                                       = SettingName("replica-auto-balance")
	SettingNameReplicaAutoBalanceDiskPressurePercentage                 = SettingName("replica-auto-balance-disk-pressure-percentage")
	SettingNameStorageOverProvisioningPercentage                        = SettingName("storage-over-provisioning-percentage")
	SettingNameStorageMinimalAvailablePercentage                        = SettingName("storage-minimal-available-percentage")
	SettingNameStorageReservedPercentageForDefaultDisk                  = SettingName("storage-reserved-percentage-for-default-disk")
	SettingNameUpgradeChecker                                           = SettingName("upgrade-checker")
	SettingNameUpgradeResponderURL                                      = SettingName("upgrade-responder-url")
	SettingNameAllowCollectingLonghornUsage                             = SettingName("allow-collecting-longhorn-usage-metrics")
	SettingNameCurrentLonghornVersion                                   = SettingName("current-longhorn-version")
	SettingNameLatestLonghornVersion                                    = SettingName("latest-longhorn-version")
	SettingNameStableLonghornVersions                                   = SettingName("stable-longhorn-versions")
	SettingNameDefaultReplicaCount                                      = SettingName("default-replica-count")
	SettingNameDefaultDataLocality                                      = SettingName("default-data-locality")
	SettingNameDefaultLonghornStaticStorageClass                        = SettingName("default-longhorn-static-storage-class")
	SettingNameTaintToleration                                          = SettingName("taint-toleration")
	SettingNameSystemManagedComponentsNodeSelector                      = SettingName("system-managed-components-node-selector")
	SettingNameCRDAPIVersion                                            = SettingName("crd-api-version")
	SettingNameAutoSalvage                                              = SettingName("auto-salvage")
	SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly              = SettingName("auto-delete-pod-when-volume-detached-unexpectedly")
	SettingNameRegistrySecret                                           = SettingName("registry-secret")
	SettingNameDisableSchedulingOnCordonedNode                          = SettingName("disable-scheduling-on-cordoned-node")
	SettingNameReplicaZoneSoftAntiAffinity                              = SettingName("replica-zone-soft-anti-affinity")
	SettingNameNodeDownPodDeletionPolicy                                = SettingName("node-down-pod-deletion-policy")
	SettingNameNodeDrainPolicy                                          = SettingName("node-drain-policy")
	SettingNameDetachManuallyAttachedVolumesWhenCordoned                = SettingName("detach-manually-attached-volumes-when-cordoned")
	SettingNamePriorityClass                                            = SettingName("priority-class")
	SettingNameDisableRevisionCounter                                   = SettingName("disable-revision-counter")
	SettingNameReplicaReplenishmentWaitInterval                         = SettingName("replica-replenishment-wait-interval")
	SettingNameConcurrentReplicaRebuildPerNodeLimit                     = SettingName("concurrent-replica-rebuild-per-node-limit")
	SettingNameConcurrentBackingImageCopyReplenishPerNodeLimit          = SettingName("concurrent-backing-image-replenish-per-node-limit")
	SettingNameConcurrentBackupRestorePerNodeLimit                      = SettingName("concurrent-volume-backup-restore-per-node-limit")
	SettingNameSystemManagedPodsImagePullPolicy                         = SettingName("system-managed-pods-image-pull-policy")
	SettingNameAllowVolumeCreationWithDegradedAvailability              = SettingName("allow-volume-creation-with-degraded-availability")
	SettingNameAutoCleanupSystemGeneratedSnapshot                       = SettingName("auto-cleanup-system-generated-snapshot")
	SettingNameAutoCleanupRecurringJobBackupSnapshot                    = SettingName("auto-cleanup-recurring-job-backup-snapshot")
	SettingNameConcurrentAutomaticEngineUpgradePerNodeLimit             = SettingName("concurrent-automatic-engine-upgrade-per-node-limit")
	SettingNameBackingImageCleanupWaitInterval                          = SettingName("backing-image-cleanup-wait-interval")
	SettingNameBackingImageRecoveryWaitInterval                         = SettingName("backing-image-recovery-wait-interval")
	SettingNameGuaranteedInstanceManagerCPU                             = SettingName("guaranteed-instance-manager-cpu")
	SettingNameKubernetesClusterAutoscalerEnabled                       = SettingName("kubernetes-cluster-autoscaler-enabled")
	SettingNameOrphanAutoDeletion                                       = SettingName("orphan-auto-deletion")
	SettingNameStorageNetwork                                           = SettingName("storage-network")
	SettingNameStorageNetworkForRWXVolumeEnabled                        = SettingName("storage-network-for-rwx-volume-enabled")
	SettingNameFailedBackupTTL                                          = SettingName("failed-backup-ttl")
	SettingNameRecurringSuccessfulJobsHistoryLimit                      = SettingName("recurring-successful-jobs-history-limit")
	SettingNameRecurringFailedJobsHistoryLimit                          = SettingName("recurring-failed-jobs-history-limit")
	SettingNameRecurringJobMaxRetention                                 = SettingName("recurring-job-max-retention")
	SettingNameSupportBundleFailedHistoryLimit                          = SettingName("support-bundle-failed-history-limit")
	SettingNameSupportBundleNodeCollectionTimeout                       = SettingName("support-bundle-node-collection-timeout")
	SettingNameDeletingConfirmationFlag                                 = SettingName("deleting-confirmation-flag")
	SettingNameEngineReplicaTimeout                                     = SettingName("engine-replica-timeout")
	SettingNameSnapshotDataIntegrity                                    = SettingName("snapshot-data-integrity")
	SettingNameSnapshotDataIntegrityImmediateCheckAfterSnapshotCreation = SettingName("snapshot-data-integrity-immediate-check-after-snapshot-creation")
	SettingNameSnapshotDataIntegrityCronJob                             = SettingName("snapshot-data-integrity-cronjob")
	SettingNameSnapshotMaxCount                                         = SettingName("snapshot-max-count")
	SettingNameRestoreVolumeRecurringJobs                               = SettingName("restore-volume-recurring-jobs")
	SettingNameRemoveSnapshotsDuringFilesystemTrim                      = SettingName("remove-snapshots-during-filesystem-trim")
	SettingNameFastReplicaRebuildEnabled                                = SettingName("fast-replica-rebuild-enabled")
	SettingNameReplicaFileSyncHTTPClientTimeout                         = SettingName("replica-file-sync-http-client-timeout")
	SettingNameLongGPRCTimeOut                                          = SettingName("long-grpc-timeout")
	SettingNameBackupCompressionMethod                                  = SettingName("backup-compression-method")
	SettingNameBackupConcurrentLimit                                    = SettingName("backup-concurrent-limit")
	SettingNameRestoreConcurrentLimit                                   = SettingName("restore-concurrent-limit")
	SettingNameLogLevel                                                 = SettingName("log-level")
	SettingNameReplicaDiskSoftAntiAffinity                              = SettingName("replica-disk-soft-anti-affinity")
	SettingNameAllowEmptyNodeSelectorVolume                             = SettingName("allow-empty-node-selector-volume")
	SettingNameAllowEmptyDiskSelectorVolume                             = SettingName("allow-empty-disk-selector-volume")
	SettingNameDisableSnapshotPurge                                     = SettingName("disable-snapshot-purge")
	SettingNameV1DataEngine                                             = SettingName("v1-data-engine")
	SettingNameV2DataEngine                                             = SettingName("v2-data-engine")
	SettingNameV2DataEngineHugepageLimit                                = SettingName("v2-data-engine-hugepage-limit")
	SettingNameV2DataEngineGuaranteedInstanceManagerCPU                 = SettingName("v2-data-engine-guaranteed-instance-manager-cpu")
	SettingNameV2DataEngineCPUMask                                      = SettingName("v2-data-engine-cpu-mask")
	SettingNameV2DataEngineLogLevel                                     = SettingName("v2-data-engine-log-level")
	SettingNameV2DataEngineLogFlags                                     = SettingName("v2-data-engine-log-flags")
	SettingNameV2DataEngineFastReplicaRebuilding                        = SettingName("v2-data-engine-fast-replica-rebuilding")
	SettingNameFreezeFilesystemForSnapshot                              = SettingName("freeze-filesystem-for-snapshot")
	SettingNameAutoCleanupSnapshotWhenDeleteBackup                      = SettingName("auto-cleanup-when-delete-backup")
	SettingNameDefaultMinNumberOfBackingImageCopies                     = SettingName("default-min-number-of-backing-image-copies")
	SettingNameBackupExecutionTimeout                                   = SettingName("backup-execution-timeout")
	SettingNameRWXVolumeFastFailover                                    = SettingName("rwx-volume-fast-failover")
	// These three backup target parameters are used in the "longhorn-default-resource" ConfigMap
	// to update the default BackupTarget resource.
	// Longhorn won't create the Setting resources for these three parameters.
	SettingNameBackupTarget                 = SettingName("backup-target")
	SettingNameBackupTargetCredentialSecret = SettingName("backup-target-credential-secret")
	SettingNameBackupstorePollInterval      = SettingName("backupstore-poll-interval")
)

var (
	SettingNameList = []SettingName{
		SettingNameAllowRecurringJobWhileVolumeDetached,
		SettingNameCreateDefaultDiskLabeledNodes,
		SettingNameDefaultDataPath,
		SettingNameDefaultEngineImage,
		SettingNameDefaultInstanceManagerImage,
		SettingNameDefaultBackingImageManagerImage,
		SettingNameSupportBundleManagerImage,
		SettingNameReplicaSoftAntiAffinity,
		SettingNameReplicaAutoBalance,
		SettingNameReplicaAutoBalanceDiskPressurePercentage,
		SettingNameStorageOverProvisioningPercentage,
		SettingNameStorageMinimalAvailablePercentage,
		SettingNameStorageReservedPercentageForDefaultDisk,
		SettingNameUpgradeChecker,
		SettingNameUpgradeResponderURL,
		SettingNameAllowCollectingLonghornUsage,
		SettingNameCurrentLonghornVersion,
		SettingNameLatestLonghornVersion,
		SettingNameStableLonghornVersions,
		SettingNameDefaultReplicaCount,
		SettingNameDefaultDataLocality,
		SettingNameDefaultLonghornStaticStorageClass,
		SettingNameTaintToleration,
		SettingNameSystemManagedComponentsNodeSelector,
		SettingNameCRDAPIVersion,
		SettingNameAutoSalvage,
		SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly,
		SettingNameRegistrySecret,
		SettingNameDisableSchedulingOnCordonedNode,
		SettingNameReplicaZoneSoftAntiAffinity,
		SettingNameNodeDownPodDeletionPolicy,
		SettingNameNodeDrainPolicy,
		SettingNameDetachManuallyAttachedVolumesWhenCordoned,
		SettingNamePriorityClass,
		SettingNameDisableRevisionCounter,
		SettingNameReplicaReplenishmentWaitInterval,
		SettingNameConcurrentReplicaRebuildPerNodeLimit,
		SettingNameConcurrentBackingImageCopyReplenishPerNodeLimit,
		SettingNameConcurrentBackupRestorePerNodeLimit,
		SettingNameSystemManagedPodsImagePullPolicy,
		SettingNameAllowVolumeCreationWithDegradedAvailability,
		SettingNameAutoCleanupSystemGeneratedSnapshot,
		SettingNameAutoCleanupRecurringJobBackupSnapshot,
		SettingNameConcurrentAutomaticEngineUpgradePerNodeLimit,
		SettingNameBackingImageCleanupWaitInterval,
		SettingNameBackingImageRecoveryWaitInterval,
		SettingNameGuaranteedInstanceManagerCPU,
		SettingNameKubernetesClusterAutoscalerEnabled,
		SettingNameOrphanAutoDeletion,
		SettingNameStorageNetwork,
		SettingNameStorageNetworkForRWXVolumeEnabled,
		SettingNameFailedBackupTTL,
		SettingNameRecurringSuccessfulJobsHistoryLimit,
		SettingNameRecurringFailedJobsHistoryLimit,
		SettingNameRecurringJobMaxRetention,
		SettingNameSupportBundleFailedHistoryLimit,
		SettingNameSupportBundleNodeCollectionTimeout,
		SettingNameDeletingConfirmationFlag,
		SettingNameEngineReplicaTimeout,
		SettingNameSnapshotDataIntegrity,
		SettingNameSnapshotDataIntegrityCronJob,
		SettingNameSnapshotDataIntegrityImmediateCheckAfterSnapshotCreation,
		SettingNameSnapshotMaxCount,
		SettingNameRestoreVolumeRecurringJobs,
		SettingNameRemoveSnapshotsDuringFilesystemTrim,
		SettingNameFastReplicaRebuildEnabled,
		SettingNameReplicaFileSyncHTTPClientTimeout,
		SettingNameLongGPRCTimeOut,
		SettingNameBackupCompressionMethod,
		SettingNameBackupConcurrentLimit,
		SettingNameRestoreConcurrentLimit,
		SettingNameLogLevel,
		SettingNameV1DataEngine,
		SettingNameV2DataEngine,
		SettingNameV2DataEngineHugepageLimit,
		SettingNameV2DataEngineGuaranteedInstanceManagerCPU,
		SettingNameV2DataEngineCPUMask,
		SettingNameV2DataEngineLogLevel,
		SettingNameV2DataEngineLogFlags,
		SettingNameV2DataEngineFastReplicaRebuilding,
		SettingNameReplicaDiskSoftAntiAffinity,
		SettingNameAllowEmptyNodeSelectorVolume,
		SettingNameAllowEmptyDiskSelectorVolume,
		SettingNameDisableSnapshotPurge,
		SettingNameFreezeFilesystemForSnapshot,
		SettingNameAutoCleanupSnapshotWhenDeleteBackup,
		SettingNameDefaultMinNumberOfBackingImageCopies,
		SettingNameBackupExecutionTimeout,
		SettingNameRWXVolumeFastFailover,
	}
)

type SettingCategory string

const (
	SettingCategoryGeneral      = SettingCategory("general")
	SettingCategoryBackup       = SettingCategory("backup")
	SettingCategoryOrphan       = SettingCategory("orphan")
	SettingCategoryScheduling   = SettingCategory("scheduling")
	SettingCategoryDangerZone   = SettingCategory("danger Zone")
	SettingCategorySnapshot     = SettingCategory("snapshot")
	SettingCategoryV2DataEngine = SettingCategory("v2 data engine (Experimental Feature)")
)

type SettingDefinition struct {
	DisplayName string          `json:"displayName"`
	Description string          `json:"description"`
	Category    SettingCategory `json:"category"`
	Type        SettingType     `json:"type"`
	Required    bool            `json:"required"`
	ReadOnly    bool            `json:"readOnly"`
	Default     string          `json:"default"`
	Choices     []string        `json:"options,omitempty"` // +optional
	// Use map to present minimum and maximum value instead of using int directly, so we can omitempy and distinguish 0 or nil at the same time.
	ValueIntRange map[string]int `json:"range,omitempty"` // +optional
}

var settingDefinitionsLock sync.RWMutex

var (
	settingDefinitions = map[SettingName]SettingDefinition{
		SettingNameAllowRecurringJobWhileVolumeDetached:                     SettingDefinitionAllowRecurringJobWhileVolumeDetached,
		SettingNameCreateDefaultDiskLabeledNodes:                            SettingDefinitionCreateDefaultDiskLabeledNodes,
		SettingNameDefaultDataPath:                                          SettingDefinitionDefaultDataPath,
		SettingNameDefaultEngineImage:                                       SettingDefinitionDefaultEngineImage,
		SettingNameDefaultInstanceManagerImage:                              SettingDefinitionDefaultInstanceManagerImage,
		SettingNameDefaultBackingImageManagerImage:                          SettingDefinitionDefaultBackingImageManagerImage,
		SettingNameSupportBundleManagerImage:                                SettingDefinitionSupportBundleManagerImage,
		SettingNameReplicaSoftAntiAffinity:                                  SettingDefinitionReplicaSoftAntiAffinity,
		SettingNameReplicaAutoBalance:                                       SettingDefinitionReplicaAutoBalance,
		SettingNameReplicaAutoBalanceDiskPressurePercentage:                 SettingDefinitionReplicaAutoBalanceDiskPressurePercentage,
		SettingNameStorageOverProvisioningPercentage:                        SettingDefinitionStorageOverProvisioningPercentage,
		SettingNameStorageMinimalAvailablePercentage:                        SettingDefinitionStorageMinimalAvailablePercentage,
		SettingNameStorageReservedPercentageForDefaultDisk:                  SettingDefinitionStorageReservedPercentageForDefaultDisk,
		SettingNameUpgradeChecker:                                           SettingDefinitionUpgradeChecker,
		SettingNameUpgradeResponderURL:                                      SettingDefinitionUpgradeResponderURL,
		SettingNameAllowCollectingLonghornUsage:                             SettingDefinitionAllowCollectingLonghornUsageMetrics,
		SettingNameCurrentLonghornVersion:                                   SettingDefinitionCurrentLonghornVersion,
		SettingNameLatestLonghornVersion:                                    SettingDefinitionLatestLonghornVersion,
		SettingNameStableLonghornVersions:                                   SettingDefinitionStableLonghornVersions,
		SettingNameDefaultReplicaCount:                                      SettingDefinitionDefaultReplicaCount,
		SettingNameDefaultDataLocality:                                      SettingDefinitionDefaultDataLocality,
		SettingNameDefaultLonghornStaticStorageClass:                        SettingDefinitionDefaultLonghornStaticStorageClass,
		SettingNameTaintToleration:                                          SettingDefinitionTaintToleration,
		SettingNameSystemManagedComponentsNodeSelector:                      SettingDefinitionSystemManagedComponentsNodeSelector,
		SettingNameCRDAPIVersion:                                            SettingDefinitionCRDAPIVersion,
		SettingNameAutoSalvage:                                              SettingDefinitionAutoSalvage,
		SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly:              SettingDefinitionAutoDeletePodWhenVolumeDetachedUnexpectedly,
		SettingNameRegistrySecret:                                           SettingDefinitionRegistrySecret,
		SettingNameDisableSchedulingOnCordonedNode:                          SettingDefinitionDisableSchedulingOnCordonedNode,
		SettingNameReplicaZoneSoftAntiAffinity:                              SettingDefinitionReplicaZoneSoftAntiAffinity,
		SettingNameNodeDownPodDeletionPolicy:                                SettingDefinitionNodeDownPodDeletionPolicy,
		SettingNameNodeDrainPolicy:                                          SettingDefinitionNodeDrainPolicy,
		SettingNameDetachManuallyAttachedVolumesWhenCordoned:                SettingDefinitionDetachManuallyAttachedVolumesWhenCordoned,
		SettingNamePriorityClass:                                            SettingDefinitionPriorityClass,
		SettingNameDisableRevisionCounter:                                   SettingDefinitionDisableRevisionCounter,
		SettingNameReplicaReplenishmentWaitInterval:                         SettingDefinitionReplicaReplenishmentWaitInterval,
		SettingNameConcurrentReplicaRebuildPerNodeLimit:                     SettingDefinitionConcurrentReplicaRebuildPerNodeLimit,
		SettingNameConcurrentBackingImageCopyReplenishPerNodeLimit:          SettingDefinitionConcurrentBackingImageCopyReplenishPerNodeLimit,
		SettingNameConcurrentBackupRestorePerNodeLimit:                      SettingDefinitionConcurrentVolumeBackupRestorePerNodeLimit,
		SettingNameSystemManagedPodsImagePullPolicy:                         SettingDefinitionSystemManagedPodsImagePullPolicy,
		SettingNameAllowVolumeCreationWithDegradedAvailability:              SettingDefinitionAllowVolumeCreationWithDegradedAvailability,
		SettingNameAutoCleanupSystemGeneratedSnapshot:                       SettingDefinitionAutoCleanupSystemGeneratedSnapshot,
		SettingNameAutoCleanupRecurringJobBackupSnapshot:                    SettingDefinitionAutoCleanupRecurringJobBackupSnapshot,
		SettingNameConcurrentAutomaticEngineUpgradePerNodeLimit:             SettingDefinitionConcurrentAutomaticEngineUpgradePerNodeLimit,
		SettingNameBackingImageCleanupWaitInterval:                          SettingDefinitionBackingImageCleanupWaitInterval,
		SettingNameBackingImageRecoveryWaitInterval:                         SettingDefinitionBackingImageRecoveryWaitInterval,
		SettingNameGuaranteedInstanceManagerCPU:                             SettingDefinitionGuaranteedInstanceManagerCPU,
		SettingNameKubernetesClusterAutoscalerEnabled:                       SettingDefinitionKubernetesClusterAutoscalerEnabled,
		SettingNameOrphanAutoDeletion:                                       SettingDefinitionOrphanAutoDeletion,
		SettingNameStorageNetwork:                                           SettingDefinitionStorageNetwork,
		SettingNameStorageNetworkForRWXVolumeEnabled:                        SettingDefinitionStorageNetworkForRWXVolumeEnabled,
		SettingNameFailedBackupTTL:                                          SettingDefinitionFailedBackupTTL,
		SettingNameRecurringSuccessfulJobsHistoryLimit:                      SettingDefinitionRecurringSuccessfulJobsHistoryLimit,
		SettingNameRecurringFailedJobsHistoryLimit:                          SettingDefinitionRecurringFailedJobsHistoryLimit,
		SettingNameRecurringJobMaxRetention:                                 SettingDefinitionRecurringJobMaxRetention,
		SettingNameSupportBundleFailedHistoryLimit:                          SettingDefinitionSupportBundleFailedHistoryLimit,
		SettingNameSupportBundleNodeCollectionTimeout:                       SettingDefinitionSupportBundleNodeCollectionTimeout,
		SettingNameDeletingConfirmationFlag:                                 SettingDefinitionDeletingConfirmationFlag,
		SettingNameEngineReplicaTimeout:                                     SettingDefinitionEngineReplicaTimeout,
		SettingNameSnapshotDataIntegrity:                                    SettingDefinitionSnapshotDataIntegrity,
		SettingNameSnapshotDataIntegrityImmediateCheckAfterSnapshotCreation: SettingDefinitionSnapshotDataIntegrityImmediateCheckAfterSnapshotCreation,
		SettingNameSnapshotDataIntegrityCronJob:                             SettingDefinitionSnapshotDataIntegrityCronJob,
		SettingNameSnapshotMaxCount:                                         SettingDefinitionSnapshotMaxCount,
		SettingNameRestoreVolumeRecurringJobs:                               SettingDefinitionRestoreVolumeRecurringJobs,
		SettingNameRemoveSnapshotsDuringFilesystemTrim:                      SettingDefinitionRemoveSnapshotsDuringFilesystemTrim,
		SettingNameFastReplicaRebuildEnabled:                                SettingDefinitionFastReplicaRebuildEnabled,
		SettingNameReplicaFileSyncHTTPClientTimeout:                         SettingDefinitionReplicaFileSyncHTTPClientTimeout,
		SettingNameLongGPRCTimeOut:                                          SettingDefinitionLongGPRCTimeOut,
		SettingNameBackupCompressionMethod:                                  SettingDefinitionBackupCompressionMethod,
		SettingNameBackupConcurrentLimit:                                    SettingDefinitionBackupConcurrentLimit,
		SettingNameRestoreConcurrentLimit:                                   SettingDefinitionRestoreConcurrentLimit,
		SettingNameLogLevel:                                                 SettingDefinitionLogLevel,
		SettingNameV1DataEngine:                                             SettingDefinitionV1DataEngine,
		SettingNameV2DataEngine:                                             SettingDefinitionV2DataEngine,
		SettingNameV2DataEngineHugepageLimit:                                SettingDefinitionV2DataEngineHugepageLimit,
		SettingNameV2DataEngineGuaranteedInstanceManagerCPU:                 SettingDefinitionV2DataEngineGuaranteedInstanceManagerCPU,
		SettingNameV2DataEngineCPUMask:                                      SettingDefinitionV2DataEngineCPUMask,
		SettingNameV2DataEngineLogLevel:                                     SettingDefinitionV2DataEngineLogLevel,
		SettingNameV2DataEngineLogFlags:                                     SettingDefinitionV2DataEngineLogFlags,
		SettingNameV2DataEngineFastReplicaRebuilding:                        SettingDefinitionV2DataEngineFastReplicaRebuilding,
		SettingNameReplicaDiskSoftAntiAffinity:                              SettingDefinitionReplicaDiskSoftAntiAffinity,
		SettingNameAllowEmptyNodeSelectorVolume:                             SettingDefinitionAllowEmptyNodeSelectorVolume,
		SettingNameAllowEmptyDiskSelectorVolume:                             SettingDefinitionAllowEmptyDiskSelectorVolume,
		SettingNameDisableSnapshotPurge:                                     SettingDefinitionDisableSnapshotPurge,
		SettingNameFreezeFilesystemForSnapshot:                              SettingDefinitionFreezeFilesystemForSnapshot,
		SettingNameAutoCleanupSnapshotWhenDeleteBackup:                      SettingDefinitionAutoCleanupSnapshotWhenDeleteBackup,
		SettingNameDefaultMinNumberOfBackingImageCopies:                     SettingDefinitionDefaultMinNumberOfBackingImageCopies,
		SettingNameBackupExecutionTimeout:                                   SettingDefinitionBackupExecutionTimeout,
		SettingNameRWXVolumeFastFailover:                                    SettingDefinitionRWXVolumeFastFailover,
	}

	SettingDefinitionAllowRecurringJobWhileVolumeDetached = SettingDefinition{
		DisplayName: "Allow Recurring Job While Volume Is Detached",
		Description: "If this setting is enabled, Longhorn will automatically attaches the volume and takes snapshot/backup when it is the time to do recurring snapshot/backup. \n\n" +
			"Note that the volume is not ready for workload during the period when the volume was automatically attached. " +
			"Workload will have to wait until the recurring job finishes.",
		Category: SettingCategoryBackup,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "false",
	}

	SettingDefinitionFailedBackupTTL = SettingDefinition{
		DisplayName: "Failed Backup Time to Live",
		Description: "In minutes. This setting determines how long Longhorn will keep the backup resource that was failed. Set to 0 to disable the auto-deletion.\n" +
			"Failed backups will be checked and cleaned up during backupstore polling which is controlled by **Backupstore Poll Interval** setting.\n" +
			"Hence this value determines the minimal wait interval of the cleanup. And the actual cleanup interval is multiple of **Backupstore Poll Interval**.\n" +
			"Disabling **Backupstore Poll Interval** also means to disable failed backup auto-deletion.\n\n",
		Category: SettingCategoryBackup,
		Type:     SettingTypeInt,
		Required: true,
		ReadOnly: false,
		Default:  "1440",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionBackupExecutionTimeout = SettingDefinition{
		DisplayName: "Backup Execution Timeout",
		Description: "Number of minutes that Longhorn allows for the backup execution. The default value is 1.",
		Category:    SettingCategoryBackup,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "1",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 1,
		},
	}

	SettingDefinitionRestoreVolumeRecurringJobs = SettingDefinition{
		DisplayName: "Restore Volume Recurring Jobs",
		Description: "Restore recurring jobs from the backup volume on the backup target and create recurring jobs if not exist during a backup restoration.\n\n" +
			"Longhorn also supports individual volume setting. The setting can be specified on Backup page when making a backup restoration, this overrules the global setting.\n\n" +
			"The available volume setting options are: \n\n" +
			"- **ignored**. This is the default option that instructs Longhorn to inherit from the global setting.\n" +
			"- **enabled**. This option instructs Longhorn to restore recurring jobs/groups from the backup target forcibly.\n" +
			"- **disabled**. This option instructs Longhorn no restoring recurring jobs/groups should be done.\n",
		Category: SettingCategoryBackup,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "false",
	}

	SettingDefinitionCreateDefaultDiskLabeledNodes = SettingDefinition{
		DisplayName: "Create Default Disk on Labeled Nodes",
		Description: "Create default Disk automatically only on Nodes with the label " +
			"\"node.longhorn.io/create-default-disk=true\" if no other disks exist. If disabled, the default disk will " +
			"be created on all new nodes when each node is first added.",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "false",
	}

	SettingDefinitionDefaultDataPath = SettingDefinition{
		DisplayName: "Default Data Path",
		Description: "Default path to use for storing data on a host",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    false,
		Default:     "/var/lib/longhorn/",
	}

	SettingDefinitionDefaultEngineImage = SettingDefinition{
		DisplayName: "Default Engine Image",
		Description: "The default engine image used by the manager. Can be changed on the manager starting command line only",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    true,
	}

	SettingDefinitionDefaultInstanceManagerImage = SettingDefinition{
		DisplayName: "Default Instance Manager Image",
		Description: "The default instance manager image used by the manager. Can be changed on the manager starting command line only",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeDeprecated,
		Required:    true,
		ReadOnly:    true,
	}

	SettingDefinitionDefaultBackingImageManagerImage = SettingDefinition{
		DisplayName: "Default Backing Image Manager Image",
		Description: "The default backing image manager image used by the manager. Can be changed on the manager starting command line only",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeDeprecated,
		Required:    true,
		ReadOnly:    true,
	}

	SettingDefinitionSupportBundleManagerImage = SettingDefinition{
		DisplayName: "Support Bundle Manager Image",
		Description: "The support bundle manager image for the support bundle generation.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    false,
	}

	SettingDefinitionReplicaSoftAntiAffinity = SettingDefinition{
		DisplayName: "Replica Node Level Soft Anti-Affinity",
		Description: "Allow scheduling on nodes with existing healthy replicas of the same volume",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "false",
	}

	SettingDefinitionFreezeFilesystemForSnapshot = SettingDefinition{
		DisplayName: "Freeze Filesystem For Snapshot",
		Description: "Setting that freezes the filesystem on the root partition before a snapshot is created.",
		Category:    SettingCategorySnapshot,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "false",
	}

	SettingDefinitionReplicaAutoBalance = SettingDefinition{
		DisplayName: "Replica Auto Balance",
		Description: "Enable this setting automatically rebalances replicas when discovered an available node.\n\n" +
			"The available global options are: \n\n" +
			"- **disabled**. This is the default option. No replica auto-balance will be done.\n" +
			"- **least-effort**. This option instructs Longhorn to balance replicas for minimal redundancy.\n" +
			"- **best-effort**. This option instructs Longhorn to balance replicas for even redundancy.\n\n" +
			"Longhorn also support individual volume setting. The setting can be specified on Volume page, this overrules the global setting.\n\n" +
			"The available volume setting options are: \n\n" +
			"- **ignored**. This is the default option that instructs Longhorn to inherit from the global setting.\n" +
			"- **disabled**. This option instructs Longhorn no replica auto-balance should be done.\n" +
			"- **least-effort**. This option instructs Longhorn to balance replicas for minimal redundancy.\n" +
			"- **best-effort**. This option instructs Longhorn to balance replicas for even redundancy.\n",
		Category: SettingCategoryScheduling,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  string(longhorn.ReplicaAutoBalanceDisabled),
		Choices: []string{
			string(longhorn.ReplicaAutoBalanceDisabled),
			string(longhorn.ReplicaAutoBalanceLeastEffort),
			string(longhorn.ReplicaAutoBalanceBestEffort),
		},
	}

	SettingDefinitionReplicaAutoBalanceDiskPressurePercentage = SettingDefinition{
		DisplayName: "Replica Auto Balance Disk Pressure Threshold (%)",
		Description: "Percentage of currently used storage that triggers automatic replica rebalancing.\n\n" +
			"When the threshold is reached, Longhorn automatically rebuilds replicas that are under disk pressure on another disk within the same node.\n\n" +
			"To disable this feature, set the value to 0.\n\n" +
			"**Note:** This setting takes effect only when the following conditions are met:\n" +
			"- **Replica Auto Balance** is set to **best-effort**.\n" +
			"- At least one other disk on the node has sufficient available space.\n\n" +
			"**Note:** This feature is not affected by the **Replica Node Level Soft Anti-Affinity** setting.",
		Category: SettingCategoryScheduling,
		Type:     SettingTypeInt,
		Required: true,
		ReadOnly: false,
		Default:  "90",
	}

	SettingDefinitionStorageOverProvisioningPercentage = SettingDefinition{
		DisplayName: "Storage Over Provisioning Percentage",
		Description: "The over-provisioning percentage defines how much storage can be allocated relative to the hard drive's capacity",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "100",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionStorageMinimalAvailablePercentage = SettingDefinition{
		DisplayName: "Storage Minimal Available Percentage",
		Description: "If the minimum available disk capacity exceeds the actual percentage of available disk capacity, the disk becomes unschedulable until more space is freed up.",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "25",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
			ValueIntRangeMaximum: 100,
		},
	}

	SettingDefinitionStorageReservedPercentageForDefaultDisk = SettingDefinition{
		DisplayName: "Storage Reserved Percentage For Default Disk",
		Description: "The reserved percentage specifies the percentage of disk space that will not be allocated to the default disk on each new Longhorn node",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "30",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
			ValueIntRangeMaximum: 100,
		},
	}

	SettingDefinitionUpgradeChecker = SettingDefinition{
		DisplayName: "Enable Upgrade Checker",
		Description: "Upgrade Checker will check for new Longhorn version periodically. When there is a new version available, a notification will appear in the UI",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionUpgradeResponderURL = SettingDefinition{
		DisplayName: "Upgrade Responder URL",
		Description: "The Upgrade Responder sends a notification whenever a new Longhorn version that you can upgrade to becomes available",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    false,
		Default:     "https://longhorn-upgrade-responder.rancher.io/v1/checkupgrade",
	}

	SettingDefinitionAllowCollectingLonghornUsageMetrics = SettingDefinition{
		DisplayName: "Allow Collecting Longhorn Usage Metrics",
		Description: "Enabling this setting will allow Longhorn to provide additional usage metrics to https://metrics.longhorn.io/.\n" +
			"This information will help us better understand how Longhorn is being used, which will ultimately contribute to future improvements.\n",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "true",
	}

	SettingDefinitionCurrentLonghornVersion = SettingDefinition{
		DisplayName: "Current Longhorn Version",
		Description: "The current Longhorn version.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    false,
		ReadOnly:    true,
		Default:     meta.Version,
	}

	SettingDefinitionLatestLonghornVersion = SettingDefinition{
		DisplayName: "Latest Longhorn Version",
		Description: "The latest version of Longhorn available. Updated by Upgrade Checker automatically",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    false,
		ReadOnly:    true,
	}

	SettingDefinitionStableLonghornVersions = SettingDefinition{
		DisplayName: "Stable Longhorn Versions",
		Description: "The latest stable version of every minor release line. Updated by Upgrade Checker automatically",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    false,
		ReadOnly:    true,
	}

	SettingDefinitionDefaultReplicaCount = SettingDefinition{
		DisplayName: "Default Replica Count",
		Description: "The default number of replicas when a volume is created from the Longhorn UI. For Kubernetes configuration, update the `numberOfReplicas` in the StorageClass",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "3",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 1,
			ValueIntRangeMaximum: 20,
		},
	}

	SettingDefinitionDefaultDataLocality = SettingDefinition{
		DisplayName: "Default Data Locality",
		Description: "We say a Longhorn volume has data locality if there is a local replica of the volume on the same node as the pod which is using the volume.\n\n" +
			"This setting specifies the default data locality when a volume is created from the Longhorn UI. For Kubernetes configuration, update the `dataLocality` in the StorageClass\n\n" +
			"The available modes are: \n\n" +
			"- **disabled**. This is the default option. There may or may not be a replica on the same node as the attached volume (workload)\n" +
			"- **best-effort**. This option instructs Longhorn to try to keep a replica on the same node as the attached volume (workload). Longhorn will not stop the volume, even if it cannot keep a replica local to the attached volume (workload) due to environment limitation, e.g. not enough disk space, incompatible disk tags, etc.\n" +
			"- **strict-local**. This option enforces Longhorn keep the only one replica on the same node as the attached volume.\n",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  string(longhorn.DataLocalityDisabled),
		Choices: []string{
			string(longhorn.DataLocalityDisabled),
			string(longhorn.DataLocalityBestEffort),
			string(longhorn.DataLocalityStrictLocal),
		},
	}

	SettingDefinitionDefaultLonghornStaticStorageClass = SettingDefinition{
		DisplayName: "Default Longhorn Static StorageClass Name",
		Description: "The 'storageClassName' is given to PVs and PVCs that are created for an existing Longhorn volume. The StorageClass name can also be used as a label, so it is possible to use a Longhorn StorageClass to bind a workload to an existing PV without creating a Kubernetes StorageClass object.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    false,
		Default:     "longhorn-static",
	}

	SettingDefinitionTaintToleration = SettingDefinition{
		DisplayName: "Kubernetes Taint Toleration",
		Description: "If you want to dedicate nodes to just store Longhorn replicas and reject other general workloads, you can set tolerations for **all** Longhorn components and add taints to the nodes dedicated for storage. " +
			"Longhorn system contains user deployed components (e.g, Longhorn manager, Longhorn driver, Longhorn UI) and system managed components (e.g, instance manager, engine image, CSI driver, etc.) " +
			"This setting only sets taint tolerations for system managed components. " +
			"Depending on how you deployed Longhorn, you need to set taint tolerations for user deployed components in Helm chart or deployment YAML file. " +
			"All Longhorn volumes should be detached before modifying toleration settings. " +
			"We recommend setting tolerations during Longhorn deployment because the Longhorn system cannot be operated during the update. " +
			"Multiple tolerations can be set here, and these tolerations are separated by semicolon. For example: \n\n" +
			"* `key1=value1:NoSchedule; key2:NoExecute` \n\n" +
			"* `:` this toleration tolerates everything because an empty key with operator `Exists` matches all keys, values and effects \n\n" +
			"* `key1=value1:`  this toleration has empty effect. It matches all effects with key `key1` \n\n" +
			"Because `kubernetes.io` is used as the key of all Kubernetes default tolerations, it should not be used in the toleration settings.\n\n",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeString,
		Required: false,
		ReadOnly: false,
	}

	SettingDefinitionSystemManagedComponentsNodeSelector = SettingDefinition{
		DisplayName: "System Managed Components Node Selector",
		Description: "If you want to restrict Longhorn components to only run on particular set of nodes, you can set node selector for **all** Longhorn components. " +
			"Longhorn system contains user deployed components (e.g, Longhorn manager, Longhorn driver, Longhorn UI) and system managed components (e.g, instance manager, engine image, CSI driver, etc.) " +
			"You must follow the below order when set the node selector:\n\n" +
			"1. Set node selector for user deployed components in Helm chart or deployment YAML file depending on how you deployed Longhorn.\n\n" +
			"2. Set node selector for system managed components in here.\n\n" +
			"All Longhorn volumes should be detached before modifying node selector settings. " +
			"We recommend setting node selector during Longhorn deployment because the Longhorn system cannot be operated during the update. " +
			"Multiple label key-value pairs are separated by semicolon. For example: \n\n" +
			"* `label-key1=label-value1; label-key2=label-value2` \n\n" +
			"Please see the documentation at https://longhorn.io for more detailed instructions about changing node selector",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeString,
		Required: false,
		ReadOnly: false,
	}

	SettingDefinitionCRDAPIVersion = SettingDefinition{
		DisplayName: "Custom Resource API Version",
		Description: "The current customer resource's API version, e.g. longhorn.io/v1beta2. Set by manager automatically",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    true,
	}

	SettingDefinitionAutoSalvage = SettingDefinition{
		DisplayName: "Automatic salvage",
		Description: "If enabled, volumes will be automatically salvaged when all the replicas become faulty e.g. due to network disconnection. Longhorn will try to figure out which replica(s) are usable, then use them for the volume.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionAutoDeletePodWhenVolumeDetachedUnexpectedly = SettingDefinition{
		DisplayName: "Automatically Delete Workload Pod when The Volume Is Detached Unexpectedly",
		Description: "If enabled, Longhorn will automatically delete the workload pod that is managed by a controller (e.g. deployment, statefulset, daemonset, etc...) when Longhorn volume is detached unexpectedly (e.g. during Kubernetes upgrade, Docker reboot, or network disconnect). " +
			"By deleting the pod, its controller restarts the pod and Kubernetes handles volume reattachment and remount. \n\n" +
			"If disabled, Longhorn will not delete the workload pod that is managed by a controller. You will have to manually restart the pod to reattach and remount the volume. \n\n" +
			"**Note:** This setting doesn't apply to the workload pods that don't have a controller. Longhorn never deletes them.",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "true",
	}

	SettingDefinitionRegistrySecret = SettingDefinition{
		DisplayName: "Registry secret",
		Description: "The Kubernetes Secret name",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    false,
		ReadOnly:    false,
		Default:     "",
	}

	SettingDefinitionDisableSchedulingOnCordonedNode = SettingDefinition{
		DisplayName: "Disable Scheduling On Cordoned Node",
		Description: `Disable Longhorn manager to schedule replica on Kubernetes cordoned node`,
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionReplicaZoneSoftAntiAffinity = SettingDefinition{
		DisplayName: "Replica Zone Level Soft Anti-Affinity",
		Description: "Allow scheduling new Replicas of Volume to the Nodes in the same Zone as existing healthy Replicas. Nodes don't belong to any Zone will be treated as in the same Zone. Notice that Longhorn relies on label `topology.kubernetes.io/zone=<Zone name of the node>` in the Kubernetes node object to identify the zone.",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionNodeDownPodDeletionPolicy = SettingDefinition{
		DisplayName: "Pod Deletion Policy When Node is Down",
		Description: "Defines the Longhorn action when a Volume is stuck with a StatefulSet/Deployment Pod on a node that is down.\n" +
			"- **do-nothing** is the default Kubernetes behavior of never force deleting StatefulSet/Deployment terminating pods. Since the pod on the node that is down isn't removed, Longhorn volumes are stuck on nodes that are down.\n" +
			"- **delete-statefulset-pod** Longhorn will force delete StatefulSet terminating pods on nodes that are down to release Longhorn volumes so that Kubernetes can spin up replacement pods.\n" +
			"- **delete-deployment-pod** Longhorn will force delete Deployment terminating pods on nodes that are down to release Longhorn volumes so that Kubernetes can spin up replacement pods.\n" +
			"- **delete-both-statefulset-and-deployment-pod** Longhorn will force delete StatefulSet/Deployment terminating pods on nodes that are down to release Longhorn volumes so that Kubernetes can spin up replacement pods.\n",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  string(NodeDownPodDeletionPolicyDoNothing),
		Choices: []string{
			string(NodeDownPodDeletionPolicyDoNothing),
			string(NodeDownPodDeletionPolicyDeleteStatefulSetPod),
			string(NodeDownPodDeletionPolicyDeleteDeploymentPod),
			string(NodeDownPodDeletionPolicyDeleteBothStatefulsetAndDeploymentPod),
		},
	}

	SettingDefinitionNodeDrainPolicy = SettingDefinition{
		DisplayName: "Node Drain Policy",
		Description: "Define the policy to use when a node with the last healthy replica of a volume is drained.\n" +
			"- **block-for-eviction** Longhorn will automatically evict all replicas and block the drain until eviction is complete.\n" +
			"- **block-for-eviction-if-contains-last-replica** Longhorn will automatically evict any replicas that don't have a healthy counterpart and block the drain until eviction is complete.\n" +
			"- **block-if-contains-last-replica** Longhorn will block the drain when the node contains the last healthy replica of a volume.\n" +
			"- **allow-if-replica-is-stopped** Longhorn will allow the drain when the node contains the last healthy replica of a volume but the replica is stopped. WARNING: possible data loss if the node is removed after draining. Select this option if you want to drain the node and do in-place upgrade/maintenance.\n" +
			"- **always-allow** Longhorn will allow the drain even though the node contains the last healthy replica of a volume. WARNING: possible data loss if the node is removed after draining. Also possible data corruption if the last replica was running during the draining.\n",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  string(NodeDrainPolicyBlockIfContainsLastReplica),
		Choices: []string{
			string(NodeDrainPolicyBlockForEviction),
			string(NodeDrainPolicyBlockForEvictionIfContainsLastReplica),
			string(NodeDrainPolicyBlockIfContainsLastReplica),
			string(NodeDrainPolicyAllowIfReplicaIsStopped),
			string(NodeDrainPolicyAlwaysAllow),
		},
	}

	SettingDefinitionDetachManuallyAttachedVolumesWhenCordoned = SettingDefinition{
		DisplayName: "Detach Manually Attached Volumes When Cordoned",
		Description: "Automatically detach volumes that are attached manually when node is cordoned.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "false",
	}

	SettingDefinitionPriorityClass = SettingDefinition{
		DisplayName: "Priority Class",
		Description: "The name of the Priority Class to set on the Longhorn components. This can help prevent Longhorn components from being evicted under Node Pressure. \n" +
			"Longhorn system contains user deployed components (e.g, Longhorn manager, Longhorn driver, Longhorn UI) and system managed components (e.g, instance manager, engine image, CSI driver, etc.) " +
			"Note that this setting only sets Priority Class for system managed components. " +
			"Depending on how you deployed Longhorn, you need to set Priority Class for user deployed components in Helm chart or deployment YAML file. \n",
		Category: SettingCategoryDangerZone,
		Required: false,
		ReadOnly: false,
	}

	SettingDefinitionDisableRevisionCounter = SettingDefinition{
		DisplayName: "Disable Revision Counter",
		Description: "This setting is only for volumes created by UI. By default, this is true meaning Longhorn will not have revision counter file to track every write to the volume. During the salvage recovering, Longhorn will use the 'volume-head-xxx.img' file last modification time and file size to pick the replica candidate to recover the whole volume. If this setting is false, there will be a revision counter file to track every write to the volume. During salvage recovering Longhorn will pick the replica with largest revision counter as candidate to recover the whole volume.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionReplicaReplenishmentWaitInterval = SettingDefinition{
		DisplayName: "Replica Replenishment Wait Interval",
		Description: "In seconds. The interval determines how long Longhorn will wait at least in order to reuse the existing data on a failed replica rather than directly creating a new replica for a degraded volume.\n" +
			"Warning: This option works only when there is a failed replica in the volume. And this option may block the rebuilding for a while in the case.",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeInt,
		Required: true,
		ReadOnly: false,
		Default:  "600",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionConcurrentReplicaRebuildPerNodeLimit = SettingDefinition{
		DisplayName: "Concurrent Replica Rebuild Per Node Limit",
		Description: "This setting controls how many replicas on a node can be rebuilt simultaneously. \n\n" +
			"Typically, Longhorn can block the replica starting once the current rebuilding count on a node exceeds the limit. But when the value is 0, it means disabling the replica rebuilding. \n\n" +
			"WARNING: \n\n" +
			"  - The old setting \"Disable Replica Rebuild\" is replaced by this setting. \n\n" +
			"  - Different from relying on replica starting delay to limit the concurrent rebuilding, if the rebuilding is disabled, replica object replenishment will be directly skipped. \n\n" +
			"  - When the value is 0, the eviction and data locality feature won't work. But this shouldn't have any impact to any current replica rebuild and backup restore.",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeInt,
		Required: true,
		ReadOnly: false,
		Default:  "5",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionConcurrentBackingImageCopyReplenishPerNodeLimit = SettingDefinition{
		DisplayName: "Concurrent Backing Image Replenish Per Node Limit",
		Description: "This setting controls how many backing images copy on a node can be replenished simultaneously. \n\n" +
			"Typically, Longhorn can block the backing image copy starting once the current replenishing count on a node exceeds the limit. But when the value is 0, it means disabling the backing image replenish.",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeInt,
		Required: true,
		ReadOnly: false,
		Default:  "5",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionConcurrentVolumeBackupRestorePerNodeLimit = SettingDefinition{
		DisplayName: "Concurrent Volume Backup Restore Per Node Limit",
		Description: "This setting controls how many volumes on a node can restore the backup concurrently.\n\n" +
			"Longhorn blocks the backup restore once the restoring volume count exceeds the limit.\n\n" +
			"Set the value to **0** to disable backup restore.\n\n",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeInt,
		Required: true,
		ReadOnly: false,
		Default:  "5",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionSystemManagedPodsImagePullPolicy = SettingDefinition{
		DisplayName: "System Managed Pod Image Pull Policy",
		Description: "This setting defines the Image Pull Policy of Longhorn system managed pods, e.g. instance manager, engine image, CSI driver, etc. " +
			"The new Image Pull Policy will only apply after the system managed pods restart.",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  string(SystemManagedPodsImagePullPolicyIfNotPresent),
		Choices: []string{
			string(SystemManagedPodsImagePullPolicyIfNotPresent),
			string(SystemManagedPodsImagePullPolicyNever),
			string(SystemManagedPodsImagePullPolicyAlways),
		},
	}

	SettingDefinitionAllowVolumeCreationWithDegradedAvailability = SettingDefinition{
		DisplayName: "Allow Volume Creation with Degraded Availability",
		Description: "This setting allows user to create and attach a volume that doesn't have all the replicas scheduled at the time of creation.",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionAutoCleanupSystemGeneratedSnapshot = SettingDefinition{
		DisplayName: "Automatically Cleanup System Generated Snapshot",
		Description: "This setting enables Longhorn to automatically cleanup the system generated snapshot before and after replica rebuilding.",
		Category:    SettingCategorySnapshot,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionAutoCleanupRecurringJobBackupSnapshot = SettingDefinition{
		DisplayName: "Automatically Cleanup Recurring Job Backup Snapshot",
		Description: "This setting enables Longhorn to automatically cleanup the snapshot generated by a recurring backup job.",
		Category:    SettingCategorySnapshot,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionConcurrentAutomaticEngineUpgradePerNodeLimit = SettingDefinition{
		DisplayName: "Concurrent Automatic Engine Upgrade Per Node Limit",
		Description: "This setting controls how Longhorn automatically upgrades volumes' engines after upgrading Longhorn manager. " +
			"The value of this setting specifies the maximum number of engines per node that are allowed to upgrade to the default engine image at the same time. " +
			"If the value is 0, Longhorn will not automatically upgrade volumes' engines to default version.",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeInt,
		Required: true,
		ReadOnly: false,
		Default:  "0",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionBackingImageCleanupWaitInterval = SettingDefinition{
		DisplayName: "Backing Image Cleanup Wait Interval",
		Description: "In minutes. The interval determines how long Longhorn will wait before cleaning up the backing image file when there is no replica in the disk using it.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "60",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionBackingImageRecoveryWaitInterval = SettingDefinition{
		DisplayName: "Backing Image Recovery Wait Interval",
		Description: "In seconds. The interval determines how long Longhorn will wait before re-downloading the backing image file when all disk files of this backing image become failed or unknown. \n\n" +
			"WARNING: \n\n" +
			"  - This recovery only works for the backing image of which the creation type is \"download\". \n\n" +
			"  - File state \"unknown\" means the related manager pods on the pod is not running or the node itself is down/disconnected.",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeInt,
		Required: true,
		ReadOnly: false,
		Default:  "300",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionGuaranteedInstanceManagerCPU = SettingDefinition{
		DisplayName: "Guaranteed Instance Manager CPU for V1 Data Engine",
		Description: "Percentage of the total allocatable CPU resources on each node to be reserved for each instance manager pod when the V1 Data Engine is enabled. For example, 10 means 10% of the total CPU on a node will be allocated to each instance manager pod on this node. This will help maintain engine and replica stability during high node workload. \n\n" +
			"In order to prevent unexpected volume instance (engine/replica) crash as well as guarantee a relative acceptable IO performance, you can use the following formula to calculate a value for this setting: \n\n" +
			"`Guaranteed Instance Manager CPU = The estimated max Longhorn volume engine and replica count on a node * 0.1 / The total allocatable CPUs on the node * 100` \n\n" +
			"The result of above calculation doesn't mean that's the maximum CPU resources the Longhorn workloads require. To fully exploit the Longhorn volume I/O performance, you can allocate/guarantee more CPU resources via this setting. \n\n" +
			"If it's hard to estimate the usage now, you can leave it with the default value, which is 12%. Then you can tune it when there is no running workload using Longhorn volumes. \n\n" +
			"WARNING: \n\n" +
			"  - Value 0 means unsetting CPU requests for instance manager pods. \n\n" +
			"  - Considering the possible new instance manager pods in the further system upgrade, this integer value is range from 0 to 40. \n\n" +
			"  - One more set of instance manager pods may need to be deployed when the Longhorn system is upgraded. If current available CPUs of the nodes are not enough for the new instance manager pods, you need to detach the volumes using the oldest instance manager pods so that Longhorn can clean up the old pods automatically and release the CPU resources. And the new pods with the latest instance manager image will be launched then. \n\n" +
			"  - This global setting will be ignored for a node if the field \"InstanceManagerCPURequest\" on the node is set. \n\n" +
			"  - After this setting is changed, the v1 instance manager pod using this global setting will be automatically restarted without instances running on the v1 instance manager. \n\n",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeInt,
		Required: true,
		ReadOnly: false,
		Default:  "12",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
			ValueIntRangeMaximum: 40,
		},
	}

	SettingDefinitionKubernetesClusterAutoscalerEnabled = SettingDefinition{
		DisplayName: "Kubernetes Cluster Autoscaler Enabled (Experimental)",
		Description: "Setting that notifies Longhorn that the cluster is using the Kubernetes Cluster Autoscaler. \n\n" +
			"Longhorn prevents data loss by only allowing the Cluster Autoscaler to scale down a node that met all conditions: \n\n" +
			"  - No volume attached to the node \n\n" +
			"  - Is not the last node containing the replica of any volume. \n\n" +
			"  - Is not running backing image components pod. \n\n" +
			"  - Is not running share manager components pod. \n\n",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "false",
	}

	SettingDefinitionOrphanAutoDeletion = SettingDefinition{
		DisplayName: "Orphan Auto-Deletion",
		Description: "This setting allows Longhorn to delete the orphan resource and its corresponding orphaned data automatically. \n\n" +
			"Orphan resources on down or unknown nodes will not be cleaned up automatically. \n\n",
		Category: SettingCategoryOrphan,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "false",
	}

	SettingDefinitionStorageNetwork = SettingDefinition{
		DisplayName: "Storage Network",
		Description: "Longhorn uses the storage network for in-cluster data traffic. Leave this blank to use the Kubernetes cluster network. \n\n" +
			"To segregate the storage network, input the pre-existing NetworkAttachmentDefinition in **<namespace>/<name>** format. \n\n" +
			"By default, this setting applies only to RWO (Read-Write-Once) volumes. For RWX (Read-Write-Many) volumes, enable 'Storage Network for RWX Volume' setting.\n\n" +
			"WARNING: \n\n" +
			"  - The cluster must have pre-existing Multus installed, and NetworkAttachmentDefinition IPs are reachable between nodes. \n\n" +
			"  - When applying the setting, Longhorn will try to restart all instance-manager, and backing-image-manager pods if all volumes are detached and eventually restart the instance manager pod without instances running on the instance manager. \n\n",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeString,
		Required: false,
		ReadOnly: false,
		Default:  CniNetworkNone,
	}

	SettingDefinitionStorageNetworkForRWXVolumeEnabled = SettingDefinition{
		DisplayName: "Storage Network for RWX Volume Enabled",
		Description: "This setting allows Longhorn to use the storage network for RWX (Read-Write-Many) volume.\n\n" +
			"WARNING: \n\n" +
			"  - This setting should change after all Longhorn RWX volumes are detached because some Longhorn component pods will be recreated to apply the setting. \n\n" +
			"  - When this setting is enabled, the RWX volumes are mounted with the storage network within the CSI plugin pod container network namespace. As a result, restarting the CSI plugin pod when there are attached RWX volumes may lead to its data path become unresponsive. When this occurs, you must restart the workload pod to re-establish the mount connection. Alternatively, you can enable the 'Automatically Delete Workload Pod when The Volume Is Detached Unexpectedly' setting to allow Longhorn to automatically delete the workload pod.\n\n",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeBool,
		Required: false,
		ReadOnly: false,
		Default:  "false",
	}

	SettingDefinitionRecurringSuccessfulJobsHistoryLimit = SettingDefinition{
		DisplayName: "Cronjob Successful Jobs History Limit",
		Description: "This setting specifies how many successful backup or snapshot job histories should be retained. \n\n" +
			"History will not be retained if the value is 0.",
		Category: SettingCategoryBackup,
		Type:     SettingTypeInt,
		Required: false,
		ReadOnly: false,
		Default:  "1",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionRecurringFailedJobsHistoryLimit = SettingDefinition{
		DisplayName: "Cronjob Failed Jobs History Limit",
		Description: "This setting specifies how many failed backup or snapshot job histories should be retained.\n\n" +
			"History will not be retained if the value is 0.",
		Category: SettingCategoryBackup,
		Type:     SettingTypeInt,
		Required: false,
		ReadOnly: false,
		Default:  "1",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionRecurringJobMaxRetention = SettingDefinition{
		DisplayName: "Maximum Retention Number for Recurring Job",
		Description: "This setting specifies how many snapshots or backups should be retained.",
		Category:    SettingCategoryBackup,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "100",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 1,
			ValueIntRangeMaximum: MaxSnapshotNum,
		},
	}

	SettingDefinitionSupportBundleFailedHistoryLimit = SettingDefinition{
		DisplayName: "SupportBundle Failed History Limit",
		Description: "This setting specifies how many failed support bundles can exist in the cluster.\n\n" +
			"The retained failed support bundle is for analysis purposes and needs to clean up manually.\n\n" +
			"Set this value to **0** to have Longhorn automatically purge all failed support bundles.\n\n",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeInt,
		Required: false,
		ReadOnly: false,
		Default:  "1",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionSupportBundleNodeCollectionTimeout = SettingDefinition{
		DisplayName: "Timeout for Support Bundle Node Collection",
		Description: "In minutes. The timeout for collecting node bundles for support bundle generation. The default value is 30.\n\n" +
			"When the timeout is reached, the support bundle generation will proceed without requiring the collection of node bundles. \n\n",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeInt,
		Required: true,
		ReadOnly: false,
		Default:  "30",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionDeletingConfirmationFlag = SettingDefinition{
		DisplayName: "Deleting Confirmation Flag",
		Description: "This flag is designed to prevent Longhorn from being accidentally uninstalled which will lead to data lost. \n\n" +
			"Set this flag to **true** to allow Longhorn uninstallation. " +
			"If this flag **false**, Longhorn uninstallation job will fail.",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "false",
	}

	SettingDefinitionEngineReplicaTimeout = SettingDefinition{
		DisplayName: "Timeout between Engine and Replica",
		Description: "In seconds. The setting specifies the timeout between the engine and replica(s), and the value should be between 8 to 30 seconds. The default value is 8 seconds.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "8",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 8,
			ValueIntRangeMaximum: 30,
		},
	}

	SettingDefinitionSnapshotDataIntegrity = SettingDefinition{
		DisplayName: "Snapshot Data Integrity",
		Description: "This setting allows users to enable or disable snapshot hashing and data integrity checking. \n\n" +
			"Available options are: \n\n" +
			"- **disabled**: Disable snapshot disk file hashing and data integrity checking. \n\n" +
			"- **enabled**: Enables periodic snapshot disk file hashing and data integrity checking. To detect the filesystem-unaware corruption caused by bit rot or other issues in snapshot disk files, Longhorn system periodically hashes files and finds corrupted ones. Hence, the system performance will be impacted during the periodical checking. \n\n" +
			"- **fast-check**: Enable snapshot disk file hashing and fast data integrity checking. Longhorn system only hashes snapshot disk files if their are not hashed or the modification time are changed. In this mode, filesystem-unaware corruption cannot be detected, but the impact on system performance can be minimized.",
		Category: SettingCategorySnapshot,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  string(longhorn.SnapshotDataIntegrityFastCheck),
		Choices: []string{
			string(longhorn.SnapshotDataIntegrityDisabled),
			string(longhorn.SnapshotDataIntegrityEnabled),
			string(longhorn.SnapshotDataIntegrityFastCheck),
		},
	}

	SettingDefinitionSnapshotDataIntegrityImmediateCheckAfterSnapshotCreation = SettingDefinition{
		DisplayName: "Immediate Snapshot Data Integrity Check After Creating a Snapshot",
		Description: "Hashing snapshot disk files impacts the performance of the system. The immediate snapshot hashing and checking can be disabled to minimize the impact after creating a snapshot.",
		Category:    SettingCategorySnapshot,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "false",
	}

	SettingDefinitionSnapshotDataIntegrityCronJob = SettingDefinition{
		DisplayName: "Snapshot Data Integrity Check CronJob",
		Description: "Unix-cron string format. The setting specifies when Longhorn checks the data integrity of snapshot disk files. \n\n" +
			"Warning: Hashing snapshot disk files impacts the performance of the system. It is recommended to run data integrity checks during off-peak times and to reduce the frequency of checks.",
		Category: SettingCategorySnapshot,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  "0 0 */7 * *",
	}

	SettingDefinitionSnapshotMaxCount = SettingDefinition{
		DisplayName: "Snapshot Maximum Count",
		Description: "Maximum snapshot count for a volume. The value should be between 2 to 250",
		Category:    SettingCategorySnapshot,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     strconv.Itoa(MaxSnapshotNum),
	}

	SettingDefinitionRemoveSnapshotsDuringFilesystemTrim = SettingDefinition{
		DisplayName: "Remove Snapshots During Filesystem Trim",
		Description: "This setting allows Longhorn filesystem trim feature to automatically mark the latest snapshot and its ancestors as removed and stops at the snapshot containing multiple children.\n\n" +
			"Since Longhorn filesystem trim feature can be applied to the volume head and the followed continuous removed or system snapshots only, trying to trim a removed file from a valid snapshot will do nothing but the filesystem will discard this kind of in-memory trimmable file info. " +
			"Later on if you mark the snapshot as removed and want to retry the trim, you may need to unmount and remount the filesystem so that the filesystem can recollect the trimmable file info.",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "false",
	}

	SettingDefinitionFastReplicaRebuildEnabled = SettingDefinition{
		DisplayName: "Fast Replica Rebuild Enabled",
		Description: "This setting enables the fast replica rebuilding feature. It relies on the checksums of snapshot disk files, so setting the snapshot-data-integrity to **enable** or **fast-check** is a prerequisite.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionReplicaFileSyncHTTPClientTimeout = SettingDefinition{
		DisplayName: "Timeout of HTTP Client to Replica File Sync Server",
		Description: "In seconds. The setting specifies the HTTP client timeout to the file sync server.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "30",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 5,
			ValueIntRangeMaximum: 120,
		},
	}

	SettingDefinitionLongGPRCTimeOut = SettingDefinition{
		DisplayName: "Long gRPC Timeout",
		Description: "Number of seconds that Longhorn allows for the completion of replica rebuilding and snapshot cloning operations.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "86400",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 1,
			ValueIntRangeMaximum: 604800,
		},
	}

	SettingDefinitionBackupCompressionMethod = SettingDefinition{
		DisplayName: "Backup Compression Method",
		Description: "This setting allows users to specify backup compression method.\n\n" +
			"Available options are: \n\n" +
			"- **none**: Disable the compression method. Suitable for multimedia data such as encoded images and videos. \n\n" +
			"- **lz4**: Fast compression method. Suitable for flat files. \n\n" +
			"- **gzip**: A bit of higher compression ratio but relatively slow.",
		Category: SettingCategoryBackup,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  string(longhorn.BackupCompressionMethodLz4),
		Choices: []string{
			string(longhorn.BackupCompressionMethodNone),
			string(longhorn.BackupCompressionMethodLz4),
			string(longhorn.BackupCompressionMethodGzip),
		},
	}

	SettingDefinitionBackupConcurrentLimit = SettingDefinition{
		DisplayName: "Backup Concurrent Limit Per Backup",
		Description: "This setting controls how many worker threads per backup concurrently.",
		Category:    SettingCategoryBackup,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "2",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 1,
		},
	}

	SettingDefinitionRestoreConcurrentLimit = SettingDefinition{
		DisplayName: "Restore Concurrent Limit Per Backup",
		Description: "This setting controls how many worker threads per restore concurrently.",
		Category:    SettingCategoryBackup,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "2",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 1,
		},
	}

	SettingDefinitionLogLevel = SettingDefinition{
		DisplayName: "Log Level",
		Description: "The log level Panic, Fatal, Error, Warn, Info, Debug, Trace used in longhorn manager. By default Info.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    false,
		Default:     "Info",
		Choices:     []string{"Panic", "Fatal", "Error", "Warn", "Info", "Debug", "Trace"},
	}

	SettingDefinitionV1DataEngine = SettingDefinition{
		DisplayName: "V1 Data Engine",
		Description: "Setting that allows you to enable the V1 Data Engine. \n\n" +
			"  - DO NOT CHANGE THIS SETTING WITH ATTACHED VOLUMES. Longhorn will block this setting update when there are attached v1 volumes. \n\n",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "true",
	}

	SettingDefinitionV2DataEngine = SettingDefinition{
		DisplayName: "V2 Data Engine",
		Description: "This setting allows users to activate v2 data engine which is based on SPDK. Currently, it is in the experimental phase and should not be utilized in a production environment.\n\n" +
			"  - DO NOT CHANGE THIS SETTING WITH ATTACHED VOLUMES. Longhorn will block this setting update when there are attached v2 volumes. \n\n" +
			"  - When the V2 Data Engine is enabled, each instance-manager pod utilizes 1 CPU core. This high CPU usage is attributed to the spdk_tgt process running within each instance-manager pod. The spdk_tgt process is responsible for handling input/output (IO) operations and requires intensive polling. As a result, it consumes 100% of a dedicated CPU core to efficiently manage and process the IO requests, ensuring optimal performance and responsiveness for storage operations. \n\n",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "false",
	}

	SettingDefinitionV2DataEngineHugepageLimit = SettingDefinition{
		DisplayName: "Hugepage Size for V2 Data Engine",
		Description: "Hugepage size in MiB for v2 data engine",
		Category:    SettingCategoryV2DataEngine,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    true,
		Default:     "2048",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 0,
		},
	}

	SettingDefinitionV2DataEngineGuaranteedInstanceManagerCPU = SettingDefinition{
		DisplayName: "Guaranteed Instance Manager CPU for V2 Data Engine",
		Description: "Number of millicpus on each node to be reserved for each instance manager pod when the V2 Data Engine is enabled. The Storage Performance Development Kit (SPDK) target daemon within each instance manager pod uses 1 or multiple CPU cores. Configuring a minimum CPU usage value is essential for maintaining engine and replica stability, especially during periods of high node workload. \n\n" +
			"WARNING: \n\n" +
			"  - Value 0 means unsetting CPU requests for instance manager pods for v2 data engine. \n\n" +
			"  - This integer value is range from 1000 to 8000. \n\n" +
			"  - After this setting is changed, the v2 instance manager pod using this global setting will be automatically restarted without instances running on the v2 instance manager. \n\n",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeInt,
		Required: true,
		ReadOnly: false,
		Default:  "1250",
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 1000,
			ValueIntRangeMaximum: 8000,
		},
	}

	SettingDefinitionV2DataEngineCPUMask = SettingDefinition{
		DisplayName: "CPU Mask for V2 Data Engine",
		Description: "CPU cores on which the Storage Performance Development Kit (SPDK) target daemon should run. The SPDK target daemon is located in each Instance Manager pod. Ensure that the number of cores is less than or equal to the guaranteed Instance Manager CPUs for the V2 Data Engine. The default value is 0x1. \n\n",
		Category:    SettingCategoryDangerZone,
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    false,
		Default:     "0x1",
	}

	SettingDefinitionReplicaDiskSoftAntiAffinity = SettingDefinition{
		DisplayName: "Replica Disk Level Soft Anti-Affinity",
		Description: "Allow scheduling on disks with existing healthy replicas of the same volume",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionAllowEmptyNodeSelectorVolume = SettingDefinition{
		DisplayName: "Allow Scheduling Empty Node Selector Volumes To Any Node",
		Description: "Allow replica of the volume without node selector to be scheduled on node with tags, default true",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionAllowEmptyDiskSelectorVolume = SettingDefinition{
		DisplayName: "Allow Scheduling Empty Disk Selector Volumes To Any Disk",
		Description: "Allow replica of the volume without disk selector to be scheduled on disk with tags, default true",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}

	SettingDefinitionDisableSnapshotPurge = SettingDefinition{
		DisplayName: "Disable Snapshot Purge",
		Description: "Temporarily prevent all attempts to purge volume snapshots",
		Category:    SettingCategoryDangerZone,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "false",
	}

	SettingDefinitionV2DataEngineLogLevel = SettingDefinition{
		DisplayName: "V2 Data Engine Log Level",
		Description: "The log level used in SPDK target daemon (spdk_tgt) of V2 Data Engine. Supported values are: Error, Warning, Notice, Info and Debug. By default Notice.",
		Category:    SettingCategoryV2DataEngine,
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    false,
		Default:     "Notice",
		Choices:     []string{"Error", "Warning", "Notice", "Info", "Debug"},
	}

	SettingDefinitionV2DataEngineLogFlags = SettingDefinition{
		DisplayName: "V2 Data Engine Log Flags",
		Description: "The log flags used in SPDK target daemon (spdk_tgt) of V2 Data Engine.",
		Category:    SettingCategoryV2DataEngine,
		Type:        SettingTypeString,
		Required:    false,
		ReadOnly:    false,
		Default:     "",
	}

	SettingDefinitionV2DataEngineFastReplicaRebuilding = SettingDefinition{
		DisplayName: "V2 Data Engine Fast Replica Rebuilding",
		Description: "This setting enables the fast replica rebuilding feature for v2 data engine. It relies on the snapshot checksums, so setting the snapshot-data-integrity to **enable** or **fast-check** is a prerequisite.",
		Category:    SettingCategoryV2DataEngine,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "false",
	}

	SettingDefinitionAutoCleanupSnapshotWhenDeleteBackup = SettingDefinition{
		DisplayName: "Automatically Cleanup Snapshot When Deleting Backup",
		Description: "This setting enables Longhorn to automatically cleanup snapshots when removing backup.",
		Category:    SettingCategorySnapshot,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "false",
	}

	SettingDefinitionDefaultMinNumberOfBackingImageCopies = SettingDefinition{
		DisplayName: "Default Minimum Number of BackingImage Copies",
		Description: "The default minimum number of backing image copies Longhorn maintains",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     strconv.Itoa(DefaultMinNumberOfCopies),
		ValueIntRange: map[string]int{
			ValueIntRangeMinimum: 1,
		},
	}

	SettingDefinitionRWXVolumeFastFailover = SettingDefinition{
		DisplayName: "RWX Volume Fast Failover",
		Description: "Turn on logic to detect and move stale RWX volumes quickly (Experimental)",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "false",
	}
)

type NodeDownPodDeletionPolicy string

const (
	NodeDownPodDeletionPolicyDoNothing                             = NodeDownPodDeletionPolicy("do-nothing") // Kubernetes default behavior
	NodeDownPodDeletionPolicyDeleteStatefulSetPod                  = NodeDownPodDeletionPolicy("delete-statefulset-pod")
	NodeDownPodDeletionPolicyDeleteDeploymentPod                   = NodeDownPodDeletionPolicy("delete-deployment-pod")
	NodeDownPodDeletionPolicyDeleteBothStatefulsetAndDeploymentPod = NodeDownPodDeletionPolicy("delete-both-statefulset-and-deployment-pod")
)

type NodeDrainPolicy string

const (
	NodeDrainPolicyBlockForEviction                      = NodeDrainPolicy("block-for-eviction")
	NodeDrainPolicyBlockForEvictionIfContainsLastReplica = NodeDrainPolicy("block-for-eviction-if-contains-last-replica")
	NodeDrainPolicyBlockIfContainsLastReplica            = NodeDrainPolicy("block-if-contains-last-replica")
	NodeDrainPolicyAllowIfReplicaIsStopped               = NodeDrainPolicy("allow-if-replica-is-stopped")
	NodeDrainPolicyAlwaysAllow                           = NodeDrainPolicy("always-allow")
)

type SystemManagedPodsImagePullPolicy string

const (
	SystemManagedPodsImagePullPolicyNever        = SystemManagedPodsImagePullPolicy("never")
	SystemManagedPodsImagePullPolicyIfNotPresent = SystemManagedPodsImagePullPolicy("if-not-present")
	SystemManagedPodsImagePullPolicyAlways       = SystemManagedPodsImagePullPolicy("always")
)

type CNIAnnotation string

const (
	CNIAnnotationNetworks      = CNIAnnotation("k8s.v1.cni.cncf.io/networks")
	CNIAnnotationNetworkStatus = CNIAnnotation("k8s.v1.cni.cncf.io/network-status")

	// CNIAnnotationNetworksStatus is deprecated since Multus v3.6 and completely removed in v4.0.0.
	// This exists to support older Multus versions.
	// Ref: https://github.com/longhorn/longhorn/issues/6953
	CNIAnnotationNetworksStatus = CNIAnnotation("k8s.v1.cni.cncf.io/networks-status")
)

func ValidateSetting(name, value string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "value %v of settings %v is invalid", value, name)
	}()
	sName := SettingName(name)

	definition, ok := GetSettingDefinition(sName)
	if !ok {
		return fmt.Errorf("setting %v is not supported", sName)
	}
	if definition.Required && value == "" {
		return fmt.Errorf("required setting %v shouldn't be empty", sName)
	}

	if err := validateBool(definition, value); err != nil {
		return errors.Wrapf(err, "failed to validate the setting %v", sName)
	}

	if err := validateInt(definition, value); err != nil {
		return errors.Wrapf(err, "failed to validate the setting %v", sName)
	}

	if err := validateString(sName, definition, value); err != nil {
		return errors.Wrapf(err, "failed to validate the setting %v", sName)
	}

	return nil
}

// isValidChoice checks if the passed value is part of the choices array,
// an empty choices array allows for all values
func isValidChoice(choices []string, value string) bool {
	for _, c := range choices {
		if value == c {
			return true
		}
	}
	return len(choices) == 0
}

func GetCustomizedDefaultSettings(defaultSettingCM *corev1.ConfigMap) (defaultSettings map[string]string, err error) {
	defaultSettingYAMLData := []byte(defaultSettingCM.Data[DefaultSettingYAMLFileName])

	defaultSettings, err = util.GetDataContentFromYAML(defaultSettingYAMLData)
	if err != nil {
		return nil, err
	}

	// won't accept partially valid result
	for name, value := range defaultSettings {
		value = strings.Trim(value, " ")
		definition, exist := GetSettingDefinition(SettingName(name))
		if !exist {
			logrus.Errorf("Customized settings are invalid, will give up using them: undefined setting %v", name)
			defaultSettings = map[string]string{}
			break
		}
		if value == "" {
			continue
		}
		// Make sure the value of boolean setting is always "true" or "false" in Longhorn.
		// Otherwise the Longhorn UI cannot display the boolean setting correctly.
		if definition.Type == SettingTypeBool {
			result, err := strconv.ParseBool(value)
			if err != nil {
				logrus.WithError(err).Errorf("Invalid value %v for the boolean setting %v", value, name)
				defaultSettings = map[string]string{}
				break
			}
			value = strconv.FormatBool(result)
		}
		if err := ValidateSetting(name, value); err != nil {
			logrus.WithError(err).Errorf("Customized settings are invalid, will give up using them: the value of customized setting %v is invalid", name)
			defaultSettings = map[string]string{}
			break
		}
		defaultSettings[name] = value
	}

	// GuaranteedInstanceManagerCPU for v1 data engine
	guaranteedInstanceManagerCPU := SettingDefinitionGuaranteedInstanceManagerCPU.Default
	if defaultSettings[string(SettingNameGuaranteedInstanceManagerCPU)] != "" {
		guaranteedInstanceManagerCPU = defaultSettings[string(SettingNameGuaranteedInstanceManagerCPU)]
	}
	if err := ValidateCPUReservationValues(SettingNameGuaranteedInstanceManagerCPU, guaranteedInstanceManagerCPU); err != nil {
		logrus.WithError(err).Error("Customized settings GuaranteedInstanceManagerCPU is invalid, will give up using it")
		defaultSettings = map[string]string{}
	}

	// GuaranteedInstanceManagerCPU for v2 data engine
	v2DataEngineGuaranteedInstanceManagerCPU := SettingDefinitionV2DataEngineGuaranteedInstanceManagerCPU.Default
	if defaultSettings[string(SettingNameV2DataEngineGuaranteedInstanceManagerCPU)] != "" {
		v2DataEngineGuaranteedInstanceManagerCPU = defaultSettings[string(SettingNameV2DataEngineGuaranteedInstanceManagerCPU)]
	}
	if err := ValidateCPUReservationValues(SettingNameV2DataEngineGuaranteedInstanceManagerCPU, v2DataEngineGuaranteedInstanceManagerCPU); err != nil {
		logrus.WithError(err).Error("Customized settings V2DataEngineGuaranteedInstanceManagerCPU is invalid, will give up using it")
		defaultSettings = map[string]string{}
	}

	return defaultSettings, nil
}

func UnmarshalTolerations(tolerationSetting string) ([]corev1.Toleration, error) {
	tolerations := []corev1.Toleration{}

	tolerationSetting = strings.ReplaceAll(tolerationSetting, " ", "")
	if tolerationSetting == "" {
		return tolerations, nil
	}

	tolerationList := strings.Split(tolerationSetting, ";")
	for _, toleration := range tolerationList {
		toleration, err := parseToleration(toleration)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse toleration: %s", toleration)
		}
		tolerations = append(tolerations, *toleration)
	}
	return tolerations, nil
}

func parseToleration(taintToleration string) (*corev1.Toleration, error) {
	// The schema should be `key=value:effect` or `key:effect`
	parts := strings.Split(taintToleration, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("missing key/value and effect pair")
	}

	// parse `key=value` or `key`
	key, value, operator := "", "", corev1.TolerationOperator("")
	pair := strings.Split(parts[0], "=")
	switch len(pair) {
	case 1:
		key, value, operator = parts[0], "", corev1.TolerationOpExists
	case 2:
		key, value, operator = pair[0], pair[1], corev1.TolerationOpEqual
	}

	effect := corev1.TaintEffect(parts[1])
	switch effect {
	case "", corev1.TaintEffectNoExecute, corev1.TaintEffectNoSchedule, corev1.TaintEffectPreferNoSchedule:
	default:
		return nil, fmt.Errorf("invalid effect: %v", parts[1])
	}

	return &corev1.Toleration{
		Key:      key,
		Value:    value,
		Operator: operator,
		Effect:   effect,
	}, nil
}

func validateAndUnmarshalLabel(label string) (key, value string, err error) {
	label = strings.Trim(label, " ")
	parts := strings.Split(label, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid label %v: should contain the separator ':'", label)
	}
	return strings.Trim(parts[0], " "), strings.Trim(parts[1], " "), nil
}

func UnmarshalNodeSelector(nodeSelectorSetting string) (map[string]string, error) {
	nodeSelector := map[string]string{}

	nodeSelectorSetting = strings.Trim(nodeSelectorSetting, " ")
	if nodeSelectorSetting != "" {
		labelList := strings.Split(nodeSelectorSetting, ";")
		for _, label := range labelList {
			key, value, err := validateAndUnmarshalLabel(label)
			if err != nil {
				return nil, errors.Wrap(err, "Error while unmarshal node selector")
			}
			nodeSelector[key] = value
		}
	}
	return nodeSelector, nil
}

// GetSettingDefinition gets the setting definition in `settingDefinitions` by the parameter `name`
func GetSettingDefinition(name SettingName) (SettingDefinition, bool) {
	settingDefinitionsLock.RLock()
	defer settingDefinitionsLock.RUnlock()
	setting, ok := settingDefinitions[name]
	return setting, ok
}

// SetSettingDefinition sets the setting definition in `settingDefinitions` by the parameter `name` and `definition`
func SetSettingDefinition(name SettingName, definition SettingDefinition) {
	settingDefinitionsLock.Lock()
	defer settingDefinitionsLock.Unlock()
	settingDefinitions[name] = definition
}

func GetDangerZoneSettings() sets.Set[SettingName] {
	settingList := sets.New[SettingName]()
	for settingName, setting := range settingDefinitions {
		if setting.Category == SettingCategoryDangerZone {
			settingList = settingList.Insert(settingName)
		}
	}

	return settingList
}

func validateBool(definition SettingDefinition, value string) error {
	if definition.Type != SettingTypeBool {
		return nil
	}

	if value != "true" && value != "false" {
		return fmt.Errorf("value %v should be true or false", value)
	}
	return nil
}

func validateInt(definition SettingDefinition, value string) error {
	if definition.Type != SettingTypeInt {
		return nil
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return errors.Wrapf(err, "value %v is not a number", value)
	}

	valueIntRange := definition.ValueIntRange
	if minValue, exists := valueIntRange[ValueIntRangeMinimum]; exists {
		if intValue < minValue {
			return fmt.Errorf("value %v should be larger than %v", intValue, minValue)
		}
	}

	if maxValue, exists := valueIntRange[ValueIntRangeMaximum]; exists {
		if intValue > maxValue {
			return fmt.Errorf("value %v should be less than %v", intValue, maxValue)
		}
	}
	return nil
}

func validateString(sName SettingName, definition SettingDefinition, value string) error {
	if definition.Type != SettingTypeString {
		return nil
	}

	// multi-choices
	if len(definition.Choices) > 0 {
		if !isValidChoice(definition.Choices, value) {
			return fmt.Errorf("value %v is not a valid choice, available choices %v", value, definition.Choices)
		}
		return nil
	}

	switch sName {
	case SettingNameSnapshotDataIntegrityCronJob:
		schedule, err := cron.ParseStandard(value)
		if err != nil {
			return errors.Wrapf(err, "invalid cron job format: %v", value)
		}

		runAt := schedule.Next(time.Unix(0, 0))
		nextRunAt := schedule.Next(runAt)

		logrus.Infof("The interval between two data integrity checks is %v seconds", nextRunAt.Sub(runAt).Seconds())

	case SettingNameTaintToleration:
		if _, err := UnmarshalTolerations(value); err != nil {
			return errors.Wrapf(err, "the value of %v is invalid", sName)
		}
	case SettingNameSystemManagedComponentsNodeSelector:
		if _, err := UnmarshalNodeSelector(value); err != nil {
			return errors.Wrapf(err, "the value of %v is invalid", sName)
		}

	case SettingNameStorageNetwork:
		if err := ValidateStorageNetwork(value); err != nil {
			return errors.Wrapf(err, "the value of %v is invalid", sName)
		}

	case SettingNameV2DataEngineLogFlags:
		if err := ValidateV2DataEngineLogFlags(value); err != nil {
			return errors.Wrapf(err, "failed to validate v2 data engine log flags %v", value)
		}
	}

	return nil
}
