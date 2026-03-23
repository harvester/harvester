package api

import (
	rpc "github.com/longhorn/types/pkg/generated/imrpc"
	"github.com/longhorn/types/pkg/generated/spdkrpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	dataEngines = map[string]string{
		"DATA_ENGINE_V1": "v1",
		"DATA_ENGINE_V2": "v2",
	}
)

type InstanceProcessSpec struct {
	Binary string   `json:"binary"`
	Args   []string `json:"args"`
}

type Instance struct {
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	DataEngine     string         `json:"dataEngine"`
	PortCount      int32          `json:"portCount"`
	PortArgs       []string       `json:"portArgs"`
	InstanceStatus InstanceStatus `json:"instanceStatus"`
	Deleted        bool           `json:"deleted"`

	InstanceProccessSpec *InstanceProcessSpec

	// Deprecated: replaced by DataEngine.
	BackendStoreDriver string `json:"backendStoreDriver"`
}

func RPCToInstance(obj *rpc.InstanceResponse) *Instance {
	instance := &Instance{
		Name: obj.Spec.Name,
		Type: obj.Spec.Type,
		//lint:ignore SA1019 replaced with DataEngine
		BackendStoreDriver: obj.Spec.BackendStoreDriver.String(), // nolint: staticcheck
		DataEngine:         dataEngines[obj.Spec.DataEngine.String()],
		PortCount:          obj.Spec.PortCount,
		PortArgs:           obj.Spec.PortArgs,
		InstanceStatus:     RPCToInstanceStatus(obj.Status),
	}

	if obj.Spec.ProcessInstanceSpec != nil {
		instance.InstanceProccessSpec = &InstanceProcessSpec{
			Binary: obj.Spec.ProcessInstanceSpec.Binary,
			Args:   obj.Spec.ProcessInstanceSpec.Args,
		}
	}

	return instance
}

func RPCToInstanceList(obj *rpc.InstanceListResponse) map[string]*Instance {
	ret := map[string]*Instance{}
	for name, p := range obj.Instances {
		ret[name] = RPCToInstance(p)
	}
	return ret
}

type InstanceStatus struct {
	State                  string          `json:"state"`
	ErrorMsg               string          `json:"errorMsg"`
	Conditions             map[string]bool `json:"conditions"`
	PortStart              int32           `json:"portStart"`
	PortEnd                int32           `json:"portEnd"`
	TargetPortStart        int32           `json:"targetPortStart"`
	TargetPortEnd          int32           `json:"targetPortEnd"`
	StandbyTargetPortStart int32           `json:"standbyTargetPortStart"`
	StandbyTargetPortEnd   int32           `json:"standbyTargetPortEnd"`
	UblkID                 int32           `json:"ublk_id"`
	UUID                   string          `json:"uuid"`
}

func RPCToInstanceStatus(obj *rpc.InstanceStatus) InstanceStatus {
	return InstanceStatus{
		State:                  obj.State,
		ErrorMsg:               obj.ErrorMsg,
		Conditions:             obj.Conditions,
		PortStart:              obj.PortStart,
		PortEnd:                obj.PortEnd,
		TargetPortStart:        obj.TargetPortStart,
		TargetPortEnd:          obj.TargetPortEnd,
		StandbyTargetPortStart: obj.StandbyTargetPortStart,
		StandbyTargetPortEnd:   obj.StandbyTargetPortEnd,
		UblkID:                 obj.UblkId,
		UUID:                   obj.Uuid,
	}
}

type InstanceStream struct {
	stream rpc.InstanceService_InstanceWatchClient
}

func NewInstanceStream(stream rpc.InstanceService_InstanceWatchClient) *InstanceStream {
	return &InstanceStream{
		stream,
	}
}

func (s *InstanceStream) Recv() (*emptypb.Empty, error) {
	return s.stream.Recv()
}

type ReplicaStream struct {
	stream spdkrpc.SPDKService_ReplicaWatchClient
}

func NewReplicaStream(stream spdkrpc.SPDKService_ReplicaWatchClient) *ReplicaStream {
	return &ReplicaStream{
		stream,
	}
}

func (s *ReplicaStream) Recv() (*emptypb.Empty, error) {
	return s.stream.Recv()
}

type EngineStream struct {
	stream spdkrpc.SPDKService_EngineWatchClient
}

func NewEngineStream(stream spdkrpc.SPDKService_EngineWatchClient) *EngineStream {
	return &EngineStream{
		stream,
	}
}

func (s *EngineStream) Recv() (*emptypb.Empty, error) {
	return s.stream.Recv()
}

func NewLogStream(stream rpc.ProcessManagerService_ProcessLogClient) *LogStream {
	return &LogStream{
		stream,
	}
}

type LogStream struct {
	stream rpc.ProcessManagerService_ProcessLogClient
}

func (s *LogStream) Recv() (string, error) {
	resp, err := s.stream.Recv()
	if err != nil {
		return "", err
	}
	return resp.Line, nil
}
