package types

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	v1 "k8s.io/api/core/v1"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/meta"
	"github.com/longhorn/longhorn-manager/util"
)

const (
	DefaultSettingYAMLFileName = "default-setting.yaml"
)

type SettingType string

const (
	SettingTypeString     = SettingType("string")
	SettingTypeInt        = SettingType("int")
	SettingTypeBool       = SettingType("bool")
	SettingTypeDeprecated = SettingType("deprecated")
)

type SettingName string

const (
	SettingNameBackupTarget                                 = SettingName("backup-target")
	SettingNameBackupTargetCredentialSecret                 = SettingName("backup-target-credential-secret")
	SettingNameAllowRecurringJobWhileVolumeDetached         = SettingName("allow-recurring-job-while-volume-detached")
	SettingNameCreateDefaultDiskLabeledNodes                = SettingName("create-default-disk-labeled-nodes")
	SettingNameDefaultDataPath                              = SettingName("default-data-path")
	SettingNameDefaultEngineImage                           = SettingName("default-engine-image")
	SettingNameDefaultInstanceManagerImage                  = SettingName("default-instance-manager-image")
	SettingNameDefaultShareManagerImage                     = SettingName("default-share-manager-image")
	SettingNameDefaultBackingImageManagerImage              = SettingName("default-backing-image-manager-image")
	SettingNameReplicaSoftAntiAffinity                      = SettingName("replica-soft-anti-affinity")
	SettingNameReplicaAutoBalance                           = SettingName("replica-auto-balance")
	SettingNameStorageOverProvisioningPercentage            = SettingName("storage-over-provisioning-percentage")
	SettingNameStorageMinimalAvailablePercentage            = SettingName("storage-minimal-available-percentage")
	SettingNameUpgradeChecker                               = SettingName("upgrade-checker")
	SettingNameCurrentLonghornVersion                       = SettingName("current-longhorn-version")
	SettingNameLatestLonghornVersion                        = SettingName("latest-longhorn-version")
	SettingNameStableLonghornVersions                       = SettingName("stable-longhorn-versions")
	SettingNameDefaultReplicaCount                          = SettingName("default-replica-count")
	SettingNameDefaultDataLocality                          = SettingName("default-data-locality")
	SettingNameGuaranteedEngineCPU                          = SettingName("guaranteed-engine-cpu")
	SettingNameDefaultLonghornStaticStorageClass            = SettingName("default-longhorn-static-storage-class")
	SettingNameBackupstorePollInterval                      = SettingName("backupstore-poll-interval")
	SettingNameTaintToleration                              = SettingName("taint-toleration")
	SettingNameSystemManagedComponentsNodeSelector          = SettingName("system-managed-components-node-selector")
	SettingNameCRDAPIVersion                                = SettingName("crd-api-version")
	SettingNameAutoSalvage                                  = SettingName("auto-salvage")
	SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly  = SettingName("auto-delete-pod-when-volume-detached-unexpectedly")
	SettingNameRegistrySecret                               = SettingName("registry-secret")
	SettingNameDisableSchedulingOnCordonedNode              = SettingName("disable-scheduling-on-cordoned-node")
	SettingNameReplicaZoneSoftAntiAffinity                  = SettingName("replica-zone-soft-anti-affinity")
	SettingNameNodeDownPodDeletionPolicy                    = SettingName("node-down-pod-deletion-policy")
	SettingNameAllowNodeDrainWithLastHealthyReplica         = SettingName("allow-node-drain-with-last-healthy-replica")
	SettingNameMkfsExt4Parameters                           = SettingName("mkfs-ext4-parameters")
	SettingNamePriorityClass                                = SettingName("priority-class")
	SettingNameDisableRevisionCounter                       = SettingName("disable-revision-counter")
	SettingNameDisableReplicaRebuild                        = SettingName("disable-replica-rebuild")
	SettingNameReplicaReplenishmentWaitInterval             = SettingName("replica-replenishment-wait-interval")
	SettingNameConcurrentReplicaRebuildPerNodeLimit         = SettingName("concurrent-replica-rebuild-per-node-limit")
	SettingNameSystemManagedPodsImagePullPolicy             = SettingName("system-managed-pods-image-pull-policy")
	SettingNameAllowVolumeCreationWithDegradedAvailability  = SettingName("allow-volume-creation-with-degraded-availability")
	SettingNameAutoCleanupSystemGeneratedSnapshot           = SettingName("auto-cleanup-system-generated-snapshot")
	SettingNameConcurrentAutomaticEngineUpgradePerNodeLimit = SettingName("concurrent-automatic-engine-upgrade-per-node-limit")
	SettingNameBackingImageCleanupWaitInterval              = SettingName("backing-image-cleanup-wait-interval")
	SettingNameBackingImageRecoveryWaitInterval             = SettingName("backing-image-recovery-wait-interval")
	SettingNameGuaranteedEngineManagerCPU                   = SettingName("guaranteed-engine-manager-cpu")
	SettingNameGuaranteedReplicaManagerCPU                  = SettingName("guaranteed-replica-manager-cpu")
	SettingNameKubernetesClusterAutoscalerEnabled           = SettingName("kubernetes-cluster-autoscaler-enabled")
	SettingNameOrphanAutoDeletion                           = SettingName("orphan-auto-deletion")
	SettingNameStorageNetwork                               = SettingName("storage-network")
)

