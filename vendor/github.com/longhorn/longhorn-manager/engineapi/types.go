package engineapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	iscsidevtypes "github.com/longhorn/go-iscsi-helper/types"
	spdkdevtypes "github.com/longhorn/go-spdk-helper/pkg/types"
	imapi "github.com/longhorn/longhorn-instance-manager/pkg/api"

	emeta "github.com/longhorn/longhorn-engine/pkg/meta"
	etypes "github.com/longhorn/longhorn-engine/pkg/types"

	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	CLIVersionFour = 4
	CLIVersionFive = 5

	// CLIAPIMinVersionForExistingEngineBeforeUpgrade will enable already created volumes before the upgrade to operate normally.
	// Additionally, they will not be impacted by the new engine upgrade enforcement mechanism.
	// This mechanism exclusively focuses on preventing users from creating or updating a volume, engine, or replica using any incompatible version.
	// It is strictly bound to the default engine image of the release, emeta.CLIAPIMinVersion.
	CLIAPIMinVersionForExistingEngineBeforeUpgrade = 3

	InstanceManagerProcessManagerServiceDefaultPort = 8500
	InstanceManagerProxyServiceDefaultPort          = InstanceManagerProcessManagerServiceDefaultPort + 1 // 8501
	InstanceManagerDiskServiceDefaultPort           = InstanceManagerProcessManagerServiceDefaultPort + 2 // 8502
	InstanceManagerInstanceServiceDefaultPort       = InstanceManagerProcessManagerServiceDefaultPort + 3 // 8503
	InstanceManagerSpdkServiceDefaultPort           = InstanceManagerProcessManagerServiceDefaultPort + 4 // 8504

	BackingImageManagerDefaultPort    = 8000
	BackingImageDataSourceDefaultPort = 8000
	BackingImageSyncServerDefaultPort = 8001

	ShareManagerDefaultPort = 9600

	EndpointISCSIPrefix = "iscsi://"
	DefaultISCSIPort    = "3260"
	DefaultISCSILUN     = "1"

	// MaxPollCount, MinPollCount, PollInterval determines how often
	// we sync with othersq

	MaxPollCount = 60
	MinPollCount = 1
	PollInterval = 1 * time.Second

	BackingImageDataSourcePollInterval = 3 * PollInterval

	MaxMonitorRetryCount = 10
)

type Replica struct {
	URL  string
	Mode longhorn.ReplicaMode
}

type Controller struct {
	URL    string
	NodeID string
}

type Metrics struct {
	ReadThroughput  uint64
	WriteThroughput uint64
	ReadLatency     uint64
	WriteLatency    uint64
	ReadIOPS        uint64
	WriteIOPS       uint64
}

