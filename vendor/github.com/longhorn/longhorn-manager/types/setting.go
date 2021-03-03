package types

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/longhorn/longhorn-manager/util"
)

const (
	EnvDefaultSettingPath = "DEFAULT_SETTING_PATH"
)

type Setting struct {
	Value string `json:"value"`
}

type SettingType string

const (
	SettingTypeString = SettingType("string")
	SettingTypeInt    = SettingType("int")
	SettingTypeBool   = SettingType("bool")
)

type SettingName string

const (
	SettingNameBackupTarget                                = SettingName("backup-target")
	SettingNameBackupTargetCredentialSecret                = SettingName("backup-target-credential-secret")
	SettingNameAllowRecurringJobWhileVolumeDetached        = SettingName("allow-recurring-job-while-volume-detached")
	SettingNameCreateDefaultDiskLabeledNodes               = SettingName("create-default-disk-labeled-nodes")
	SettingNameDefaultDataPath                             = SettingName("default-data-path")
	SettingNameDefaultEngineImage                          = SettingName("default-engine-image")
	SettingNameDefaultInstanceManagerImage                 = SettingName("default-instance-manager-image")
	SettingNameDefaultShareManagerImage                    = SettingName("default-share-manager-image")
	SettingNameReplicaSoftAntiAffinity                     = SettingName("replica-soft-anti-affinity")
	SettingNameStorageOverProvisioningPercentage           = SettingName("storage-over-provisioning-percentage")
	SettingNameStorageMinimalAvailablePercentage           = SettingName("storage-minimal-available-percentage")
	SettingNameUpgradeChecker                              = SettingName("upgrade-checker")
	SettingNameLatestLonghornVersion                       = SettingName("latest-longhorn-version")
	SettingNameDefaultReplicaCount                         = SettingName("default-replica-count")
	SettingNameDefaultDataLocality                         = SettingName("default-data-locality")
	SettingNameGuaranteedEngineCPU                         = SettingName("guaranteed-engine-cpu")
	SettingNameDefaultLonghornStaticStorageClass           = SettingName("default-longhorn-static-storage-class")
	SettingNameBackupstorePollInterval                     = SettingName("backupstore-poll-interval")
	SettingNameTaintToleration                             = SettingName("taint-toleration")
	SettingNameCRDAPIVersion                               = SettingName("crd-api-version")
	SettingNameAutoSalvage                                 = SettingName("auto-salvage")
	SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly = SettingName("auto-delete-pod-when-volume-detached-unexpectedly")
	SettingNameRegistrySecret                              = SettingName("registry-secret")
	SettingNameDisableSchedulingOnCordonedNode             = SettingName("disable-scheduling-on-cordoned-node")
	SettingNameReplicaZoneSoftAntiAffinity                 = SettingName("replica-zone-soft-anti-affinity")
	SettingNameVolumeAttachmentRecoveryPolicy              = SettingName("volume-attachment-recovery-policy")
	SettingNameNodeDownPodDeletionPolicy                   = SettingName("node-down-pod-deletion-policy")
	SettingNameAllowNodeDrainWithLastHealthyReplica        = SettingName("allow-node-drain-with-last-healthy-replica")
	SettingNameMkfsExt4Parameters                          = SettingName("mkfs-ext4-parameters")
	SettingNamePriorityClass                               = SettingName("priority-class")
	SettingNameDisableRevisionCounter                      = SettingName("disable-revision-counter")
	SettingNameDisableReplicaRebuild                       = SettingName("disable-replica-rebuild")
	SettingNameReplicaReplenishmentWaitInterval            = SettingName("replica-replenishment-wait-interval")
	SettingNameSystemManagedPodsImagePullPolicy            = SettingName("system-managed-pods-image-pull-policy")
	SettingNameAllowVolumeCreationWithDegradedAvailability = SettingName("allow-volume-creation-with-degraded-availability")
	SettingNameAutoCleanupSystemGeneratedSnapshot          = SettingName("auto-cleanup-system-generated-snapshot")
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
		SettingNameReplicaSoftAntiAffinity,
		SettingNameStorageOverProvisioningPercentage,
		SettingNameStorageMinimalAvailablePercentage,
		SettingNameUpgradeChecker,
		SettingNameLatestLonghornVersion,
		SettingNameDefaultReplicaCount,
		SettingNameDefaultDataLocality,
		SettingNameGuaranteedEngineCPU,
		SettingNameDefaultLonghornStaticStorageClass,
		SettingNameBackupstorePollInterval,
		SettingNameTaintToleration,
		SettingNameCRDAPIVersion,
		SettingNameAutoSalvage,
		SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly,
		SettingNameRegistrySecret,
		SettingNameDisableSchedulingOnCordonedNode,
		SettingNameReplicaZoneSoftAntiAffinity,
		SettingNameVolumeAttachmentRecoveryPolicy,
		SettingNameNodeDownPodDeletionPolicy,
		SettingNameAllowNodeDrainWithLastHealthyReplica,
		SettingNameMkfsExt4Parameters,
		SettingNamePriorityClass,
		SettingNameDisableRevisionCounter,
		SettingNameDisableReplicaRebuild,
		SettingNameReplicaReplenishmentWaitInterval,
		SettingNameSystemManagedPodsImagePullPolicy,
		SettingNameAllowVolumeCreationWithDegradedAvailability,
		SettingNameAutoCleanupSystemGeneratedSnapshot,
	}
)

