package types

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	lhns "github.com/longhorn/go-common-libs/ns"

	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	LonghornKindNode                = "Node"
	LonghornKindVolume              = "Volume"
	LonghornKindVolumeAttachment    = "VolumeAttachment"
	LonghornKindEngine              = "Engine"
	LonghornKindReplica             = "Replica"
	LonghornKindBackupTarget        = "BackupTarget"
	LonghornKindBackupVolume        = "BackupVolume"
	LonghornKindBackup              = "Backup"
	LonghornKindBackupBackingImage  = "BackupBackingImage"
	LonghornKindSnapshot            = "Snapshot"
	LonghornKindEngineImage         = "EngineImage"
	LonghornKindInstanceManager     = "InstanceManager"
	LonghornKindShareManager        = "ShareManager"
	LonghornKindBackingImage        = "BackingImage"
	LonghornKindBackingImageManager = "BackingImageManager"
	LonghornKindRecurringJob        = "RecurringJob"
	LonghornKindSetting             = "Setting"
	LonghornKindSupportBundle       = "SupportBundle"
	LonghornKindSystemBackup        = "SystemBackup"
	LonghornKindSystemRestore       = "SystemRestore"
	LonghornKindOrphan              = "Orphan"

	LonghornKindBackingImageDataSource = "BackingImageDataSource"

	LonghornKindEngineImageList  = "EngineImageList"
	LonghornKindRecurringJobList = "RecurringJobList"
	LonghornKindSettingList      = "SettingList"
	LonghornKindVolumeList       = "VolumeList"
	LonghornKindBackingImageList = "BackingImageList"
	LonghornKindBackupTargetList = "BackupTargetList"

	KubernetesKindClusterRole           = "ClusterRole"
	KubernetesKindClusterRoleBinding    = "ClusterRoleBinding"
	KubernetesKindConfigMap             = "ConfigMap"
	KubernetesKindDaemonSet             = "DaemonSet"
	KubernetesKindDeployment            = "Deployment"
	KubernetesKindJob                   = "Job"
	KubernetesKindPersistentVolume      = "PersistentVolume"
	KubernetesKindPersistentVolumeClaim = "PersistentVolumeClaim"
	KubernetesKindRole                  = "Role"
	KubernetesKindRoleBinding           = "RoleBinding"
	KubernetesKindService               = "Service"
	KubernetesKindServiceAccount        = "ServiceAccount"
	KubernetesKindStorageClass          = "StorageClass"

	KubernetesKindClusterRoleList           = "ClusterRoleList"
	KubernetesKindClusterRoleBindingList    = "ClusterRoleBindingList"
	KubernetesKindConfigMapList             = "ConfigMapList"
	KubernetesKindDaemonSetList             = "DaemonSetList"
	KubernetesKindDeploymentList            = "DeploymentList"
	KubernetesKindPod                       = "Pod"
	KubernetesKindPersistentVolumeList      = "PersistentVolumeList"
	KubernetesKindPersistentVolumeClaimList = "PersistentVolumeClaimList"
	KubernetesKindRoleList                  = "RoleList"
	KubernetesKindRoleBindingList           = "RoleBindingList"
	KubernetesKindServiceList               = "ServiceList"
	KubernetesKindServiceAccountList        = "ServiceAccountList"
	KubernetesKindStorageClassList          = "StorageClassList"

	APIExtensionsKindCustomResourceDefinition = "CustomResourceDefinition"

	APIExtensionsKindCustomResourceDefinitionList = "CustomResourceDefinitionList"

	CRDAPIVersionV1alpha1 = "longhorn.rancher.io/v1alpha1"
	CRDAPIVersionV1beta1  = "longhorn.io/v1beta1"
	CRDAPIVersionV1beta2  = "longhorn.io/v1beta2"
	CurrentCRDAPIVersion  = CRDAPIVersionV1beta2
)

