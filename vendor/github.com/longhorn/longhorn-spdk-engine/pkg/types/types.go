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
	InstanceStatePending     = "pending"
	InstanceStateStopped     = "stopped"
	InstanceStateRunning     = "running"
	InstanceStateTerminating = "terminating"
	InstanceStateError       = "error"
)

type InstanceType string

const (
	InstanceTypeReplica = InstanceType("replica")
	InstanceTypeEngine  = InstanceType("engine")
)

const VolumeHead = "volume-head"

const SPDKServicePort = 8504
