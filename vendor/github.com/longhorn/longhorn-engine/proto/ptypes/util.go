package ptypes

import (
	"github.com/longhorn/longhorn-engine/pkg/types"
)

func ReplicaModeToGRPCReplicaMode(mode types.Mode) ReplicaMode {
	switch mode {
	case types.WO:
		return ReplicaMode_WO
	case types.RW:
		return ReplicaMode_RW
	case types.ERR:
		return ReplicaMode_ERR
	}
	return ReplicaMode_ERR
}

func GRPCReplicaModeToReplicaMode(replicaMode ReplicaMode) types.Mode {
	switch replicaMode {
	case ReplicaMode_WO:
		return types.WO
	case ReplicaMode_RW:
		return types.RW
	case ReplicaMode_ERR:
		return types.ERR
	}
	return types.ERR
}
