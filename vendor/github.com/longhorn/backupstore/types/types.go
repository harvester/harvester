package types

type ProgressState string

const (
	ProgressStateUndefined  = ProgressState("")
	ProgressStateInProgress = ProgressState("in_progress")
	ProgressStateComplete   = ProgressState("complete")
	ProgressStateError      = ProgressState("error")
	ProgressStateCanceled   = ProgressState("canceled")
)

const (
	AWSAccessKey = "AWS_ACCESS_KEY_ID"
	AWSSecretKey = "AWS_SECRET_ACCESS_KEY"
	AWSEndPoint  = "AWS_ENDPOINTS"
	AWSCert      = "AWS_CERT"

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
)

type Mapping struct {
	Offset int64
	Size   int64
}

type Mappings struct {
	Mappings  []Mapping
	BlockSize int64
}

type MessageType string

const (
	MessageTypeError = MessageType("error")
)

type JobResult struct {
	Payload interface{}
	Err     error
}

const (
	// This is used for all the BackingImages since they all share the same block pool.
	// For lock mechanism, please refer to: https://github.com/longhorn/longhorn/blob/master/enhancements/20200701-backupstore-file-locks.md#proposal
	// Currently the lock file is stored in each BackupVolume folder.
	// For BackingImage Lock it is also stored there with the folder name "BACKINGIMAGE" defined here.
	// To prevent Longhorn from accidentally considering it as another normal BackupVolume,
	// we use uppercase here so it will be filtered out when listing.
	BackupBackingImageLockName = "BACKINGIMAGE"
)

const (
	ErrorMsgRestoreCancelled = "backup restoration is cancelled"
)
