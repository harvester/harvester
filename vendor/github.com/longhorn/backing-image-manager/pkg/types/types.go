package types

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	BackingImageManagerDirectoryName = "backing-images"
	DiskPathInContainer              = "/data/"
	DataSourceDirectoryName          = "/tmp/"

	DefaultSectorSize = 512

	DefaultManagerPort              = 8000
	DefaultDataSourceServerPort     = 8000
	DefaultSyncServerPort           = 8001
	DefaultVolumeExportReceiverPort = 8002

	GRPCServiceTimeout = 1 * time.Minute
	HTTPTimeout        = 4 * time.Second
	MonitorInterval    = 3 * time.Second
	FileSyncTimeout    = 120

	SendingLimit = 3

	BackingImageFileName    = "backing"
	TmpFileSuffix           = ".tmp"
	BackingImageTmpFileName = BackingImageFileName + TmpFileSuffix
)

type State string

const (
	StatePending          = State("pending")
	StateStarting         = State("starting")
	StateInProgress       = State("in-progress")
	StateFailed           = State("failed")
	StateUnknown          = State("unknown")
	StateReady            = State("ready")
	StateReadyForTransfer = State("ready-for-transfer")
)

type DataSourceType string

const (
	DataSourceTypeDownload         = DataSourceType("download")
	DataSourceTypeUpload           = DataSourceType("upload")
	DataSourceTypeExportFromVolume = DataSourceType("export-from-volume")
)

const (
	DataSourceTypeDownloadParameterURL = "url"
	DataSourceTypeFileType             = "file-type"

	DataSourceTypeExportFromVolumeParameterVolumeSize    = "volume-size"
	DataSourceTypeExportFromVolumeParameterSnapshotName  = "snapshot-name"
	DataSourceTypeExportFromVolumeParameterSenderAddress = "sender-address"

	DataSourceTypeExportFromVolumeParameterExportTypeRAW   = "raw"
	DataSourceTypeExportFromVolumeParameterExportTypeQCOW2 = "qcow2"

	SyncingFileTypeEmpty = ""
	SyncingFileTypeRaw   = "raw"
	SyncingFileTypeQcow2 = "qcow2"
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
