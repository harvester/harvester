package client

import (
	"fmt"
	"strings"

	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
)

const (
	dataEngineV1 = "v1"
	dataEngineV2 = "v2"
)

type TaskError struct {
	ReplicaErrors []ReplicaError
}

type ReplicaError struct {
	Address string
	Message string
}

func (e TaskError) Error() string {
	var errs []string
	for _, re := range e.ReplicaErrors {
		errs = append(errs, re.Error())
	}

	if errs == nil {
		return "Unknown"
	}

	return strings.Join(errs, "; ")
}

func (e ReplicaError) Error() string {
	return fmt.Sprintf("%v: %v", e.Address, e.Message)
}

func getDataEngine(dataEngine string) string {
	if strings.HasSuffix(strings.ToLower(dataEngine), dataEngineV2) {
		return rpc.DataEngine_name[int32(rpc.DataEngine_DATA_ENGINE_V2)]
	}

	return rpc.DataEngine_name[int32(rpc.DataEngine_DATA_ENGINE_V1)]
}
