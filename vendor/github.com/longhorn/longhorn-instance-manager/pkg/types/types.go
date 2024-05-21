package types

import (
	"time"
)

const (
	GRPCServiceTimeout = 3 * time.Minute

	ProcessStateRunning  = "running"
	ProcessStateStarting = "starting"
	ProcessStateStopped  = "stopped"
	ProcessStateStopping = "stopping"
	ProcessStateError    = "error"

	DiskGrpcService           = "disk gRPC server"
	SpdkGrpcService           = "spdk gRPC server"
	ProcessManagerGrpcService = "process-manager gRPC server"
	InstanceGrpcService       = "instance gRPC server"
	ProxyGRPCService          = "proxy gRPC server"
)

const (
	InstanceManagerProcessManagerServiceDefaultPort = 8500
	InstanceManagerProxyServiceDefaultPort          = InstanceManagerProcessManagerServiceDefaultPort + 1 // 8501
	InstanceManagerDiskServiceDefaultPort           = InstanceManagerProcessManagerServiceDefaultPort + 2 // 8502
	InstanceManagerInstanceServiceDefaultPort       = InstanceManagerProcessManagerServiceDefaultPort + 3 // 8503
	InstanceManagerSpdkServiceDefaultPort           = InstanceManagerProcessManagerServiceDefaultPort + 4 // 8504
)

var (
	WaitInterval = 100 * time.Millisecond
	WaitCount    = 600
)

const (
	RetryInterval = 3 * time.Second
	RetryCounts   = 3
)

const (
	InstanceTypeEngine  = "engine"
	InstanceTypeReplica = "replica"
)

const (
	EngineConditionFilesystemReadOnly = "FilesystemReadOnly"
)
