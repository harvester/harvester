package types

type ProgressState string

const (
	ProgressStateInProgress = ProgressState("in_progress")
	ProgressStateComplete   = ProgressState("complete")
	ProgressStateError      = ProgressState("error")
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
