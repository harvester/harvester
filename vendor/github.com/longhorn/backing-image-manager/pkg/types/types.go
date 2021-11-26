package types

import (
	"time"
)

const (
	BackingImageManagerDirectoryName = "backing-images"
	DiskPathInContainer              = "/data/"
	DataSourceDirectoryName          = "/tmp/"

	DefaultSectorSize = 512

	DefaultManagerPort          = 8000
	DefaultDataSourceServerPort = 8001

	GRPCServiceTimeout     = 3 * time.Minute
	HTTPTimeout            = 4 * time.Second
	FileValidationInterval = 5 * time.Second
	FileSyncTimeout        = 120

	SendingLimit = 3

	BackingImageTmpFileName = "backing.tmp"
	BackingImageFileName    = "backing"
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
	DataSourceTypeDownload = DataSourceType("download")
	DataSourceTypeUpload   = DataSourceType("upload")
)

const (
	DataSourceTypeDownloadParameterURL = "url"
)