const (
	DefaultAPIPort                   = 9500
	DefaultConversionWebhookPort     = 9501
	DefaultAdmissionWebhookPort      = 9502
	DefaultRecoveryBackendServerPort = 9503

	EngineBinaryDirectoryInContainer = "/engine-binaries/"
	EngineBinaryDirectoryOnHost      = "/var/lib/longhorn/engine-binaries/"
	ReplicaHostPrefix                = "/host"
	EngineBinaryName                 = "longhorn"

	UnixDomainSocketDirectoryInContainer = "/host/var/lib/longhorn/unix-domain-socket/"
	UnixDomainSocketDirectoryOnHost      = "/var/lib/longhorn/unix-domain-socket/"

	BackingImageManagerDirectory = "/backing-images/"
	BackingImageFileName         = "backing"

	TLSDirectoryInContainer = "/tls-files/"
	TLSSecretName           = "longhorn-grpc-tls"
	TLSCAFile               = "ca.crt"
	TLSCertFile             = "tls.crt"
	TLSKeyFile              = "tls.key"

	DefaultBackupTargetName = "default"

	LonghornNodeKey     = "longhornnode"
	LonghornDiskUUIDKey = "longhorndiskuuid"

	NodeCreateDefaultDiskLabelKey             = "node.longhorn.io/create-default-disk"
	NodeCreateDefaultDiskLabelValueTrue       = "true"
	NodeCreateDefaultDiskLabelValueConfig     = "config"
	NodeDisableV2DataEngineLabelKey           = "node.longhorn.io/disable-v2-data-engine"
	NodeDisableV2DataEngineLabelKeyTrue       = "true"
	KubeNodeDefaultDiskConfigAnnotationKey    = "node.longhorn.io/default-disks-config"
	KubeNodeDefaultNodeTagConfigAnnotationKey = "node.longhorn.io/default-node-tags"

	LastAppliedTolerationAnnotationKeySuffix = "last-applied-tolerations"

	ConfigMapResourceVersionKey = "configmap-resource-version"
	UpdateSettingFromLonghorn   = "update-setting-from-longhorn"

	DeleteCustomResourceOnly = "delete-custom-resource-only"

	// const value `DeleteBackupTargetFromLonghorn` is used for annotation to note that deleting backup target is by Longhorn during uninstalling.
	DeleteBackupTargetFromLonghorn = "delete-backup-target-from-longhorn"

	KubernetesStatusLabel = "KubernetesStatus"
	KubernetesReplicaSet  = "ReplicaSet"
	KubernetesStatefulSet = "StatefulSet"
	RecurringJobLabel     = "RecurringJob"

	VolumeRecurringJobInfoLabel     = "VolumeRecurringJobInfo"
	VolumeRecurringJobRestorePrefix = "restored-recurring-job-"

	LonghornLabelKeyPrefix = "longhorn.io"

	LonghornLabelRecurringJobKeyPrefixFmt = "recurring-%s.longhorn.io"
	LonghornLabelVolumeSettingKeyPrefix   = "setting.longhorn.io"

	LonghornLabelEngineImage                = "engine-image"
	LonghornLabelInstanceManager            = "instance-manager"
	LonghornLabelNode                       = "node"
	LonghornLabelDiskUUID                   = "disk-uuid"
	LonghornLabelInstanceManagerType        = "instance-manager-type"
	LonghornLabelInstanceManagerImage       = "instance-manager-image"
	LonghornLabelVolume                     = "longhornvolume"
	LonghornLabelShareManager               = "share-manager"
	LonghornLabelShareManagerImage          = "share-manager-image"
	LonghornLabelShareManagerConfigMap      = "share-manager-configmap"
	LonghornLabelBackingImage               = "backing-image"
	LonghornLabelBackingImageManager        = "backing-image-manager"
	LonghornLabelManagedBy                  = "managed-by"
	LonghornLabelSnapshotForCloningVolume   = "for-cloning-volume"
	LonghornLabelBackingImageDataSource     = "backing-image-data-source"
	LonghornLabelBackupTarget               = "backup-target"
	LonghornLabelBackupVolume               = "backup-volume"
	LonghornLabelRecurringJob               = "job"
	LonghornLabelRecurringJobGroup          = "job-group"
	LonghornLabelRecurringJobSource         = "source"
	LonghornLabelOrphan                     = "orphan"
	LonghornLabelOrphanType                 = "orphan-type"
	LonghornLabelRecoveryBackend            = "recovery-backend"
	LonghornLabelCRDAPIVersion              = "crd-api-version"
	LonghornLabelVolumeAccessMode           = "volume-access-mode"
	LonghornLabelFollowGlobalSetting        = "follow-global-setting"
	LonghornLabelSystemRestore              = "system-restore"
	LonghornLabelLastSkippedSystemRestore   = "last-skipped-system-restored"
	LonghornLabelLastSkippedSystemRestoreAt = "last-skipped-system-restored-at"
	LonghornLabelLastSystemRestore          = "last-system-restored"
	LonghornLabelLastSystemRestoreAt        = "last-system-restored-at"
	LonghornLabelLastSystemRestoreBackup    = "last-system-restored-backup"
	LonghornLabelDataEngine                 = "data-engine"
	LonghornLabelVersion                    = "version"
	LonghornLabelAdmissionWebhook           = "admission-webhook"
	LonghornLabelConversionWebhook          = "conversion-webhook"

	LonghornRecoveryBackendServiceName = "longhorn-recovery-backend"

	LonghornLabelValueEnabled = "enabled"
	LonghornLabelValueIgnored = "ignored"

	LonghornLabelExportFromVolume                 = "export-from-volume"
	LonghornLabelSnapshotForExportingBackingImage = "for-exporting-backing-image"

	KubernetesFailureDomainRegionLabelKey = "failure-domain.beta.kubernetes.io/region"
	KubernetesFailureDomainZoneLabelKey   = "failure-domain.beta.kubernetes.io/zone"
	KubernetesTopologyRegionLabelKey      = "topology.kubernetes.io/region"
	KubernetesTopologyZoneLabelKey        = "topology.kubernetes.io/zone"

	KubernetesClusterAutoscalerSafeToEvictKey = "cluster-autoscaler.kubernetes.io/safe-to-evict"

	LonghornDriverName = "driver.longhorn.io"

	DefaultDiskPrefix = "default-disk-"

	DeprecatedProvisionerName           = "rancher.io/longhorn"
	DepracatedDriverName                = "io.rancher.longhorn"
	DefaultStorageClassConfigMapName    = "longhorn-storageclass"
	DefaultDefaultSettingConfigMapName  = "longhorn-default-setting"
	DefaultDefaultResourceConfigMapName = "longhorn-default-resource"
	DefaultStorageClassName             = "longhorn"
	ControlPlaneName                    = "longhorn-manager"

	DefaultRecurringJobConcurrency = 10

	PVAnnotationLonghornVolumeSchedulingError = "longhorn.io/volume-scheduling-error"

	CniNetworkNone          = ""
	StorageNetworkInterface = "lhnet1"

	KubeAPIQPS   = 50
	KubeAPIBurst = 100

	CSISidecarMetricsPort         = 8000
	CSISidecarPortNameAttacher    = "csi-attacher"
	CSISidecarPortNameProvisioner = "csi-provisioner"
	CSISidecarPortNameResizer     = "csi-resizer"
	CSISidecarPortNameSnapshotter = "csi-snapshotter"
)

const (
	RecurringJobBackupParameterFullBackupInterval = "full-backup-interval"
)

const (
	KubernetesMinVersion = "v1.18.0"
)

const (
	EnvNodeName       = "NODE_NAME"
	EnvPodName        = "POD_NAME"
	EnvPodNamespace   = "POD_NAMESPACE"
	EnvPodIP          = "POD_IP"
	EnvServiceAccount = "SERVICE_ACCOUNT"

	BackupStoreTypeS3     = "s3"
	BackupStoreTypeCIFS   = "cifs"
	BackupStoreTypeNFS    = "nfs"
	BackupStoreTypeAZBlob = "azblob"

	AWSIAMRoleAnnotation = "iam.amazonaws.com/role"
	AWSIAMRoleArn        = "AWS_IAM_ROLE_ARN"
	AWSAccessKey         = "AWS_ACCESS_KEY_ID"
	AWSSecretKey         = "AWS_SECRET_ACCESS_KEY"
	AWSEndPoint          = "AWS_ENDPOINTS"
	AWSCert              = "AWS_CERT"

	CIFSUsername = "CIFS_USERNAME"
	CIFSPassword = "CIFS_PASSWORD"

	AZBlobAccountName = "AZBLOB_ACCOUNT_NAME"
	AZBlobAccountKey  = "AZBLOB_ACCOUNT_KEY"
	AZBlobEndpoint    = "AZBLOB_ENDPOINT"
	AZBlobCert        = "AZBLOB_CERT"

	HTTPSProxy = "HTTPS_PROXY"
	HTTPProxy  = "HTTP_PROXY"
	NOProxy    = "NO_PROXY"

	VirtualHostedStyle = "VIRTUAL_HOSTED_STYLE"

	OptionFromBackup          = "fromBackup"
	OptionNumberOfReplicas    = "numberOfReplicas"
	OptionStaleReplicaTimeout = "staleReplicaTimeout"
	OptionBaseImage           = "baseImage"
	OptionFrontend            = "frontend"
	OptionDiskSelector        = "diskSelector"
	OptionNodeSelector        = "nodeSelector"

	// DefaultStaleReplicaTimeout in minutes. 48h by default
	DefaultStaleReplicaTimeout = "2880"

	ImageChecksumNameLength             = 8
	InstanceManagerSuffixChecksumLength = 32
)

