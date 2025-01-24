package types

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	BackingImageManagerDirectoryName = "backing-images"
	DiskPathInContainer              = "/data/"
	DataSourceDirectoryName          = "/tmp/"

	DefaultSectorSize     = 512
	DefaultLinuxBlcokSize = 512

	DefaultManagerPort              = 8000
	DefaultDataSourceServerPort     = 8000
	DefaultSyncServerPort           = 8001
	DefaultVolumeExportReceiverPort = 8002

	GRPCServiceTimeout      = 1 * time.Minute
	HTTPTimeout             = 4 * time.Second
	MonitorInterval         = 3 * time.Second
	CommandExecutionTimeout = 10 * time.Second

	FileSyncHTTPClientTimeout = 5 // TODO: use 5 seconds as default, need to refactor it

	SendingLimit = 3

	BackingImageFileName    = "backing"
	TmpFileSuffix           = ".tmp"
	BackingImageTmpFileName = BackingImageFileName + TmpFileSuffix

	MapperFilePathPrefix = "/dev/mapper"
	EncryptionMetaSize   = 16 * 1024 * 1024 // 16MB
)

type State string

const (
	StatePending          = State("pending")
	StateStarting         = State("starting")
	StateInProgress       = State("in-progress")
	StateFailed           = State("failed")
	StateFailedAndCleanUp = State("failed-and-cleanup")
	StateUnknown          = State("unknown")
	StateReady            = State("ready")
	StateReadyForTransfer = State("ready-for-transfer")
)

type DataSourceType string

const (
	DataSourceTypeDownload         = DataSourceType("download")
	DataSourceTypeUpload           = DataSourceType("upload")
	DataSourceTypeExportFromVolume = DataSourceType("export-from-volume")
	DataSourceTypeRestore          = DataSourceType("restore")
	DataSourceTypeClone            = DataSourceType("clone")
)

const (
	DataSourceTypeCloneParameterBackingImage     = "backing-image"
	DataSourceTypeCloneParameterBackingImageUUID = "backing-image-uuid"
	DataSourceTypeCloneParameterEncryption       = "encryption"

	DataSourceTypeDownloadParameterURL            = "url"
	DataSourceTypeRestoreParameterBackupURL       = "backup-url"
	DataSourceTypeRestoreParameterConcurrentLimit = "concurrent-limit"
	DataSourceTypeFileType                        = "file-type"
	DataSourceTypeParameterDataEngine             = "data-engine"
	DataEnginev1                                  = "v1"
	DataEnginev2                                  = "v2"

	DataSourceTypeExportFromVolumeParameterVolumeSize                = "volume-size"
	DataSourceTypeExportFromVolumeParameterSnapshotName              = "snapshot-name"
	DataSourceTypeExportFromVolumeParameterSenderAddress             = "sender-address"
	DataSourceTypeExportFromVolumeParameterFileSyncHTTPClientTimeout = "file-sync-http-client-timeout"

	DataSourceTypeExportFromVolumeParameterExportTypeRAW   = "raw"
	DataSourceTypeExportFromVolumeParameterExportTypeQCOW2 = "qcow2"

	SyncingFileTypeEmpty = ""
	SyncingFileTypeRaw   = "raw"
	SyncingFileTypeQcow2 = "qcow2"
)

type EncryptionType string

const (
	EncryptionTypeEncrypt = EncryptionType("encrypt")
	EncryptionTypeDecrypt = EncryptionType("decrypt")
	EncryptionTypeIgnore  = EncryptionType("ignore")
)

func GetDataSourceFileName(biName, biUUID string) string {
	return fmt.Sprintf("%s-%s", biName, biUUID)
}

func GetDataSourceFilePath(diskPath, biName, biUUID string) string {
	return filepath.Join(diskPath, DataSourceDirectoryName, GetDataSourceFileName(biName, biUUID))
}

func GetBackingImageDirectoryName(biName, biUUID string) string {
	return fmt.Sprintf("%s-%s", biName, biUUID)
}

func GetBackingImageDirectory(diskPath, biName, biUUID string) string {
	return filepath.Join(diskPath, BackingImageManagerDirectoryName, GetBackingImageDirectoryName(biName, biUUID))
}

func GetBackingImageFilePath(diskPath, biName, biUUID string) string {
	return filepath.Join(GetBackingImageDirectory(diskPath, biName, biUUID), BackingImageFileName)
}

func GetBackingImageNameFromFilePath(biFilePath, biUUID string) string {
	biDirName := filepath.Join(filepath.Base(filepath.Dir(biFilePath)))
	return strings.TrimSuffix(biDirName, "-"+biUUID)
}

func BackingImageMapper(uuid string) string {
	return path.Join(MapperFilePathPrefix, GetLuksBackingImageName(uuid))
}

func GetLuksBackingImageName(uuid string) string {
	return fmt.Sprintf("%v-%v", BackingImageFileName, uuid)
}
