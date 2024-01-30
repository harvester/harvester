package spdkrpc

import (
	"github.com/longhorn/longhorn-spdk-engine/pkg/types"
)

func ReplicaModeToGRPCReplicaMode(mode types.Mode) ReplicaMode {
	switch mode {
	case types.ModeWO:
		return ReplicaMode_WO
	case types.ModeRW:
		return ReplicaMode_RW
	case types.ModeERR:
		return ReplicaMode_ERR
	}
	return ReplicaMode_ERR
}

func GRPCReplicaModeToReplicaMode(replicaMode ReplicaMode) types.Mode {
	switch replicaMode {
	case ReplicaMode_WO:
		return types.ModeWO
	case ReplicaMode_RW:
		return types.ModeRW
	case ReplicaMode_ERR:
		return types.ModeERR
	}
	return types.ModeERR
}