const (
	// CryptoKeyProvider specifies how the CryptoKeyValue is retrieved
	// We currently only support passphrase retrieval via direct secret values
	CryptoKeyProvider = "CRYPTO_KEY_PROVIDER"
	CryptoKeyValue    = "CRYPTO_KEY_VALUE"
	CryptoKeyCipher   = "CRYPTO_KEY_CIPHER"
	CryptoKeyHash     = "CRYPTO_KEY_HASH"
	CryptoKeySize     = "CRYPTO_KEY_SIZE"
	CryptoPBKDF       = "CRYPTO_PBKDF"
)

// SettingsRelatedToVolume should match the items in datastore.GetLabelsForVolumesFollowsGlobalSettings
//
//	TODO: May need to add the data locality check
var SettingsRelatedToVolume = map[string]string{
	string(SettingNameReplicaAutoBalance):                  LonghornLabelValueIgnored,
	string(SettingNameSnapshotDataIntegrity):               LonghornLabelValueIgnored,
	string(SettingNameRemoveSnapshotsDuringFilesystemTrim): LonghornLabelValueIgnored,
}

type NotFoundError struct {
	Name string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("cannot find %v", e.Name)
}

type ErrorInvalidState struct {
	Reason string
}

func (e *ErrorInvalidState) Error() string {
	return fmt.Sprintf("current state prevents this: %v", e.Reason)
}

const (
	engineSuffix    = "-e"
	replicaSuffix   = "-r"
	recurringSuffix = "-c"

	engineImagePrefix          = "ei-"
	instanceManagerImagePrefix = "imi-"
	shareManagerImagePrefix    = "smi-"
	orphanPrefix               = "orphan-"

	BackingImageDataSourcePodNamePrefix = "backing-image-ds-"

	shareManagerPrefix    = "share-manager-"
	recoveryBackendPrefix = "recovery-backend-"
	instanceManagerPrefix = "instance-manager-"
	engineManagerPrefix   = instanceManagerPrefix + "e-"
	replicaManagerPrefix  = instanceManagerPrefix + "r-"
)

func GenerateEngineNameForVolume(vName, currentEngineName string) string {
	if currentEngineName == "" {
		return vName + engineSuffix + "-" + "0"
	}
	parts := strings.Split(currentEngineName, "-")
	suffix, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return vName + engineSuffix + "-" + "1"
	}
	return vName + engineSuffix + "-" + strconv.Itoa(suffix+1)
}

func GenerateReplicaNameForVolume(vName string) string {
	return vName + replicaSuffix + "-" + util.RandomID()
}

func GetCronJobNameForRecurringJob(name string) string {
	return name + recurringSuffix
}

func GetCronJobNameForVolumeAndJob(vName, job string) string {
	return vName + "-" + job + recurringSuffix
}

func GetAPIServerAddressFromIP(ip string) string {
	return net.JoinHostPort(ip, strconv.Itoa(DefaultAPIPort))
}

func GetDefaultManagerURL() string {
	return "http://longhorn-backend:" + strconv.Itoa(DefaultAPIPort) + "/v1"
}

func GetImageCanonicalName(image string) string {
	return strings.Replace(strings.Replace(image, ":", "-", -1), "/", "-", -1)
}

func GetEngineBinaryDirectoryOnHostForImage(image string) string {
	cname := GetImageCanonicalName(image)
	return filepath.Join(EngineBinaryDirectoryOnHost, cname)
}

func GetEngineBinaryDirectoryForEngineManagerContainer(image string) string {
	cname := GetImageCanonicalName(image)
	return filepath.Join(EngineBinaryDirectoryInContainer, cname)
}

func GetEngineBinaryDirectoryForReplicaManagerContainer(image string) string {
	cname := GetImageCanonicalName(image)
	return filepath.Join(filepath.Join(ReplicaHostPrefix, EngineBinaryDirectoryOnHost), cname)
}

func EngineBinaryExistOnHostForImage(image string) bool {
	st, err := os.Stat(filepath.Join(GetEngineBinaryDirectoryOnHostForImage(image), "longhorn"))
	return err == nil && !st.IsDir()
}

func GetBackingImageManagerName(image, diskUUID string) string {
	return fmt.Sprintf("backing-image-manager-%s-%s", util.GetStringChecksum(image)[:4], diskUUID[:4])
}

func GetBackingImageDirectoryName(backingImageName, backingImageUUID string) string {
	return fmt.Sprintf("%s-%s", backingImageName, backingImageUUID)
}

func GetBackingImageManagerDirectoryOnHost(diskPath string) string {
	return filepath.Join(diskPath, BackingImageManagerDirectory)
}

func GetBackingImageDirectoryOnHost(diskPath, backingImageName, backingImageUUID string) string {
	return filepath.Join(GetBackingImageManagerDirectoryOnHost(diskPath), GetBackingImageDirectoryName(backingImageName, backingImageUUID))
}

func GetBackingImagePathForReplicaManagerContainer(diskPath, backingImageName, backingImageUUID string) string {
	return filepath.Join(ReplicaHostPrefix, GetBackingImageDirectoryOnHost(diskPath, backingImageName, backingImageUUID), BackingImageFileName)
}

var (
	LonghornSystemKey = "longhorn"
)

func GetLonghornLabelKey(name string) string {
	return fmt.Sprintf("%s/%s", LonghornLabelKeyPrefix, name)
}

func GetBaseLabelsForSystemManagedComponent() map[string]string {
	return map[string]string{GetLonghornLabelKey(LonghornLabelManagedBy): ControlPlaneName}
}

func GetLonghornLabelComponentKey() string {
	return GetLonghornLabelKey("component")
}

func GetLonghornLabelCRDAPIVersionKey() string {
	return GetLonghornLabelKey(LonghornLabelCRDAPIVersion)
}

func GetManagerLabels() map[string]string {
	return map[string]string{
		"app": LonghornManagerDaemonSetName,
	}
}

func GetAdmissionWebhookLabel() map[string]string {
	return map[string]string{
		GetLonghornLabelKey(LonghornLabelAdmissionWebhook): AdmissionWebhookServiceName,
	}
}

func GetRecoveryBackendLabel() map[string]string {
	return map[string]string{
		GetLonghornLabelKey(LonghornLabelRecoveryBackend): LonghornRecoveryBackendServiceName,
	}
}

func GetConversionWebhookLabel() map[string]string {
	return map[string]string{
		GetLonghornLabelKey(LonghornLabelConversionWebhook): ConversionWebhookServiceName,
	}
}