var (
	SettingNameList = []SettingName{
		SettingNameBackupTarget,
		SettingNameBackupTargetCredentialSecret,
		SettingNameAllowRecurringJobWhileVolumeDetached,
		SettingNameCreateDefaultDiskLabeledNodes,
		SettingNameDefaultDataPath,
		SettingNameDefaultEngineImage,
		SettingNameDefaultInstanceManagerImage,
		SettingNameDefaultShareManagerImage,
		SettingNameDefaultBackingImageManagerImage,
		SettingNameReplicaSoftAntiAffinity,
		SettingNameReplicaAutoBalance,
		SettingNameStorageOverProvisioningPercentage,
		SettingNameStorageMinimalAvailablePercentage,
		SettingNameUpgradeChecker,
		SettingNameCurrentLonghornVersion,
		SettingNameLatestLonghornVersion,
		SettingNameStableLonghornVersions,
		SettingNameDefaultReplicaCount,
		SettingNameDefaultDataLocality,
		SettingNameGuaranteedEngineCPU,
		SettingNameDefaultLonghornStaticStorageClass,
		SettingNameBackupstorePollInterval,
		SettingNameTaintToleration,
		SettingNameSystemManagedComponentsNodeSelector,
		SettingNameCRDAPIVersion,
		SettingNameAutoSalvage,
		SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly,
		SettingNameRegistrySecret,
		SettingNameDisableSchedulingOnCordonedNode,
		SettingNameReplicaZoneSoftAntiAffinity,
		SettingNameNodeDownPodDeletionPolicy,
		SettingNameAllowNodeDrainWithLastHealthyReplica,
		SettingNameMkfsExt4Parameters,
		SettingNamePriorityClass,
		SettingNameDisableRevisionCounter,
		SettingNameDisableReplicaRebuild,
		SettingNameReplicaReplenishmentWaitInterval,
		SettingNameConcurrentReplicaRebuildPerNodeLimit,
		SettingNameSystemManagedPodsImagePullPolicy,
		SettingNameAllowVolumeCreationWithDegradedAvailability,
		SettingNameAutoCleanupSystemGeneratedSnapshot,
		SettingNameConcurrentAutomaticEngineUpgradePerNodeLimit,
		SettingNameBackingImageCleanupWaitInterval,
		SettingNameBackingImageRecoveryWaitInterval,
		SettingNameGuaranteedEngineManagerCPU,
		SettingNameGuaranteedReplicaManagerCPU,
		SettingNameKubernetesClusterAutoscalerEnabled,
		SettingNameOrphanAutoDeletion,
		SettingNameStorageNetwork,
	}
)

type SettingCategory string

const (
	SettingCategoryGeneral    = SettingCategory("general")
	SettingCategoryBackup     = SettingCategory("backup")
	SettingCategoryOrphan     = SettingCategory("orphan")
	SettingCategoryScheduling = SettingCategory("scheduling")
	SettingCategoryDangerZone = SettingCategory("danger Zone")
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
}

var settingDefinitionsLock sync.RWMutex