type EngineClient interface {
	VersionGet(engine *longhorn.Engine, clientOnly bool) (*EngineVersion, error)

	VolumeGet(*longhorn.Engine) (*Volume, error)
	VolumeExpand(*longhorn.Engine) error

	VolumeFrontendStart(*longhorn.Engine) error
	VolumeFrontendShutdown(*longhorn.Engine) error
	VolumeUnmapMarkSnapChainRemovedSet(engine *longhorn.Engine) error
	VolumeSnapshotMaxCountSet(engine *longhorn.Engine) error
	VolumeSnapshotMaxSizeSet(engine *longhorn.Engine) error

	ReplicaList(*longhorn.Engine) (map[string]*Replica, error)
	ReplicaAdd(engine *longhorn.Engine, replicaName, url string, isRestoreVolume, fastSync bool, localSync *etypes.FileLocalSync, replicaFileSyncHTTPClientTimeout, grpcTimeoutSeconds int64) error
	ReplicaRemove(engine *longhorn.Engine, url, replicaName string) error
	ReplicaRebuildStatus(*longhorn.Engine) (map[string]*longhorn.RebuildStatus, error)
	ReplicaRebuildVerify(engine *longhorn.Engine, replicaName, url string) error
	ReplicaModeUpdate(engine *longhorn.Engine, url string, mode string) error

	SnapshotCreate(engine *longhorn.Engine, name string, labels map[string]string, freezeFilesystem bool) (string, error)
	SnapshotList(engine *longhorn.Engine) (map[string]*longhorn.SnapshotInfo, error)
	SnapshotGet(engine *longhorn.Engine, name string) (*longhorn.SnapshotInfo, error)
	SnapshotDelete(engine *longhorn.Engine, name string) error
	SnapshotRevert(engine *longhorn.Engine, name string) error
	SnapshotPurge(engine *longhorn.Engine) error
	SnapshotPurgeStatus(engine *longhorn.Engine) (map[string]*longhorn.PurgeStatus, error)
	SnapshotBackup(engine *longhorn.Engine, snapshotName, backupName, backupTarget, backingImageName, backingImageChecksum, compressionMethod string, concurrentLimit int, storageClassName string, labels, credential, parameters map[string]string) (string, string, error)
	SnapshotBackupStatus(engine *longhorn.Engine, backupName, replicaAddress, replicaName string) (*longhorn.EngineBackupStatus, error)
	SnapshotCloneStatus(engine *longhorn.Engine) (map[string]*longhorn.SnapshotCloneStatus, error)
	SnapshotClone(engine *longhorn.Engine, snapshotName, fromEngineAddress, fromVolumeName, fromEngineName string, fileSyncHTTPClientTimeout, grpcTimeoutSeconds int64) error
	SnapshotHash(engine *longhorn.Engine, snapshotName string, rehash bool) error
	SnapshotHashStatus(engine *longhorn.Engine, snapshotName string) (map[string]*longhorn.HashStatus, error)

	BackupRestore(engine *longhorn.Engine, backupTarget, backupName, backupVolume, lastRestored string, credential map[string]string, concurrentLimit int) error
	BackupRestoreStatus(engine *longhorn.Engine) (map[string]*longhorn.RestoreStatus, error)

	SPDKBackingImageCreate(name, backingImageUUID, diskUUID, checksum, fromAddress, srcDiskUUID string, size uint64) (*imapi.BackingImage, error)
	SPDKBackingImageDelete(name, diskUUID string) error
	SPDKBackingImageGet(name, diskUUID string) (*imapi.BackingImage, error)
	SPDKBackingImageList() (map[string]longhorn.BackingImageV2CopyInfo, error)
	SPDKBackingImageWatch(ctx context.Context) (*imapi.BackingImageStream, error)

	CleanupBackupMountPoints() error

	MetricsGet(engine *longhorn.Engine) (*Metrics, error)
	RemountReadOnlyVolume(engine *longhorn.Engine) error
}

type EngineClientRequest struct {
	VolumeName   string
	EngineImage  string
	IP           string
	Port         int
	InstanceName string
}

type EngineClientCollection interface {
	NewEngineClient(request *EngineClientRequest) (*EngineBinary, error)
}

type Volume struct {
	Name                      string `json:"name"`
	Size                      int64  `json:"size"`
	ReplicaCount              int    `json:"replicaCount"`
	Endpoint                  string `json:"endpoint"`
	Frontend                  string `json:"frontend"`
	FrontendState             string `json:"frontendState"`
	IsExpanding               bool   `json:"isExpanding"`
	LastExpansionError        string `json:"lastExpansionError"`
	LastExpansionFailedAt     string `json:"lastExpansionFailedAt"`
	UnmapMarkSnapChainRemoved bool   `json:"unmapMarkSnapChainRemoved"`
	SnapshotMaxCount          int    `json:"snapshotMaxCount"`
	SnapshotMaxSize           int64  `json:"SnapshotMaxSize"`
}

type BackupTarget struct {
	Name             string `json:"name"`
	BackupTargetURL  string `json:"backupTargetURL"`
	CredentialSecret string `json:"credentialSecret"`
	PollInterval     string `json:"pollInterval"`
	Available        bool   `json:"available"`
	Message          string `json:"message"`
}

type BackupVolume struct {
	Name                 string             `json:"name"`
	Size                 string             `json:"size"`
	Labels               map[string]string  `json:"labels"`
	Created              string             `json:"created"`
	LastBackupName       string             `json:"lastBackupName"`
	LastBackupAt         string             `json:"lastBackupAt"`
	DataStored           string             `json:"dataStored"`
	Messages             map[string]string  `json:"messages"`
	Backups              map[string]*Backup `json:"backups"`
	BackingImageName     string             `json:"backingImageName"`
	BackingImageChecksum string             `json:"backingImageChecksum"`
	StorageClassName     string             `json:"storageClassName"`
	BackupTargetName     string             `json:"backupTargetName"`
	VolumeName           string             `json:"volumeName"`
}

type Backup struct {
	Name                   string               `json:"name"`
	State                  longhorn.BackupState `json:"state"`
	URL                    string               `json:"url"`
	SnapshotName           string               `json:"snapshotName"`
	SnapshotCreated        string               `json:"snapshotCreated"`
	Created                string               `json:"created"`
	Size                   string               `json:"size"`
	Labels                 map[string]string    `json:"labels"`
	VolumeName             string               `json:"volumeName"`
	VolumeSize             string               `json:"volumeSize"`
	VolumeCreated          string               `json:"volumeCreated"`
	VolumeBackingImageName string               `json:"volumeBackingImageName"`
	Messages               map[string]string    `json:"messages"`
	CompressionMethod      string               `json:"compressionMethod"`
	Parameters             map[string]string    `json:"parameters"`
	NewlyUploadedDataSize  string               `json:"newlyUploadedDataSize"`
	ReUploadedDataSize     string               `json:"reUploadedDataSize"`
	BackupTargetName       string               `json:"backupTargetName"`
}