func GetEngineImageLabels(engineImageName string) map[string]string {
	labels := GetBaseLabelsForSystemManagedComponent()
	labels[GetLonghornLabelComponentKey()] = LonghornLabelEngineImage
	labels[GetLonghornLabelKey(LonghornLabelEngineImage)] = engineImageName
	return labels
}

// GetEIDaemonSetLabelSelector returns labels for engine image daemonset's Spec.Selector.MatchLabels
func GetEIDaemonSetLabelSelector(engineImageName string) map[string]string {
	labels := make(map[string]string)
	labels[GetLonghornLabelComponentKey()] = LonghornLabelEngineImage
	labels[GetLonghornLabelKey(LonghornLabelEngineImage)] = engineImageName
	return labels
}

func GetInstanceManagerLabels(node, imImage string, imType longhorn.InstanceManagerType, dataEngine longhorn.DataEngineType) map[string]string {
	labels := GetBaseLabelsForSystemManagedComponent()
	labels[GetLonghornLabelComponentKey()] = LonghornLabelInstanceManager
	labels[GetLonghornLabelKey(LonghornLabelInstanceManagerType)] = string(imType)

	if node != "" {
		labels[GetLonghornLabelKey(LonghornLabelNode)] = node
	}
	if imImage != "" {
		labels[GetLonghornLabelKey(LonghornLabelInstanceManagerImage)] = GetInstanceManagerImageChecksumName(GetImageCanonicalName(imImage))
	}
	if dataEngine != "" && dataEngine != longhorn.DataEngineTypeAll {
		labels[GetLonghornLabelKey(LonghornLabelDataEngine)] = string(dataEngine)
	}

	return labels
}

func GetInstanceManagerComponentLabel() map[string]string {
	return map[string]string{
		GetLonghornLabelComponentKey(): LonghornLabelInstanceManager,
	}
}

func GetShareManagerComponentLabel() map[string]string {
	return map[string]string{
		GetLonghornLabelComponentKey(): LonghornLabelShareManager,
	}
}

func GetShareManagerInstanceLabel(name string) map[string]string {
	labels := GetBaseLabelsForSystemManagedComponent()
	labels[GetLonghornLabelKey(LonghornLabelShareManager)] = name
	return labels
}

func GetShareManagerLabels(name, image string) map[string]string {
	labels := GetBaseLabelsForSystemManagedComponent()
	labels[GetLonghornLabelComponentKey()] = LonghornLabelShareManager

	if name != "" {
		labels[GetLonghornLabelKey(LonghornLabelShareManager)] = name
	}

	if image != "" {
		labels[GetLonghornLabelKey(LonghornLabelShareManagerImage)] = GetShareManagerImageChecksumName(GetImageCanonicalName(image))
	}

	return labels
}

func GetShareManagerConfigMapLabels(name string) map[string]string {
	labels := GetBaseLabelsForSystemManagedComponent()
	labels[GetLonghornLabelKey(LonghornLabelShareManager)] = name
	labels[GetLonghornLabelComponentKey()] = LonghornLabelShareManagerConfigMap
	return labels
}

func GetCronJobLabels(job *longhorn.RecurringJobSpec) map[string]string {
	labels := GetBaseLabelsForSystemManagedComponent()
	labels[fmt.Sprintf(LonghornLabelRecurringJobKeyPrefixFmt, LonghornLabelRecurringJob)] = job.Name
	return labels
}

func GetBackingImageLabels() map[string]string {
	labels := GetBaseLabelsForSystemManagedComponent()
	labels[GetLonghornLabelComponentKey()] = LonghornLabelBackingImage
	return labels
}

func GetBackingImageWithBackupTargetLabels(backupTargetName, backingImageName string) map[string]string {
	return map[string]string{
		LonghornLabelBackingImage: backingImageName,
		LonghornLabelBackupTarget: backupTargetName,
	}
}

func GetBackingImageManagerLabels(nodeID, diskUUID string) map[string]string {
	labels := GetBaseLabelsForSystemManagedComponent()
	labels[GetLonghornLabelComponentKey()] = LonghornLabelBackingImageManager
	if diskUUID != "" {
		labels[GetLonghornLabelKey(LonghornLabelDiskUUID)] = diskUUID
	}
	if nodeID != "" {
		labels[GetLonghornLabelKey(LonghornLabelNode)] = nodeID
	}
	return labels
}

func GetBackingImageDataSourceLabels(name, nodeID, diskUUID string) map[string]string {
	labels := GetBaseLabelsForSystemManagedComponent()
	labels[GetLonghornLabelComponentKey()] = LonghornLabelBackingImageDataSource
	if name != "" {
		labels[GetLonghornLabelKey(LonghornLabelBackingImageDataSource)] = name
	}
	if diskUUID != "" {
		labels[GetLonghornLabelKey(LonghornLabelDiskUUID)] = diskUUID
	}
	if nodeID != "" {
		labels[GetLonghornLabelKey(LonghornLabelNode)] = nodeID
	}
	return labels
}

func GetBackupVolumeWithBackupTargetLabels(backupTargetName, volumeName string) map[string]string {
	return map[string]string{
		LonghornLabelBackupVolume: volumeName,
		LonghornLabelBackupTarget: backupTargetName,
	}
}

func GetBackupTargetLabels(backupTargetName string) map[string]string {
	return map[string]string{
		LonghornLabelBackupTarget: backupTargetName,
	}
}

func GetBackupVolumeLabels(volumeName string) map[string]string {
	return map[string]string{
		LonghornLabelBackupVolume: volumeName,
	}
}

func GetVolumeLabels(volumeName string) map[string]string {
	return map[string]string{
		LonghornLabelVolume: volumeName,
	}
}

func GetRecurringJobLabelKeyByType(name string, isGroup bool) string {
	if isGroup {
		return GetRecurringJobLabelKey(LonghornLabelRecurringJobGroup, name)
	}
	return GetRecurringJobLabelKey(LonghornLabelRecurringJob, name)
}

func GetRecurringJobLabelKey(labelType, recurringJobName string) string {
	prefix := fmt.Sprintf(LonghornLabelRecurringJobKeyPrefixFmt, labelType)
	return fmt.Sprintf("%s/%s", prefix, recurringJobName)
}

// GetRecurringJobSourceLabelKey returns "recurring-job.longhorn.io/source"
func GetRecurringJobSourceLabelKey() string {
	prefix := fmt.Sprintf(LonghornLabelRecurringJobKeyPrefixFmt, LonghornLabelRecurringJob)
	return fmt.Sprintf("%s/%s", prefix, LonghornLabelRecurringJobSource)
}

func GetRecurringJobLabelValueMap(labelType, recurringJobName string) map[string]string {
	return map[string]string{
		GetRecurringJobLabelKey(labelType, recurringJobName): LonghornLabelValueEnabled,
	}
}