var (
	settingDefinitions = map[SettingName]SettingDefinition{
		SettingNameBackupTarget:                                 SettingDefinitionBackupTarget,
		SettingNameBackupTargetCredentialSecret:                 SettingDefinitionBackupTargetCredentialSecret,
		SettingNameAllowRecurringJobWhileVolumeDetached:         SettingDefinitionAllowRecurringJobWhileVolumeDetached,
		SettingNameCreateDefaultDiskLabeledNodes:                SettingDefinitionCreateDefaultDiskLabeledNodes,
		SettingNameDefaultDataPath:                              SettingDefinitionDefaultDataPath,
		SettingNameDefaultEngineImage:                           SettingDefinitionDefaultEngineImage,
		SettingNameDefaultInstanceManagerImage:                  SettingDefinitionDefaultInstanceManagerImage,
		SettingNameDefaultShareManagerImage:                     SettingDefinitionDefaultShareManagerImage,
		SettingNameDefaultBackingImageManagerImage:              SettingDefinitionDefaultBackingImageManagerImage,
		SettingNameReplicaSoftAntiAffinity:                      SettingDefinitionReplicaSoftAntiAffinity,
		SettingNameReplicaAutoBalance:                           SettingDefinitionReplicaAutoBalance,
		SettingNameStorageOverProvisioningPercentage:            SettingDefinitionStorageOverProvisioningPercentage,
		SettingNameStorageMinimalAvailablePercentage:            SettingDefinitionStorageMinimalAvailablePercentage,
		SettingNameUpgradeChecker:                               SettingDefinitionUpgradeChecker,
		SettingNameCurrentLonghornVersion:                       SettingDefinitionCurrentLonghornVersion,
		SettingNameLatestLonghornVersion:                        SettingDefinitionLatestLonghornVersion,
		SettingNameStableLonghornVersions:                       SettingDefinitionStableLonghornVersions,
		SettingNameDefaultReplicaCount:                          SettingDefinitionDefaultReplicaCount,
		SettingNameDefaultDataLocality:                          SettingDefinitionDefaultDataLocality,
		SettingNameGuaranteedEngineCPU:                          SettingDefinitionGuaranteedEngineCPU,
		SettingNameDefaultLonghornStaticStorageClass:            SettingDefinitionDefaultLonghornStaticStorageClass,
		SettingNameBackupstorePollInterval:                      SettingDefinitionBackupstorePollInterval,
		SettingNameTaintToleration:                              SettingDefinitionTaintToleration,
		SettingNameSystemManagedComponentsNodeSelector:          SettingDefinitionSystemManagedComponentsNodeSelector,
		SettingNameCRDAPIVersion:                                SettingDefinitionCRDAPIVersion,
		SettingNameAutoSalvage:                                  SettingDefinitionAutoSalvage,
		SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly:  SettingDefinitionAutoDeletePodWhenVolumeDetachedUnexpectedly,
		SettingNameRegistrySecret:                               SettingDefinitionRegistrySecret,
		SettingNameDisableSchedulingOnCordonedNode:              SettingDefinitionDisableSchedulingOnCordonedNode,
		SettingNameReplicaZoneSoftAntiAffinity:                  SettingDefinitionReplicaZoneSoftAntiAffinity,
		SettingNameNodeDownPodDeletionPolicy:                    SettingDefinitionNodeDownPodDeletionPolicy,
		SettingNameAllowNodeDrainWithLastHealthyReplica:         SettingDefinitionAllowNodeDrainWithLastHealthyReplica,
		SettingNameMkfsExt4Parameters:                           SettingDefinitionMkfsExt4Parameters,
		SettingNamePriorityClass:                                SettingDefinitionPriorityClass,
		SettingNameDisableRevisionCounter:                       SettingDefinitionDisableRevisionCounter,
		SettingNameDisableReplicaRebuild:                        SettingDefinitionDisableReplicaRebuild,
		SettingNameReplicaReplenishmentWaitInterval:             SettingDefinitionReplicaReplenishmentWaitInterval,
		SettingNameConcurrentReplicaRebuildPerNodeLimit:         SettingDefinitionConcurrentReplicaRebuildPerNodeLimit,
		SettingNameSystemManagedPodsImagePullPolicy:             SettingDefinitionSystemManagedPodsImagePullPolicy,
		SettingNameAllowVolumeCreationWithDegradedAvailability:  SettingDefinitionAllowVolumeCreationWithDegradedAvailability,
		SettingNameAutoCleanupSystemGeneratedSnapshot:           SettingDefinitionAutoCleanupSystemGeneratedSnapshot,
		SettingNameConcurrentAutomaticEngineUpgradePerNodeLimit: SettingDefinitionConcurrentAutomaticEngineUpgradePerNodeLimit,
		SettingNameBackingImageCleanupWaitInterval:              SettingDefinitionBackingImageCleanupWaitInterval,
		SettingNameBackingImageRecoveryWaitInterval:             SettingDefinitionBackingImageRecoveryWaitInterval,
		SettingNameGuaranteedEngineManagerCPU:                   SettingDefinitionGuaranteedEngineManagerCPU,
		SettingNameGuaranteedReplicaManagerCPU:                  SettingDefinitionGuaranteedReplicaManagerCPU,
		SettingNameKubernetesClusterAutoscalerEnabled:           SettingDefinitionKubernetesClusterAutoscalerEnabled,
		SettingNameOrphanAutoDeletion:                           SettingDefinitionOrphanAutoDeletion,
		SettingNameStorageNetwork:                               SettingDefinitionStorageNetwork,
	}

	SettingDefinitionBackupTarget = SettingDefinition{
		DisplayName: "Backup Target",
		Description: "The endpoint used to access the backupstore. NFS and S3 are supported.",
		Category:    SettingCategoryBackup,
		Type:        SettingTypeString,
		Required:    false,
		ReadOnly:    false,
	}

	SettingDefinitionBackupTargetCredentialSecret = SettingDefinition{
		DisplayName: "Backup Target Credential Secret",
		Description: "The name of the Kubernetes secret associated with the backup target.",
		Category:    SettingCategoryBackup,
		Type:        SettingTypeString,
		Required:    false,
		ReadOnly:    false,
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

	SettingDefinitionBackupstorePollInterval = SettingDefinition{
		DisplayName: "Backupstore Poll Interval",
		Description: "In seconds. The backupstore poll interval determines how often Longhorn checks the backupstore for new backups. Set to 0 to disable the polling.",
		Category:    SettingCategoryBackup,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "300",
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
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    true,
	}

	SettingDefinitionDefaultShareManagerImage = SettingDefinition{
		DisplayName: "Default Share Manager Image",
		Description: "The default share manager image used by the manager. Can be changed on the manager starting command line only",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    true,
	}

	SettingDefinitionDefaultBackingImageManagerImage = SettingDefinition{
		DisplayName: "Default Backing Image Manager Image",
		Description: "The default backing image manager image used by the manager. Can be changed on the manager starting command line only",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    true,
		ReadOnly:    true,
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

	SettingDefinitionStorageOverProvisioningPercentage = SettingDefinition{
		DisplayName: "Storage Over Provisioning Percentage",
		Description: "The over-provisioning percentage defines how much storage can be allocated relative to the hard drive's capacity",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "200",
	}

	SettingDefinitionStorageMinimalAvailablePercentage = SettingDefinition{
		DisplayName: "Storage Minimal Available Percentage",
		Description: "If the minimum available disk capacity exceeds the actual percentage of available disk capacity, the disk becomes unschedulable until more space is freed up.",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "25",
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
	}

	SettingDefinitionDefaultDataLocality = SettingDefinition{
		DisplayName: "Default Data Locality",
		Description: "We say a Longhorn volume has data locality if there is a local replica of the volume on the same node as the pod which is using the volume.\n\n" +
			"This setting specifies the default data locality when a volume is created from the Longhorn UI. For Kubernetes configuration, update the `dataLocality` in the StorageClass\n\n" +
			"The available modes are: \n\n" +
			"- **disabled**. This is the default option. There may or may not be a replica on the same node as the attached volume (workload)\n" +
			"- **best-effort**. This option instructs Longhorn to try to keep a replica on the same node as the attached volume (workload). Longhorn will not stop the volume, even if it cannot keep a replica local to the attached volume (workload) due to environment limitation, e.g. not enough disk space, incompatible disk tags, etc.\n",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  string(longhorn.DataLocalityDisabled),
		Choices: []string{
			string(longhorn.DataLocalityDisabled),
			string(longhorn.DataLocalityBestEffort),
		},
	}

	SettingDefinitionGuaranteedEngineCPU = SettingDefinition{
		DisplayName: "Guaranteed Engine CPU (Deprecated)",
		Description: "This setting is replaced by 2 new settings \"Guaranteed Engine Manager CPU\" and \"Guaranteed Replica Manager CPU\" since Longhorn version v1.1.1. \n" +
			"This setting was used to control the CPU requests of all Longhorn Instance Manager pods. \n",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeDeprecated,
		Required: false,
		ReadOnly: true,
		Default:  "",
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
			"Because `kubernetes.io` is used as the key of all Kubernetes default tolerations, it should not be used in the toleration settings.\n\n " +
			"WARNING: DO NOT CHANGE THIS SETTING WITH ATTACHED VOLUMES! ",
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
			"WARNING: DO NOT CHANGE THIS SETTING WITH ATTACHED VOLUMES! \n\n" +
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
		Description: "If enabled, volumes will be automatically salvaged when all the replicas become faulty e.g. due to network disconnection. Longhorn will try to figure out which replica(s) are usable, then use them for the volume.",
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

	SettingDefinitionAllowNodeDrainWithLastHealthyReplica = SettingDefinition{
		DisplayName: "Allow Node Drain with the Last Healthy Replica",
		Description: "By default, Longhorn will block `kubectl drain` action on a node if the node contains the last healthy replica of a volume.\n\n" +
			"If this setting is enabled, Longhorn will **not** block `kubectl drain` action on a node even if the node contains the last healthy replica of a volume.",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeBool,
		Required: true,
		ReadOnly: false,
		Default:  "false",
	}

	SettingDefinitionMkfsExt4Parameters = SettingDefinition{
		DisplayName: "Custom mkfs.ext4 parameters",
		Description: "Allows setting additional filesystem creation parameters for ext4. For older host kernels it might be necessary to disable the optional ext4 metadata_csum feature by specifying `-O ^64bit,^metadata_csum`",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeString,
		Required:    false,
		ReadOnly:    false,
	}
	SettingDefinitionPriorityClass = SettingDefinition{
		DisplayName: "Priority Class",
		Description: "The name of the Priority Class to set on the Longhorn components. This can help prevent Longhorn components from being evicted under Node Pressure. \n" +
			"Longhorn system contains user deployed components (e.g, Longhorn manager, Longhorn driver, Longhorn UI) and system managed components (e.g, instance manager, engine image, CSI driver, etc.) " +
			"Note that this setting only sets Priority Class for system managed components. " +
			"Depending on how you deployed Longhorn, you need to set Priority Class for user deployed components in Helm chart or deployment YAML file. \n" +
			"WARNING: DO NOT CHANGE THIS SETTING WITH ATTACHED VOLUMES.",
		Category: SettingCategoryDangerZone,
		Required: false,
		ReadOnly: false,
	}
	SettingDefinitionDisableRevisionCounter = SettingDefinition{
		DisplayName: "Disable Revision Counter",
		Description: "This setting is only for volumes created by UI. By default, this is false meaning there will be a revision counter file to track every write to the volume. During salvage recovering Longhorn will pick the repica with largest revision counter as candidate to recover the whole volume. If revision counter is disabled, Longhorn will not track every write to the volume. During the salvage recovering, Longhorn will use the 'volume-head-xxx.img' file last modification time and file size to pick the replica candidate to recover the whole volume.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "false",
	}
	SettingDefinitionDisableReplicaRebuild = SettingDefinition{
		DisplayName: "Disable Replica Rebuild",
		Description: "This setting disable replica rebuild cross the whole cluster, eviction and data locality feature won't work if this setting is true. But doesn't have any impact to any current replica rebuild and restore disaster recovery volume.",
		Category:    SettingCategoryDangerZone,
		Type:        SettingTypeDeprecated,
		Required:    true,
		ReadOnly:    false,
		Default:     "false",
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
		Category:    SettingCategoryGeneral,
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
	}

	SettingDefinitionBackingImageCleanupWaitInterval = SettingDefinition{
		DisplayName: "Backing Image Cleanup Wait Interval",
		Description: "In minutes. The interval determines how long Longhorn will wait before cleaning up the backing image file when there is no replica in the disk using it.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeInt,
		Required:    true,
		ReadOnly:    false,
		Default:     "60",
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
	}

	SettingDefinitionGuaranteedEngineManagerCPU = SettingDefinition{
		DisplayName: "Guaranteed Engine Manager CPU",
		Description: "This integer value indicates how many percentage of the total allocatable CPU on each node will be reserved for each engine manager Pod. For example, 10 means 10% of the total CPU on a node will be allocated to each engine manager pod on this node. This will help maintain engine stability during high node workload. \n\n" +
			"In order to prevent unexpected volume engine crash as well as guarantee a relative acceptable IO performance, you can use the following formula to calculate a value for this setting: \n\n" +
			"Guaranteed Engine Manager CPU = The estimated max Longhorn volume engine count on a node * 0.1 / The total allocatable CPUs on the node * 100. \n\n" +
			"The result of above calculation doesn't mean that's the maximum CPU resources the Longhorn workloads require. To fully exploit the Longhorn volume I/O performance, you can allocate/guarantee more CPU resources via this setting. \n\n" +
			"If it's hard to estimate the usage now, you can leave it with the default value, which is 12%. Then you can tune it when there is no running workload using Longhorn volumes. \n\n" +
			"WARNING: \n\n" +
			"  - Value 0 means unsetting CPU requests for engine manager pods. \n\n" +
			"  - Considering the possible new instance manager pods in the further system upgrade, this integer value is range from 0 to 40. And the sum with setting 'Guaranteed Engine Manager CPU' should not be greater than 40. \n\n" +
			"  - One more set of instance manager pods may need to be deployed when the Longhorn system is upgraded. If current available CPUs of the nodes are not enough for the new instance manager pods, you need to detach the volumes using the oldest instance manager pods so that Longhorn can clean up the old pods automatically and release the CPU resources. And the new pods with the latest instance manager image will be launched then. \n\n" +
			"  - This global setting will be ignored for a node if the field \"EngineManagerCPURequest\" on the node is set. \n\n" +
			"  - After this setting is changed, all engine manager pods using this global setting on all the nodes will be automatically restarted. In other words, DO NOT CHANGE THIS SETTING WITH ATTACHED VOLUMES. \n\n",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  "12",
	}

	SettingDefinitionGuaranteedReplicaManagerCPU = SettingDefinition{
		DisplayName: "Guaranteed Replica Manager CPU",
		Description: "This integer value indicates how many percentage of the total allocatable CPU on each node will be reserved for each replica manager Pod. 10 means 10% of the total CPU on a node will be allocated to each replica manager pod on this node. This will help maintain replica stability during high node workload. \n\n" +
			"In order to prevent unexpected volume replica crash as well as guarantee a relative acceptable IO performance, you can use the following formula to calculate a value for this setting: \n\n" +
			"Guaranteed Replica Manager CPU = The estimated max Longhorn volume replica count on a node * 0.1 / The total allocatable CPUs on the node * 100. \n\n" +
			"The result of above calculation doesn't mean that's the maximum CPU resources the Longhorn workloads require. To fully exploit the Longhorn volume I/O performance, you can allocate/guarantee more CPU resources via this setting. \n\n" +
			"If it's hard to estimate the usage now, you can leave it with the default value, which is 12%. Then you can tune it when there is no running workload using Longhorn volumes. \n\n" +
			"WARNING: \n\n" +
			"  - Value 0 means unsetting CPU requests for replica manager pods. \n\n" +
			"  - Considering the possible new instance manager pods in the further system upgrade, this integer value is range from 0 to 40. And the sum with setting 'Guaranteed Replica Manager CPU' should not be greater than 40. \n\n" +
			"  - One more set of instance manager pods may need to be deployed when the Longhorn system is upgraded. If current available CPUs of the nodes are not enough for the new instance manager pods, you need to detach the volumes using the oldest instance manager pods so that Longhorn can clean up the old pods automatically and release the CPU resources. And the new pods with the latest instance manager image will be launched then. \n\n" +
			"  - This global setting will be ignored for a node if the field \"ReplicaManagerCPURequest\" on the node is set. \n\n" +
			"  - After this setting is changed, all replica manager pods using this global setting on all the nodes will be automatically restarted. In other words, DO NOT CHANGE THIS SETTING WITH ATTACHED VOLUMES. \n\n",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  "12",
	}

	SettingDefinitionKubernetesClusterAutoscalerEnabled = SettingDefinition{
		DisplayName: "Kubernetes Cluster Autoscaler Enabled (Experimental)",
		Description: "Enabling this setting will notify Longhorn that the cluster is using Kubernetes Cluster Autoscaler. \n\n" +
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
			"WARNING: \n\n" +
			"  - The cluster must have pre-existing Multus installed, and NetworkAttachmentDefinition IPs are reachable between nodes. \n\n" +
			"  - DO NOT CHANGE THIS SETTING WITH ATTACHED VOLUMES. Longhorn will try to block this setting update when there are attached volumes. \n\n" +
			"  - When applying the setting, Longhorn will restart all instance-manager, and backing-image-manager pods. \n\n",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeString,
		Required: false,
		ReadOnly: false,
		Default:  CniNetworkNone,
	}
)

type NodeDownPodDeletionPolicy string

const (
	NodeDownPodDeletionPolicyDoNothing                             = NodeDownPodDeletionPolicy("do-nothing") // Kubernetes default behavior
	NodeDownPodDeletionPolicyDeleteStatefulSetPod                  = NodeDownPodDeletionPolicy("delete-statefulset-pod")
	NodeDownPodDeletionPolicyDeleteDeploymentPod                   = NodeDownPodDeletionPolicy("delete-deployment-pod")
	NodeDownPodDeletionPolicyDeleteBothStatefulsetAndDeploymentPod = NodeDownPodDeletionPolicy("delete-both-statefulset-and-deployment-pod")
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
	CNIAnnotationNetworkStatus = CNIAnnotation("k8s.v1.cni.cncf.io/networks-status")
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

	switch sName {
	case SettingNameBackupTarget:
		// check whether have $ or , have been set in BackupTarget
		regStr := `[\$\,]`
		reg := regexp.MustCompile(regStr)
		findStr := reg.FindAllString(value, -1)
		if len(findStr) != 0 {
			return fmt.Errorf("value %s, contains %v", value, strings.Join(findStr, " or "))
		}

	// boolean
	case SettingNameCreateDefaultDiskLabeledNodes:
		fallthrough
	case SettingNameAllowRecurringJobWhileVolumeDetached:
		fallthrough
	case SettingNameReplicaSoftAntiAffinity:
		fallthrough
	case SettingNameDisableSchedulingOnCordonedNode:
		fallthrough
	case SettingNameReplicaZoneSoftAntiAffinity:
		fallthrough
	case SettingNameAllowNodeDrainWithLastHealthyReplica:
		fallthrough
	case SettingNameAllowVolumeCreationWithDegradedAvailability:
		fallthrough
	case SettingNameAutoCleanupSystemGeneratedSnapshot:
		fallthrough
	case SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly:
		fallthrough
	case SettingNameKubernetesClusterAutoscalerEnabled:
		fallthrough
	case SettingNameOrphanAutoDeletion:
		fallthrough
	case SettingNameUpgradeChecker:
		if value != "true" && value != "false" {
			return fmt.Errorf("value %v of setting %v should be true or false", value, sName)
		}

	case SettingNameStorageOverProvisioningPercentage:
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("value %v is not a number", value)
		}
		// additional check whether over provisioning percentage is positive
		value, err := util.ConvertSize(value)
		if err != nil || value < 0 {
			return fmt.Errorf("value %v should be positive", value)
		}
	case SettingNameStorageMinimalAvailablePercentage:
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("value %v is not a number", value)
		}
		// additional check whether minimal available percentage is between 0 to 100
		value, err := util.ConvertSize(value)
		if err != nil || value < 0 || value > 100 {
			return fmt.Errorf("value %v should between 0 to 100", value)
		}
	case SettingNameDefaultReplicaCount:
		c, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("value %v is not int: %v", SettingNameDefaultReplicaCount, err)
		}
		if err := ValidateReplicaCount(c); err != nil {
			return fmt.Errorf("value %v: %v", c, err)
		}
	case SettingNameReplicaAutoBalance:
		if err := ValidateReplicaAutoBalance(longhorn.ReplicaAutoBalance(value)); err != nil {
			return fmt.Errorf("value %v: %v", value, err)
		}
	case SettingNameGuaranteedEngineCPU:
		if value != "" {
			return fmt.Errorf("cannot set a value %v for the deprecated setting %v", value, sName)
		}
	case SettingNameBackingImageCleanupWaitInterval:
		fallthrough
	case SettingNameBackingImageRecoveryWaitInterval:
		fallthrough
	case SettingNameReplicaReplenishmentWaitInterval:
		fallthrough
	case SettingNameConcurrentReplicaRebuildPerNodeLimit:
		fallthrough
	case SettingNameConcurrentAutomaticEngineUpgradePerNodeLimit:
		fallthrough
	case SettingNameBackupstorePollInterval:
		interval, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("value is not int: %v", err)
		}
		if interval < 0 {
			return fmt.Errorf("the value %v shouldn't be less than 0", value)
		}
	case SettingNameTaintToleration:
		if _, err = UnmarshalTolerations(value); err != nil {
			return fmt.Errorf("the value of %v is invalid: %v", sName, err)
		}
	case SettingNameSystemManagedComponentsNodeSelector:
		if _, err = UnmarshalNodeSelector(value); err != nil {
			return fmt.Errorf("the value of %v is invalid: %v", sName, err)
		}
	case SettingNameStorageNetwork:
		if err = ValidateStorageNetwork(value); err != nil {
			return fmt.Errorf("the value of %v is invalid: %v", sName, err)
		}

	// multi-choices
	case SettingNameNodeDownPodDeletionPolicy:
		fallthrough
	case SettingNameDefaultDataLocality:
		fallthrough
	case SettingNameSystemManagedPodsImagePullPolicy:
		definition, _ := GetSettingDefinition(sName)
		choices := definition.Choices
		if !isValidChoice(choices, value) {
			return fmt.Errorf("value %v is not a valid choice, available choices %v", value, choices)
		}
	case SettingNameGuaranteedEngineManagerCPU:
		fallthrough
	case SettingNameGuaranteedReplicaManagerCPU:
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("guaranteed engine/replica cpu value %v is not a valid integer: %v", value, err)
		}
		if i < 0 || i > 40 {
			return fmt.Errorf("guaranteed engine/replica cpu value %v should be between 0 to 40", value)
		}
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

