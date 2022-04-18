package types

import (
	"time"
)

const (
	BackingImageManagerDirectoryName = "backing-images"
	DiskPathInContainer              = "/data/"
	DataSourceDirectoryName          = "/tmp/"

	EnvPodIP = "POD_IP"

	DefaultSectorSize = 512

	DefaultManagerPort              = 8000
	DefaultDataSourceServerPort     = 8001
	DefaultVolumeExportReceiverPort = 8002

	GRPCServiceTimeout     = 3 * time.Minute
	HTTPTimeout            = 4 * time.Second
	FileValidationInterval = 5 * time.Second
	FileSyncTimeout        = 120

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
	StateReadyForTransfer = State("ready-for-transfer")
	StateReady            = State("ready")
	StateFailed           = State("failed")
)

type DataSourceType string

const (
	DataSourceTypeDownload         = DataSourceType("download")
	DataSourceTypeUpload           = DataSourceType("upload")
	DataSourceTypeExportFromVolume = DataSourceType("export-from-volume")
)

const (
	DataSourceTypeDownloadParameterURL                   = "url"
	DataSourceTypeExportFromVolumeParameterVolumeName    = "volume-name"
	DataSourceTypeExportFromVolumeParameterVolumeSize    = "volume-size"
	DataSourceTypeExportFromVolumeParameterSnapshotName  = "snapshot-name"
	DataSourceTypeExportFromVolumeParameterSenderAddress = "sender-address"
	DataSourceTypeExportFromVolumeParameterExportType    = "export-type"

	DataSourceTypeExportFromVolumeParameterExportTypeRAW   = "raw"
	DataSourceTypeExportFromVolumeParameterExportTypeQCOW2 = "qcow2"
)