// IsRecurringJobLabel checks if the given key is a recurring job label.
func IsRecurringJobLabel(key string) bool {
	if IsRecurringJobSourceLabel(key) {
		return false
	}

	jobPrefix := fmt.Sprintf(LonghornLabelRecurringJobKeyPrefixFmt, LonghornLabelRecurringJob)
	groupPrefix := fmt.Sprintf(LonghornLabelRecurringJobKeyPrefixFmt, LonghornLabelRecurringJobGroup)
	return strings.HasPrefix(key, jobPrefix) || strings.HasPrefix(key, groupPrefix)
}

// IsRecurringJobSourceLabel checks if the given key is the recurring job source label.
func IsRecurringJobSourceLabel(key string) bool {
	return key == GetRecurringJobSourceLabelKey()
}

func GetOrphanLabelsForOrphanedDirectory(nodeID, diskUUID string) map[string]string {
	labels := GetBaseLabelsForSystemManagedComponent()
	labels[GetLonghornLabelComponentKey()] = LonghornLabelOrphan
	labels[LonghornNodeKey] = nodeID
	labels[GetLonghornLabelKey(LonghornLabelOrphanType)] = string(longhorn.OrphanTypeReplica)
	return labels
}

func GetRecoveryBackendConfigMapLabels() map[string]string {
	labels := GetBaseLabelsForSystemManagedComponent()
	labels[GetLonghornLabelComponentKey()] = LonghornLabelRecoveryBackend
	return labels
}

func GetSystemRestoreInProgressLabel() map[string]string {
	return map[string]string{
		GetSystemRestoreLabelKey(): string(longhorn.SystemRestoreStateInProgress),
	}
}

func GetSystemRestoreLabelKey() string {
	return GetLonghornLabelKey(LonghornLabelSystemRestore)
}

func GetLastSystemRestoreLabelKey() string {
	return GetLonghornLabelKey(LonghornLabelLastSystemRestore)
}

func GetLastSystemRestoreAtLabelKey() string {
	return GetLonghornLabelKey(LonghornLabelLastSystemRestoreAt)
}

func GetLastSkippedSystemRestoreLabelKey() string {
	return GetLonghornLabelKey(LonghornLabelLastSkippedSystemRestore)
}

func GetLastSkippedSystemRestoreAtLabelKey() string {
	return GetLonghornLabelKey(LonghornLabelLastSkippedSystemRestoreAt)
}

func GetLastSystemRestoreBackupLabelKey() string {
	return GetLonghornLabelKey(LonghornLabelLastSystemRestoreBackup)
}

func GetVersionLabelKey() string {
	return GetLonghornLabelKey(LonghornLabelVersion)
}

func GetRegionAndZone(labels map[string]string) (string, string) {
	region := ""
	zone := ""
	if v, ok := labels[KubernetesTopologyRegionLabelKey]; ok {
		region = v
	}
	if v, ok := labels[KubernetesTopologyZoneLabelKey]; ok {
		zone = v
	}
	return region, zone
}

func GetEngineImageChecksumName(image string) string {
	return engineImagePrefix + util.GetStringChecksum(strings.TrimSpace(image))[:ImageChecksumNameLength]
}

func GetInstanceManagerImageChecksumName(image string) string {
	return instanceManagerImagePrefix + util.GetStringChecksum(strings.TrimSpace(image))[:ImageChecksumNameLength]
}

func GetShareManagerImageChecksumName(image string) string {
	return shareManagerImagePrefix + util.GetStringChecksum(strings.TrimSpace(image))[:ImageChecksumNameLength]
}

func GetOrphanChecksumNameForOrphanedDataStore(nodeID, diskName, diskPath, diskUUID, dataStore string) string {
	return orphanPrefix + util.GetStringChecksumSHA256(strings.TrimSpace(fmt.Sprintf("%s-%s-%s-%s-%s", nodeID, diskName, diskPath, diskUUID, dataStore)))
}

func GetShareManagerPodNameFromShareManagerName(smName string) string {
	return shareManagerPrefix + smName
}

func GetConfigMapNameFromShareManagerName(smName string) string {
	return recoveryBackendPrefix + shareManagerPrefix + smName
}

func GetConfigMapNameFromHostname(hostname string) string {
	return recoveryBackendPrefix + hostname
}

func GetShareManagerNameFromShareManagerPodName(podName string) string {
	return strings.TrimPrefix(podName, shareManagerPrefix)
}

func GetBackupVolumeNameFromVolumeName(volumeName string) string {
	return volumeName + "-" + util.RandomID()
}

func GetBackupBackingImageNameFromBIName(backingImageName string) string {
	return backingImageName + "-" + util.RandomID()
}

func ValidateEngineImageChecksumName(name string) bool {
	matched, _ := regexp.MatchString(fmt.Sprintf("^%s[a-fA-F0-9]{%d}$", engineImagePrefix, ImageChecksumNameLength), name)
	return matched
}

func GetInstanceManagerName(imType longhorn.InstanceManagerType, nodeName, image, dataEngine string) (string, error) {
	hashedSuffix := util.GetStringChecksum(nodeName + image + dataEngine)[:InstanceManagerSuffixChecksumLength]
	switch imType {
	case longhorn.InstanceManagerTypeEngine:
		return engineManagerPrefix + hashedSuffix, nil
	case longhorn.InstanceManagerTypeReplica:
		return replicaManagerPrefix + hashedSuffix, nil
	case longhorn.InstanceManagerTypeAllInOne:
		return instanceManagerPrefix + hashedSuffix, nil
	}
	return "", fmt.Errorf("cannot generate name for unknown instance manager type %v", imType)
}

func GetInstanceManagerPrefix(imType longhorn.InstanceManagerType) string {
	switch imType {
	case longhorn.InstanceManagerTypeAllInOne:
		return instanceManagerPrefix
	case longhorn.InstanceManagerTypeEngine:
		return engineManagerPrefix
	case longhorn.InstanceManagerTypeReplica:
		return replicaManagerPrefix
	}
	return ""
}

func GetBackingImageDataSourcePodName(bidsName string) string {
	return fmt.Sprintf("%s%s", BackingImageDataSourcePodNamePrefix, bidsName)
}

func GetReplicaDataPath(diskPath, dataDirectoryName string) string {
	return filepath.Join(diskPath, "replicas", dataDirectoryName)
}

func GetReplicaMountedDataPath(dataPath string) string {
	if !strings.HasPrefix(dataPath, ReplicaHostPrefix) {
		return filepath.Join(ReplicaHostPrefix, dataPath)
	}
	return dataPath
}

func ErrorRecordNotFoundButLvolFound(err error) bool {
	return strings.Contains(err.Error(), "lvol found")
}

