package types

import (
	"io"
	"strings"
	"time"

	"github.com/longhorn/types/pkg/generated/enginerpc"
)

const (
	WO  = Mode("WO")
	RW  = Mode("RW")
	ERR = Mode("ERR")

	ProcessStateComplete   = ProcessState("complete")
	ProcessStateError      = ProcessState("error")
	ProcessStateInProgress = ProcessState("in_progress")

	StateUp   = State("up")
	StateDown = State("down")

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

	RetryCounts   = 30
	RetryInterval = 1 * time.Second

	EngineFrontendBlockDev = "tgt-blockdev"
	EngineFrontendISCSI    = "tgt-iscsi"

	VolumeHeadName = "volume-head"

	MaximumTotalSnapshotCount = 250
)

type DataServerProtocol string

const (
	DataServerProtocolTCP  = DataServerProtocol("tcp")
	DataServerProtocolUNIX = DataServerProtocol("unix")
)

type ReplicaState string

const (
	ReplicaStateInitial    = ReplicaState("initial")
	ReplicaStateOpen       = ReplicaState("open")
	ReplicaStateClosed     = ReplicaState("closed")
	ReplicaStateDirty      = ReplicaState("dirty")
	ReplicaStateRebuilding = ReplicaState("rebuilding")
	ReplicaStateError      = ReplicaState("error")
)

type ReaderWriterUnmapperAt interface {
	io.ReaderAt
	io.WriterAt
	UnmapperAt
}

type UnmapperAt interface {
	UnmapAt(length uint32, off int64) (n int, err error)
}

type DiffDisk interface {
	ReaderWriterUnmapperAt
	io.Closer
	Fd() uintptr
	Size() (int64, error)
}

type MonitorChannel chan error

type Backend interface {
	ReaderWriterUnmapperAt
	io.Closer
	Snapshot(name string, userCreated bool, created string, labels map[string]string) error
	Expand(size int64) error
	Size() (int64, error)
	SectorSize() (int64, error)
	GetRevisionCounter() (int64, error)
	SetRevisionCounter(counter int64) error
	GetState() (string, error)
	GetMonitorChannel() MonitorChannel
	StopMonitoring()
	IsRevisionCounterDisabled() (bool, error)
	GetLastModifyTime() (int64, error)
	GetHeadFileSize() (int64, error)
	GetUnmapMarkSnapChainRemoved() (bool, error)
	SetUnmapMarkSnapChainRemoved(enabled bool) error
	ResetRebuild() error
	SetSnapshotMaxCount(count int) error
	SetSnapshotMaxSize(size int64) error
	GetSnapshotCountAndSizeUsage() (int, int, int64, error)
}

type BackendFactory interface {
	Create(volumeName, address string, dataServerProtocol DataServerProtocol,
		sharedTimeouts SharedTimeouts) (Backend, error)
}

type SharedTimeouts interface {
	Increment()
	Decrement()
	CheckAndDecrement(duration time.Duration) time.Duration
}

type Controller interface {
	AddReplica(address string) error
	RemoveReplica(address string) error
	SetReplicaMode(address string, mode Mode) error
	ListReplicas() []Replica
	Start(address ...string) error
	Shutdown() error
}

type Server interface {
	ReaderWriterUnmapperAt
	Controller
}

type Mode string

type ProcessState string

type State string

type Replica struct {
	Address string
	Mode    Mode
}

type ReplicaSalvageInfo struct {
	Address        string
	LastModifyTime time.Time
	HeadFileSize   int64
}

type Frontend interface {
	FrontendName() string
	Init(name string, size, sectorSize int64) error
	Startup(rwu ReaderWriterUnmapperAt) error
	Shutdown() error
	State() State
	Endpoint() string
	Upgrade(name string, size, sectorSize int64, rwu ReaderWriterUnmapperAt) error
	Expand(size int64) error
}

type DataProcessor interface {
	ReaderWriterUnmapperAt
	PingResponse() error
}

const (
	EventTypeVolume  = "volume"
	EventTypeReplica = "replica"
	EventTypeMetrics = "metrics"
)

type Metrics struct {
	Throughput   RWMetrics // in byte
	TotalLatency RWMetrics // in nanoseconds
	IOPS         RWMetrics
}

type RWMetrics struct {
	Read  uint64
	Write uint64
}

func IsAlreadyPurgingError(err error) bool {
	return strings.Contains(err.Error(), "already purging")
}

func ReplicaModeToGRPCReplicaMode(mode Mode) enginerpc.ReplicaMode {
	switch mode {
	case WO:
		return enginerpc.ReplicaMode_WO
	case RW:
		return enginerpc.ReplicaMode_RW
	case ERR:
		return enginerpc.ReplicaMode_ERR
	}
	return enginerpc.ReplicaMode_ERR
}

func GRPCReplicaModeToReplicaMode(replicaMode enginerpc.ReplicaMode) Mode {
	switch replicaMode {
	case enginerpc.ReplicaMode_WO:
		return WO
	case enginerpc.ReplicaMode_RW:
		return RW
	case enginerpc.ReplicaMode_ERR:
		return ERR
	}
	return ERR
}

type FileLocalSync struct {
	SourcePath string
	TargetPath string
}
