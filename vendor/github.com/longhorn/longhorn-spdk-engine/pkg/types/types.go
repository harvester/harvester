package types

type Mode string

const (
	ModeWO  = Mode("WO")
	ModeRW  = Mode("RW")
	ModeERR = Mode("ERR")
)

const (
	FrontendSPDKTCPNvmf     = "spdk-tcp-nvmf"
	FrontendSPDKTCPBlockdev = "spdk-tcp-blockdev"
	FrontendEmpty           = ""
)

type InstanceState string

const (
	InstanceStatePending = "pending"
	InstanceStateStopped = "stopped"
	InstanceStateRunning = "running"
	InstanceStateError   = "error"
)

type InstanceType string

const (
	InstanceTypeReplica = InstanceType("replica")
	InstanceTypeEngine  = InstanceType("engine")
)

const SPDKServicePort = 8504