func ErrorIsNotFound(err error) bool {
	return strings.Contains(err.Error(), "cannot find")
}

func ErrorIsInProgress(err error) bool {
	return strings.Contains(err.Error(), "in progress")
}

func ErrorIsStopped(err error) bool {
	return strings.Contains(err.Error(), "is stopped")
}

func ErrorIsNotSupport(err error) bool {
	return strings.Contains(err.Error(), "not support")
}

func ErrorAlreadyExists(err error) bool {
	return strings.Contains(err.Error(), "already exists")
}

func ErrorIsInvalidState(err error) bool {
	var dummy *ErrorInvalidState
	return errors.As(err, &dummy)
}

func ValidateReplicaCount(count int) error {

	definition, _ := GetSettingDefinition(SettingNameDefaultReplicaCount)
	valueIntRange := definition.ValueIntRange

	if count < valueIntRange[ValueIntRangeMinimum] || count > valueIntRange[ValueIntRangeMaximum] {
		return fmt.Errorf("replica count value %v must between %v to %v", count, valueIntRange[ValueIntRangeMinimum], valueIntRange[ValueIntRangeMaximum])
	}
	return nil
}

func ValidateMinNumberOfBackingIamgeCopies(number int) error {
	definition, exists := GetSettingDefinition(SettingNameDefaultMinNumberOfBackingImageCopies)
	if !exists {
		return fmt.Errorf("setting %v definition does not exists", SettingNameDefaultMinNumberOfBackingImageCopies)
	}
	valueIntRange := definition.ValueIntRange

	if number < valueIntRange[ValueIntRangeMinimum] {
		return fmt.Errorf("minimum number of backing image copies %v must larger than %v", number, valueIntRange[ValueIntRangeMaximum])
	}
	return nil
}

func ValidateV2DataEngineLogFlags(flags string) error {
	if flags == "" {
		return nil
	}

	// A valid string is like "all,bdev_raid,vbdev_zone_block"
	pattern := "^(([a-zA-Z]+_?)+[a-zA-Z]+,?)+$"
	reg := regexp.MustCompile(pattern)

	if !reg.MatchString(flags) {
		return fmt.Errorf("log flags %s is invalid", flags)
	}

	return nil
}

func ValidateDataLocalityAndReplicaCount(mode longhorn.DataLocality, count int) error {
	if mode == longhorn.DataLocalityStrictLocal && count != 1 {
		return fmt.Errorf("replica count should be 1 in data locality %v mode", longhorn.DataLocalityStrictLocal)
	}
	return nil
}

func ValidateDataLocalityAndAccessMode(locality longhorn.DataLocality, migratable bool, mode longhorn.AccessMode) error {
	if mode == longhorn.AccessModeReadWriteMany && !migratable && locality == longhorn.DataLocalityStrictLocal {
		return fmt.Errorf("access mode %v (migratable: %v) is incompatible with data locality %v mode", mode, migratable, longhorn.DataLocalityStrictLocal)
	}
	return nil
}

func ValidateReplicaAutoBalance(option longhorn.ReplicaAutoBalance) error {
	switch option {
	case longhorn.ReplicaAutoBalanceIgnored,
		longhorn.ReplicaAutoBalanceDisabled,
		longhorn.ReplicaAutoBalanceLeastEffort,
		longhorn.ReplicaAutoBalanceBestEffort:
		return nil
	default:
		return fmt.Errorf("invalid replica auto-balance option: %v", option)
	}
}

func ValidateDataLocality(mode longhorn.DataLocality) error {
	if mode != longhorn.DataLocalityDisabled && mode != longhorn.DataLocalityBestEffort && mode != longhorn.DataLocalityStrictLocal {
		return fmt.Errorf("invalid data locality mode: %v", mode)
	}
	return nil
}

func ValidateAccessMode(mode longhorn.AccessMode) error {
	if mode != longhorn.AccessModeReadWriteMany && mode != longhorn.AccessModeReadWriteOnce {
		return fmt.Errorf("invalid access mode: %v", mode)
	}
	return nil
}

func ValidateStorageNetwork(value string) (err error) {
	if value == CniNetworkNone {
		return nil
	}

	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return errors.Errorf("storage network must be in <NAMESPACE>/<NETWORK-ATTACHMENT-DEFINITION> format: %v", value)
	}
	return nil
}

func ValidateSnapshotDataIntegrity(mode string) error {
	if mode != string(longhorn.SnapshotDataIntegrityDisabled) &&
		mode != string(longhorn.SnapshotDataIntegrityEnabled) &&
		mode != string(longhorn.SnapshotDataIntegrityFastCheck) {
		return fmt.Errorf("invalid snapshot data integrity mode: %v", mode)
	}
	return nil
}

func ValidateBackupCompressionMethod(method string) error {
	if method != string(longhorn.BackupCompressionMethodNone) &&
		method != string(longhorn.BackupCompressionMethodLz4) &&
		method != string(longhorn.BackupCompressionMethodGzip) {
		return fmt.Errorf("invalid backup compression method: %v", method)
	}
	return nil
}

func ValidateUnmapMarkSnapChainRemoved(dataEngine longhorn.DataEngineType, unmapValue longhorn.UnmapMarkSnapChainRemoved) error {
	if IsDataEngineV2(dataEngine) {
		if unmapValue != longhorn.UnmapMarkSnapChainRemovedDisabled {
			return fmt.Errorf("invalid UnmapMarkSnapChainRemoved setting: %v", unmapValue)
		}
	}

	if unmapValue != longhorn.UnmapMarkSnapChainRemovedIgnored &&
		unmapValue != longhorn.UnmapMarkSnapChainRemovedEnabled &&
		unmapValue != longhorn.UnmapMarkSnapChainRemovedDisabled {
		return fmt.Errorf("invalid UnmapMarkSnapChainRemoved setting: %v", unmapValue)
	}
	return nil
}

func ValidateReplicaSoftAntiAffinity(value longhorn.ReplicaSoftAntiAffinity) error {
	if value != longhorn.ReplicaSoftAntiAffinityDefault &&
		value != longhorn.ReplicaSoftAntiAffinityEnabled &&
		value != longhorn.ReplicaSoftAntiAffinityDisabled {
		return fmt.Errorf("invalid ReplicaSoftAntiAffinity setting: %v", value)
	}
	return nil
}

func ValidateReplicaZoneSoftAntiAffinity(value longhorn.ReplicaZoneSoftAntiAffinity) error {
	if value != longhorn.ReplicaZoneSoftAntiAffinityDefault &&
		value != longhorn.ReplicaZoneSoftAntiAffinityEnabled &&
		value != longhorn.ReplicaZoneSoftAntiAffinityDisabled {
		return fmt.Errorf("invalid ReplicaZoneSoftAntiAffinity setting: %v", value)
	}
	return nil
}