type SettingCategory string

const (
	SettingCategoryGeneral    = SettingCategory("general")
	SettingCategoryBackup     = SettingCategory("backup")
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

var (
	SettingDefinitions = map[SettingName]SettingDefinition{
		SettingNameBackupTarget:                                SettingDefinitionBackupTarget,
		SettingNameBackupTargetCredentialSecret:                SettingDefinitionBackupTargetCredentialSecret,
		SettingNameAllowRecurringJobWhileVolumeDetached:        SettingDefinitionAllowRecurringJobWhileVolumeDetached,
		SettingNameCreateDefaultDiskLabeledNodes:               SettingDefinitionCreateDefaultDiskLabeledNodes,
		SettingNameDefaultDataPath:                             SettingDefinitionDefaultDataPath,
		SettingNameDefaultEngineImage:                          SettingDefinitionDefaultEngineImage,
		SettingNameDefaultInstanceManagerImage:                 SettingDefinitionDefaultInstanceManagerImage,
		SettingNameDefaultShareManagerImage:                    SettingDefinitionDefaultShareManagerImage,
		SettingNameReplicaSoftAntiAffinity:                     SettingDefinitionReplicaSoftAntiAffinity,
		SettingNameStorageOverProvisioningPercentage:           SettingDefinitionStorageOverProvisioningPercentage,
		SettingNameStorageMinimalAvailablePercentage:           SettingDefinitionStorageMinimalAvailablePercentage,
		SettingNameUpgradeChecker:                              SettingDefinitionUpgradeChecker,
		SettingNameLatestLonghornVersion:                       SettingDefinitionLatestLonghornVersion,
		SettingNameDefaultReplicaCount:                         SettingDefinitionDefaultReplicaCount,
		SettingNameDefaultDataLocality:                         SettingDefinitionDefaultDataLocality,
		SettingNameGuaranteedEngineCPU:                         SettingDefinitionGuaranteedEngineCPU,
		SettingNameDefaultLonghornStaticStorageClass:           SettingDefinitionDefaultLonghornStaticStorageClass,
		SettingNameBackupstorePollInterval:                     SettingDefinitionBackupstorePollInterval,
		SettingNameTaintToleration:                             SettingDefinitionTaintToleration,
		SettingNameCRDAPIVersion:                               SettingDefinitionCRDAPIVersion,
		SettingNameAutoSalvage:                                 SettingDefinitionAutoSalvage,
		SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly: SettingDefinitionAutoDeletePodWhenVolumeDetachedUnexpectedly,
		SettingNameRegistrySecret:                              SettingDefinitionRegistrySecret,
		SettingNameDisableSchedulingOnCordonedNode:             SettingDefinitionDisableSchedulingOnCordonedNode,
		SettingNameReplicaZoneSoftAntiAffinity:                 SettingDefinitionReplicaZoneSoftAntiAffinity,
		SettingNameVolumeAttachmentRecoveryPolicy:              SettingDefinitionVolumeAttachmentRecoveryPolicy,
		SettingNameNodeDownPodDeletionPolicy:                   SettingDefinitionNodeDownPodDeletionPolicy,
		SettingNameAllowNodeDrainWithLastHealthyReplica:        SettingDefinitionAllowNodeDrainWithLastHealthyReplica,
		SettingNameMkfsExt4Parameters:                          SettingDefinitionMkfsExt4Parameters,
		SettingNamePriorityClass:                               SettingDefinitionPriorityClass,
		SettingNameDisableRevisionCounter:                      SettingDefinitionDisableRevisionCounter,
		SettingNameDisableReplicaRebuild:                       SettingDefinitionDisableReplicaRebuild,
		SettingNameReplicaReplenishmentWaitInterval:            SettingDefinitionReplicaReplenishmentWaitInterval,
		SettingNameSystemManagedPodsImagePullPolicy:            SettingDefinitionSystemManagedPodsImagePullPolicy,
		SettingNameAllowVolumeCreationWithDegradedAvailability: SettingDefinitionAllowVolumeCreationWithDegradedAvailability,
		SettingNameAutoCleanupSystemGeneratedSnapshot:          SettingDefinitionAutoCleanupSystemGeneratedSnapshot,
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

	SettingDefinitionReplicaSoftAntiAffinity = SettingDefinition{
		DisplayName: "Replica Node Level Soft Anti-Affinity",
		Description: "Allow scheduling on nodes with existing healthy replicas of the same volume",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "false",
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

	SettingDefinitionLatestLonghornVersion = SettingDefinition{
		DisplayName: "Latest Longhorn Version",
		Description: "The latest version of Longhorn available. Updated by Upgrade Checker automatically",
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
		Default:  string(DataLocalityDisabled),
		Choices: []string{
			string(DataLocalityDisabled),
			string(DataLocalityBestEffort),
		},
	}

	SettingDefinitionGuaranteedEngineCPU = SettingDefinition{
		DisplayName: "Guaranteed Engine CPU",
		Description: "Allow Longhorn Instance Managers to have guaranteed CPU allocation. The value is how many CPUs should be reserved for each Engine/Replica Instance Manager Pod created by Longhorn. For example, 0.1 means one-tenth of a CPU. This will help maintain engine stability during high node workload. It only applies to the Engine/Replica Instance Manager Pods created after the setting took effect.  \n" +
			"In order to prevent unexpected volume crash, you can use the following formula to calculate an appropriate value for this setting:  \n" +
			"Guaranteed Engine CPU = The estimated max Longhorn volume/replica count on a node * 0.1  \n" +
			"The result of above calculation doesn't mean that's the maximum CPU resources the Longhorn workloads require. To fully exploit the Longhorn volume I/O performance, you can allocate/guarantee more CPU resources via this setting.  \n" +
			"If it's hard to estimate the volume/replica count now, you can leave it with the default value, or allocate 1/8 of total CPU of a node. Then you can tune it when there is no running workload using Longhorn volumes.  \n" +
			"WARNING: After this setting is changed, all the instance managers on all the nodes will be automatically restarted.  \n" +
			"WARNING: DO NOT CHANGE THIS SETTING WITH ATTACHED VOLUMES.",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  "0.25",
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
		Description: "To dedicate nodes to store Longhorn replicas and reject other general workloads, set tolerations for Longhorn and add taints for the storage nodes. " +
			"All Longhorn volumes should be detached before modifying toleration settings. " +
			"We recommend setting tolerations during Longhorn deployment because the Longhorn system cannot be operated during the update. " +
			"Multiple tolerations can be set here, and these tolerations are separated by semicolon. For example: \n\n" +
			"* `key1=value1:NoSchedule; key2:NoExecute` \n\n" +
			"* `:` this toleration tolerates everything because an empty key with operator `Exists` matches all keys, values and effects \n\n" +
			"* `key1=value1:`  this toleration has empty effect. It matches all effects with key `key1` \n\n" +
			"Because `kubernetes.io` is used as the key of all Kubernetes default tolerations, it should not be used in the toleration settings.\n\n " +
			"WARNING: DO NOT CHANGE THIS SETTING WITH ATTACHED VOLUMES!",
		Category: SettingCategoryDangerZone,
		Type:     SettingTypeString,
		Required: false,
		ReadOnly: false,
	}

	SettingDefinitionCRDAPIVersion = SettingDefinition{
		DisplayName: "Custom Resource API Version",
		Description: "The current customer resource's API version, e.g. longhorn.io/v1beta1. Set by manager automatically",
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
		Description: "Allow scheduling new Replicas of Volume to the Nodes in the same Zone as existing healthy Replicas. Nodes don't belong to any Zone will be treated as in the same Zone.",
		Category:    SettingCategoryScheduling,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}
	SettingDefinitionVolumeAttachmentRecoveryPolicy = SettingDefinition{
		DisplayName: "Volume Attachment Recovery Policy",
		Description: "Defines the Longhorn action when a Volume is stuck with a Deployment Pod on a failed node.\n" +
			"- **wait** leads to the deletion of the volume attachment as soon as the pods deletion time has passed.\n" +
			"- **never** is the default Kubernetes behavior of never deleting volume attachments on terminating pods.\n" +
			"- **immediate** leads to the deletion of the volume attachment as soon as all workload pods are pending.\n",
		Category: SettingCategoryGeneral,
		Type:     SettingTypeString,
		Required: true,
		ReadOnly: false,
		Default:  string(VolumeAttachmentRecoveryPolicyWait),
		Choices: []string{
			string(VolumeAttachmentRecoveryPolicyNever),
			string(VolumeAttachmentRecoveryPolicyWait),
			string(VolumeAttachmentRecoveryPolicyImmediate),
		},
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
		Description: "The name of the Priority Class to set on the Longhorn workloads. This can help prevent Longhorn workloads from being evicted under Node Pressure.\nWARNING: DO NOT CHANGE THIS SETTING WITH ATTACHED VOLUMES.",
		Category:    SettingCategoryDangerZone,
		Required:    false,
		ReadOnly:    false,
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
		Type:        SettingTypeBool,
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
		Description: "This setting enables Longhorn to automatically cleanup the system generated snapshot after replica rebuild is done.",
		Category:    SettingCategoryGeneral,
		Type:        SettingTypeBool,
		Required:    true,
		ReadOnly:    false,
		Default:     "true",
	}
)

type VolumeAttachmentRecoveryPolicy string

const (
	VolumeAttachmentRecoveryPolicyNever     = VolumeAttachmentRecoveryPolicy("never") // Kubernetes default behavior
	VolumeAttachmentRecoveryPolicyWait      = VolumeAttachmentRecoveryPolicy("wait")  // Longhorn default behavior
	VolumeAttachmentRecoveryPolicyImmediate = VolumeAttachmentRecoveryPolicy("immediate")
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

func ValidateInitSetting(name, value string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "value %v of settings %v is invalid", value, name)
	}()
	sName := SettingName(name)

	definition, ok := SettingDefinitions[sName]
	if !ok {
		return fmt.Errorf("setting %v is not supported", sName)
	}
	if definition.Required == true && value == "" {
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
	case SettingNameGuaranteedEngineCPU:
		if _, err := resource.ParseQuantity(value); err != nil {
			return errors.Wrapf(err, "invalid value %v as CPU resource", value)
		}
	case SettingNameReplicaReplenishmentWaitInterval:
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

	// multi-choices
	case SettingNameVolumeAttachmentRecoveryPolicy:
		fallthrough
	case SettingNameNodeDownPodDeletionPolicy:
		fallthrough
	case SettingNameDefaultDataLocality:
		fallthrough
	case SettingNameSystemManagedPodsImagePullPolicy:
		choices := SettingDefinitions[sName].Choices
		if !isValidChoice(choices, value) {
			return fmt.Errorf("value %v is not a valid choice, available choices %v", value, choices)
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

func GetCustomizedDefaultSettings() (map[string]string, error) {
	settingPath := os.Getenv(EnvDefaultSettingPath)
	defaultSettings := map[string]string{}
	if settingPath != "" {
		data, err := ioutil.ReadFile(settingPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read default setting file %v: %v", settingPath, err)
		}

		// `yaml.Unmarshal()` can return a partial result. We shouldn't allow it
		if err := yaml.Unmarshal(data, &defaultSettings); err != nil {
			logrus.Errorf("Failed to unmarshal customized default settings from yaml data %v, will give up using them: %v", string(data), err)
			defaultSettings = map[string]string{}
		}
	}

	// won't accept partially valid result
	for name, value := range defaultSettings {
		value = strings.Trim(value, " ")
		definition, exist := SettingDefinitions[SettingName(name)]
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
		if err := ValidateInitSetting(name, value); err != nil {
			logrus.Errorf("Customized settings are invalid, will give up using them: the value of customized setting %v is invalid: %v", name, err)
			defaultSettings = map[string]string{}
			break
		}
		defaultSettings[name] = value
	}

	return defaultSettings, nil
}

func OverwriteBuiltInSettingsWithCustomizedValues() error {
	logrus.Infof("Start overwriting built-in settings with customized values")
	customizedDefaultSettings, err := GetCustomizedDefaultSettings()
	if err != nil {
		return err
	}

	for _, sName := range SettingNameList {
		definition, ok := SettingDefinitions[sName]
		if !ok {
			return fmt.Errorf("BUG: setting %v is not defined", sName)
		}

		value, exists := customizedDefaultSettings[string(sName)]
		if exists && value != "" {
			definition.Default = value
			SettingDefinitions[sName] = definition
		}
	}

	return nil
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