func GetCustomizedDefaultSettings(defaultSettingCM *v1.ConfigMap) (defaultSettings map[string]string, err error) {
	defaultSettingYAMLData := []byte(defaultSettingCM.Data[DefaultSettingYAMLFileName])

	defaultSettings, err = getDefaultSettingFromYAML(defaultSettingYAMLData)
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
				logrus.Errorf("Invalid value %v for the boolean setting %v: %v", value, name, err)
				defaultSettings = map[string]string{}
				break
			}
			value = strconv.FormatBool(result)
		}
		if err := ValidateSetting(name, value); err != nil {
			logrus.Errorf("Customized settings are invalid, will give up using them: the value of customized setting %v is invalid: %v", name, err)
			defaultSettings = map[string]string{}
			break
		}
		defaultSettings[name] = value
	}

	guaranteedEngineManagerCPU := SettingDefinitionGuaranteedEngineManagerCPU.Default
	if defaultSettings[string(SettingNameGuaranteedEngineManagerCPU)] != "" {
		guaranteedEngineManagerCPU = defaultSettings[string(SettingNameGuaranteedEngineManagerCPU)]
	}
	guaranteedReplicaManagerCPU := SettingDefinitionGuaranteedReplicaManagerCPU.Default
	if defaultSettings[string(SettingNameGuaranteedReplicaManagerCPU)] != "" {
		guaranteedReplicaManagerCPU = defaultSettings[string(SettingNameGuaranteedReplicaManagerCPU)]
	}
	if err := ValidateCPUReservationValues(guaranteedEngineManagerCPU, guaranteedReplicaManagerCPU); err != nil {
		logrus.Errorf("Customized settings GuaranteedEngineManagerCPU and GuaranteedReplicaManagerCPU are invalid, will give up using them: %v", err)
		defaultSettings = map[string]string{}
	}

	return defaultSettings, nil
}