func ValidateReplicaDiskSoftAntiAffinity(value longhorn.ReplicaDiskSoftAntiAffinity) error {
	if value != longhorn.ReplicaDiskSoftAntiAffinityDefault &&
		value != longhorn.ReplicaDiskSoftAntiAffinityEnabled &&
		value != longhorn.ReplicaDiskSoftAntiAffinityDisabled {
		return fmt.Errorf("invalid ReplicaDiskSoftAntiAffinity setting: %v", value)
	}
	return nil
}

func ValidateFreezeFilesystemForSnapshot(value longhorn.FreezeFilesystemForSnapshot) error {
	if value != longhorn.FreezeFilesystemForSnapshotDefault &&
		value != longhorn.FreezeFilesystemForSnapshotEnabled &&
		value != longhorn.FreezeFilesystemForSnapshotDisabled {
		return fmt.Errorf("invalid FreezeFilesystemForSnapshot setting: %v", value)
	}
	return nil
}

func GetDaemonSetNameFromEngineImageName(engineImageName string) string {
	return "engine-image-" + engineImageName
}

func GetEngineImageNameFromDaemonSetName(dsName string) string {
	return strings.TrimPrefix(dsName, "engine-image-")
}

func GetVolumeSettingLabelKey(settingName string) string {
	return fmt.Sprintf("%s/%s", LonghornLabelVolumeSettingKeyPrefix, settingName)
}

func LabelsToString(labels map[string]string) string {
	res := ""
	for k, v := range labels {
		res += fmt.Sprintf("%s=%s,", k, v)
	}
	res = strings.TrimSuffix(res, ",")
	return res
}

func CreateDisksFromAnnotation(annotation string, storageReservedPercentage int64) (map[string]longhorn.DiskSpec, error) {
	validDisks := map[string]longhorn.DiskSpec{}
	existDiskID := map[string]string{}

	disks, err := UnmarshalToDisks(annotation)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal the default disks annotation")
	}
	for _, disk := range disks {
		if disk.Path == "" {
			return nil, fmt.Errorf("invalid disk %+v", disk)
		}
		diskStat, err := lhns.GetDiskStat(disk.Path)
		if err != nil {
			return nil, err
		}
		for _, vDisk := range validDisks {
			if vDisk.Path == disk.Path {
				return nil, fmt.Errorf("duplicate disk path %v", disk.Path)
			}
		}

		// Set to default disk name
		if disk.Name == "" {
			disk.Name = DefaultDiskPrefix + diskStat.DiskID
		}

		if _, exist := existDiskID[diskStat.DiskID]; exist {
			return nil, fmt.Errorf(
				"the disk %v is the same"+
					"file system with %v, diskID %v",
				disk.Path, existDiskID[diskStat.DiskID],
				diskStat.DiskID)
		}

		existDiskID[diskStat.DiskID] = disk.Path

		if disk.StorageReserved < 0 || disk.StorageReserved > diskStat.StorageMaximum {
			return nil, fmt.Errorf("the storageReserved setting of disk %v is not valid, should be positive and no more than storageMaximum and storageAvailable", disk.Path)
		}
		if disk.StorageReserved == 0 {
			if disk.Type == longhorn.DiskTypeBlock {
				size, err := getBlockDeviceSize(ReplicaHostPrefix + disk.Path)
				if err != nil {
					return nil, err
				}
				disk.StorageReserved = int64(size) * storageReservedPercentage / 100
			} else {
				disk.StorageReserved = diskStat.StorageMaximum * storageReservedPercentage / 100
			}
		}
		tags, err := util.ValidateTags(disk.Tags)
		if err != nil {
			return nil, err
		}
		disk.Tags = tags
		_, exists := validDisks[disk.Name]
		if exists {
			return nil, fmt.Errorf("the disk name %v has duplicated", disk.Name)
		}
		validDisks[disk.Name] = disk.DiskSpec
	}

	return validDisks, nil
}

func getBlockDeviceSize(devicePath string) (uint64, error) {
	file, err := os.Open(devicePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open block device at %s: %w", devicePath, err)
	}
	defer file.Close()
	var size uint64
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, file.Fd(), 0x80081272, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return 0, fmt.Errorf("failed to get block device size for %s: errno=%v", devicePath, errno)
	}

	return size, nil
}

func GetNodeTagsFromAnnotation(annotation string) ([]string, error) {
	nodeTags, err := UnmarshalToNodeTags(annotation)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal the node tag annotation")
	}
	validNodeTags, err := util.ValidateTags(nodeTags)
	if err != nil {
		return nil, err
	}

	return validNodeTags, nil
}

type DiskSpecWithName struct {
	longhorn.DiskSpec
	Name string `json:"name"`
}

// UnmarshalToDisks input format should be:
// `[{"path":"/mnt/disk1","allowScheduling":false},
//
//	{"path":"/mnt/disk2","allowScheduling":false,"storageReserved":1024,"tags":["ssd","fast"]}]`
func UnmarshalToDisks(s string) (ret []DiskSpecWithName, err error) {
	if err := json.Unmarshal([]byte(s), &ret); err != nil {
		return nil, err
	}
	return ret, nil
}

// UnmarshalToNodeTags input format should be:
// `["worker1","enabled"]`
func UnmarshalToNodeTags(s string) ([]string, error) {
	var res []string
	if err := json.Unmarshal([]byte(s), &res); err != nil {
		return nil, err
	}
	return res, nil
}

func IsBDF(addr string) bool {
	bdfFormat := "[a-f0-9]{4}:[a-f0-9]{2}:[a-f0-9]{2}\\.[a-f0-9]{1}"
	bdfPattern := regexp.MustCompile(bdfFormat)
	return bdfPattern.MatchString(addr)
}

func IsPotentialBlockDisk(path string) bool {
	if IsBDF(path) {
		return true
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		logrus.WithError(err).Warnf("Failed to get file info for %v", path)
		return strings.HasPrefix(path, "/dev/")
	}

	if fileInfo != nil && (fileInfo.Mode()&os.ModeDevice) == os.ModeDevice {
		return true
	}

	return false
}