type ConfigMetadata struct {
	ModificationTime time.Time `json:"modificationTime"`
}

type BackupCreateInfo struct {
	BackupID       string
	ReplicaAddress string
	IsIncremental  bool
}

type LauncherVolumeInfo struct {
	Volume   string `json:"volume,omitempty"`
	Frontend string `json:"frontend,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

type EngineVersion struct {
	ClientVersion *longhorn.EngineVersionDetails `json:"clientVersion"`
	ServerVersion *longhorn.EngineVersionDetails `json:"serverVersion"`
}

type TaskError struct {
	ReplicaErrors []ReplicaError
}

type ReplicaError struct {
	Address string
	Message string
}

func (e TaskError) Error() string {
	var errs []string
	for _, re := range e.ReplicaErrors {
		errs = append(errs, re.Error())
	}

	if errs == nil {
		return "Unknown"
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return strings.Join(errs, "; ")
}

func (e ReplicaError) Error() string {
	return fmt.Sprintf("%v: %v", e.Address, e.Message)
}

func GetBackendReplicaURL(address string) string {
	return "tcp://" + address
}

func GetAddressFromBackendReplicaURL(url string) string {
	// tcp://<address>:<Port>
	return strings.TrimPrefix(url, "tcp://")
}

func ValidateReplicaURL(url string) error {
	if !strings.HasPrefix(url, "tcp://") {
		return fmt.Errorf("invalid replica url %v", url)
	}
	return nil
}

func CheckCLICompatibility(cliVersion, cliMinVersion int) error {
	currentCLIVersion := emeta.CLIAPIVersion
	minCLIVersion := emeta.CLIAPIMinVersion

	if minCLIVersion > cliVersion || currentCLIVersion < cliMinVersion {
		return fmt.Errorf("manager current CLI version %v and min CLI version %v is not compatible with CLIVersion %v and CLIMinVersion %v", currentCLIVersion, minCLIVersion, cliVersion, cliMinVersion)
	}

	return nil
}

func GetEngineInstanceFrontend(dataEngine longhorn.DataEngineType, volumeFrontend longhorn.VolumeFrontend) (frontend string, err error) {
	switch volumeFrontend {
	case longhorn.VolumeFrontendBlockDev:
		frontend = string(iscsidevtypes.FrontendTGTBlockDev)
		if types.IsDataEngineV2(dataEngine) {
			frontend = string(spdkdevtypes.FrontendSPDKTCPBlockdev)
		}
	case longhorn.VolumeFrontendISCSI:
		frontend = string(iscsidevtypes.FrontendTGTISCSI)
	case longhorn.VolumeFrontendNvmf:
		frontend = string(spdkdevtypes.FrontendSPDKTCPNvmf)
	case longhorn.VolumeFrontendEmpty:
		frontend = ""
	default:
		err = fmt.Errorf("unknown volume frontend %v", volumeFrontend)
	}

	return frontend, err
}

func GetEngineEndpoint(volume *Volume, ip string) (string, error) {
	if volume == nil || volume.Frontend == "" {
		return "", nil
	}

	switch volume.Frontend {
	case iscsidevtypes.FrontendTGTBlockDev, spdkdevtypes.FrontendSPDKTCPBlockdev:
		return volume.Endpoint, nil
	case iscsidevtypes.FrontendTGTISCSI:
		if ip == "" {
			return "", fmt.Errorf("iscsi endpoint %v is missing ip", volume.Endpoint)
		}

		// it will looks like this in the end
		// iscsi://10.42.0.12:3260/iqn.2014-09.com.rancher:vol-name/1
		return EndpointISCSIPrefix + ip + ":" + DefaultISCSIPort + "/" + volume.Endpoint + "/" + DefaultISCSILUN, nil
	case spdkdevtypes.FrontendSPDKTCPNvmf:
		return volume.Endpoint, nil
	}

	return "", fmt.Errorf("unknown frontend %v", volume.Frontend)
}

func IsEndpointTGTBlockDev(endpoint string) bool {
	if endpoint == "" {
		return false
	}

	if strings.Contains(endpoint, EndpointISCSIPrefix) {
		return false
	}

	return true
}
