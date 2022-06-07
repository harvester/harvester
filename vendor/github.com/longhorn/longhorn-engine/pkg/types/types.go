package types

import (
	"io"
	"strings"
	"time"
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

	HTTPSProxy = "HTTPS_PROXY"
	HTTPProxy  = "HTTP_PROXY"
	NOProxy    = "NO_PROXY"

	VirtualHostedStyle = "VIRTUAL_HOSTED_STYLE"

	RetryCounts   = 30
	RetryInterval = 1 * time.Second

	EngineFrontendBlockDev = "tgt-blockdev"
	EngineFrontendISCSI    = "tgt-iscsi"
)

type ReaderWriterAt interface {
	io.ReaderAt
	io.WriterAt
}

type DiffDisk interface {
	ReaderWriterAt
	io.Closer
	Fd() uintptr
	Size() (int64, error)
}

type MonitorChannel chan error

type Backend interface {
	ReaderWriterAt
	io.Closer
	Snapshot(name string, userCreated bool, created string, labels map[string]string) error
	Expand(size int64) error
	Size() (int64, error)
	SectorSize() (int64, error)
	RemainSnapshots() (int, error)
	GetRevisionCounter() (int64, error)
	SetRevisionCounter(counter int64) error
	GetMonitorChannel() MonitorChannel
	StopMonitoring()
	IsRevisionCounterDisabled() (bool, error)
	GetLastModifyTime() (int64, error)
	GetHeadFileSize() (int64, error)
}

type BackendFactory interface {
	Create(address string) (Backend, error)
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
	ReaderWriterAt
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
	LastModifyTime int64
	HeadFileSize   int64
}

type Frontend interface {
	FrontendName() string
	Init(name string, size, sectorSize int64) error
	Startup(rw ReaderWriterAt) error
	Shutdown() error
	State() State
	Endpoint() string
	Upgrade(name string, size, sectorSize int64, rw ReaderWriterAt) error
	Expand(size int64) error
}

type DataProcessor interface {
	ReaderWriterAt
	PingResponse() error
}

const (
	EventTypeVolume  = "volume"
	EventTypeReplica = "replica"
	EventTypeMetrics = "metrics"
)

type Metrics struct {
	Bandwidth    RWMetrics // in byte
	TotalLatency RWMetrics // in microsecond(us)
	IOPS         RWMetrics
}

type RWMetrics struct {
	Read  uint64
	Write uint64
}

func IsAlreadyPurgingError(err error) bool {
	return strings.Contains(err.Error(), "already purging")
}