func CreateDefaultDisk(dataPath string, storageReservedPercentage int64) (map[string]longhorn.DiskSpec, error) {
	if IsPotentialBlockDisk(dataPath) {
		size, err := getBlockDeviceSize(dataPath)
		if err != nil {
			return nil, err
		}
		return map[string]longhorn.DiskSpec{
			DefaultDiskPrefix + util.RandomID(): {
				Type:              longhorn.DiskTypeBlock,
				Path:              dataPath,
				DiskDriver:        longhorn.DiskDriverAuto,
				AllowScheduling:   true,
				EvictionRequested: false,
				StorageReserved:   int64(size) * storageReservedPercentage / 100,
				Tags:              []string{},
			},
		}, nil
	}

	// Currently, we create a filesystem-type disk for any disk path except for that in /dev/
	if _, err := lhns.CreateDirectory(filepath.Join(dataPath, util.ReplicaDirectory), time.Now()); err != nil {
		return nil, errors.Wrapf(err, "failed to create filesystem-type disk %v", dataPath)
	}

	diskStat, err := lhns.GetDiskStat(dataPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get disk stat for creating default disk %v", dataPath)
	}

	return map[string]longhorn.DiskSpec{
		DefaultDiskPrefix + diskStat.DiskID: {
			Type:              longhorn.DiskTypeFilesystem,
			Path:              diskStat.Path,
			DiskDriver:        longhorn.DiskDriverNone,
			AllowScheduling:   true,
			EvictionRequested: false,
			StorageReserved:   diskStat.StorageMaximum * storageReservedPercentage / 100,
			Tags:              []string{},
		},
	}, nil
}

func ValidateCPUReservationValues(settingName SettingName, instanceManagerCPUStr string) error {
	instanceManagerCPU, err := strconv.Atoi(instanceManagerCPUStr)
	if err != nil {
		return errors.Wrapf(err, "invalid guaranteed/requested instance manager CPU value (%v)", instanceManagerCPUStr)
	}

	definition, _ := GetSettingDefinition(settingName)
	valueIntRange := definition.ValueIntRange

	switch settingName {
	case SettingNameGuaranteedInstanceManagerCPU, SettingNameV2DataEngineGuaranteedInstanceManagerCPU:
		isUnderLimit := instanceManagerCPU < valueIntRange[ValueIntRangeMinimum]
		isOverLimit := instanceManagerCPU > valueIntRange[ValueIntRangeMaximum]
		if isUnderLimit || isOverLimit {
			return fmt.Errorf("invalid requested instance manager CPUs. Valid instance manager CPU range between %v - %v millicpu", valueIntRange[ValueIntRangeMinimum], valueIntRange[ValueIntRangeMaximum])
		}
	}
	return nil
}

type CniNetwork struct {
	Name string   `json:"name"`
	IPs  []string `json:"ips,omitempty"`
}

func CreateCniAnnotationFromSetting(storageNetwork *longhorn.Setting) string {
	if storageNetwork.Value == "" {
		return ""
	}

	storageNetworkSplit := strings.Split(storageNetwork.Value, "/")
	return fmt.Sprintf("[{\"namespace\": \"%s\", \"name\": \"%s\", \"interface\": \"%s\"}]", storageNetworkSplit[0], storageNetworkSplit[1], StorageNetworkInterface)
}

func BackupStoreRequireCredential(backupType string) bool {
	return backupType == BackupStoreTypeS3 || backupType == BackupStoreTypeCIFS || backupType == BackupStoreTypeAZBlob
}

func ConsolidateInstances(instancesMaps ...map[string]longhorn.InstanceProcess) map[string]longhorn.InstanceProcess {
	consolidated := make(map[string]longhorn.InstanceProcess)
	for _, instances := range instancesMaps {
		for name, instance := range instances {
			consolidated[name] = instance
		}
	}
	return consolidated
}

func ConsolidateInstanceManagers(instanceManagerMaps ...map[string]*longhorn.InstanceManager) map[string]*longhorn.InstanceManager {
	consolidated := make(map[string]*longhorn.InstanceManager)
	for _, instanceManagers := range instanceManagerMaps {
		for name, instanceManager := range instanceManagers {
			consolidated[name] = instanceManager
		}
	}
	return consolidated
}

func GetLHVolumeAttachmentNameFromVolumeName(volName string) string {
	return volName
}

// IsSelectorsInTags checks if all the selectors are present in the tags slice.
// It returns true if all selectors are found, false otherwise.
func IsSelectorsInTags(tags, selectors []string, allowEmptySelector bool) bool {
	if !sort.StringsAreSorted(tags) {
		logrus.Debug("BUG: Tags are not sorted, sorting now")
		sort.Strings(tags)
	}

	if len(selectors) == 0 {
		if !allowEmptySelector && len(tags) != 0 {
			return false
		}
	}

	for _, selector := range selectors {
		index := sort.SearchStrings(tags, selector)
		// If the selector is not found or the index is out of bounds, return false.
		if index >= len(tags) || tags[index] != selector {
			return false
		}
	}

	return true
}

func GetKubernetesProviderNameFromURL(providerURL string) string {
	if providerURL == "" {
		return ValueEmpty
	}

	scheme := util.GetSchemeFromURL(providerURL)
	if scheme == "" {
		scheme = ValueUnknown
	}
	return scheme
}

func GetBackupTargetSchemeFromURL(backupTargetURL string) string {
	if backupTargetURL == "" {
		return ValueEmpty
	}

	scheme := util.GetSchemeFromURL(backupTargetURL)
	switch scheme {
	case "azblob", "cifs", "nfs", "s3":
		return scheme
	default:
		return ValueUnknown
	}
}

func GetPDBName(im *longhorn.InstanceManager) string {
	return GetPDBNameFromIMName(im.Name)
}

func GetPDBNameFromIMName(imName string) string {
	return imName
}

func GetIMNameFromPDBName(pdbName string) string {
	return pdbName
}

// IsDataEngineV1 returns true if the given dataEngine is v1
func IsDataEngineV1(dataEngine longhorn.DataEngineType) bool {
	return dataEngine != longhorn.DataEngineTypeV2
}

// IsDataEngineV2 returns true if the given dataEngine is v2
func IsDataEngineV2(dataEngine longhorn.DataEngineType) bool {
	return dataEngine == longhorn.DataEngineTypeV2
}

// IsStorageNetworkForRWXVolume returns true if the storage network setting value is not empty.
// And isStorageNetworkForRWXVolumeEnabled is true.
func IsStorageNetworkForRWXVolume(storageNetwork *longhorn.Setting, isStorageNetworkForRWXVolumeEnabled bool) bool {
	if storageNetwork == nil {
		return false
	}
	return storageNetwork.Value != CniNetworkNone && isStorageNetworkForRWXVolumeEnabled
}

func MergeStringMaps(baseMap, overwriteMap map[string]string) map[string]string {
	result := map[string]string{}
	for k, v := range baseMap {
		result[k] = v
	}
	for k, v := range overwriteMap {
		result[k] = v
	}
	return result
}

func GetBackingImageMonitorName(imName string) string {
	return fmt.Sprintf("backing-image-monitor-%s", imName)
}

func GetV2BackingImageWithDiskUUIDName(biName, v2DiskUUID string) string {
	return fmt.Sprintf("%v-%v", biName, v2DiskUUID)
}