func getDefaultSettingFromYAML(defaultSettingYAMLData []byte) (map[string]string, error) {
	defaultSettings := map[string]string{}

	if err := yaml.Unmarshal(defaultSettingYAMLData, &defaultSettings); err != nil {
		logrus.Errorf("Failed to unmarshal customized default settings from yaml data %v, will give up using them: %v",
			string(defaultSettingYAMLData), err)
		defaultSettings = map[string]string{}
	}

	return defaultSettings, nil
}

func ValidateAndUnmarshalToleration(s string) (*v1.Toleration, error) {
	toleration := &v1.Toleration{}

	// The schema should be `key=value:effect` or `key:effect`
	s = strings.Trim(s, " ")
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid toleration setting %v: should contain both effect and key/value pair", s)
	}

	effect := v1.TaintEffect(strings.Trim(parts[1], " "))
	if effect != v1.TaintEffectNoExecute && effect != v1.TaintEffectNoSchedule && effect != v1.TaintEffectPreferNoSchedule && effect != v1.TaintEffect("") {
		return nil, fmt.Errorf("invalid toleration setting %v: invalid effect", parts[1])
	}
	toleration.Effect = effect

	if strings.Contains(parts[0], "=") {
		pair := strings.Split(parts[0], "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid toleration setting %v: invalid key/value pair", parts[0])
		}
		toleration.Key = strings.Trim(pair[0], " ")
		toleration.Value = strings.Trim(pair[1], " ")
		toleration.Operator = v1.TolerationOpEqual
	} else {
		toleration.Key = strings.Trim(parts[0], " ")
		toleration.Operator = v1.TolerationOpExists
	}

	return toleration, nil
}

func UnmarshalTolerations(tolerationSetting string) ([]v1.Toleration, error) {
	res := []v1.Toleration{}

	tolerationSetting = strings.Trim(tolerationSetting, " ")
	if tolerationSetting != "" {
		tolerationList := strings.Split(tolerationSetting, ";")
		for _, t := range tolerationList {
			toleration, err := ValidateAndUnmarshalToleration(t)
			if err != nil {
				return nil, err
			}
			res = append(res, *toleration)
		}
	}

	return res, nil
}

func validateAndUnmarshalLabel(label string) (key, value string, err error) {
	label = strings.Trim(label, " ")
	parts := strings.Split(label, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid label %v: should contain the seprator ':'", label)
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

func GetSettingDefinition(name SettingName) (SettingDefinition, bool) {
	settingDefinitionsLock.RLock()
	defer settingDefinitionsLock.RUnlock()
	setting, ok := settingDefinitions[name]
	return setting, ok
}

func SetSettingDefinition(name SettingName, definition SettingDefinition) {
	settingDefinitionsLock.Lock()
	defer settingDefinitionsLock.Unlock()
	settingDefinitions[name] = definition
}
