package api

import (
	"github.com/golang/protobuf/ptypes/empty"

	"github.com/longhorn/longhorn-spdk-engine/proto/spdkrpc"

	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
)

type InstanceProcessSpec struct {
	Binary string   `json:"binary"`
	Args   []string `json:"args"`
}

type Instance struct {
	Name               string         `json:"name"`
	Type               string         `json:"type"`
	BackendStoreDriver string         `json:"backendStoreDriver"`
	PortCount          int32          `json:"portCount"`
	PortArgs           []string       `json:"portArgs"`
	InstanceStatus     InstanceStatus `json:"instanceStatus"`
	Deleted            bool           `json:"deleted"`

	InstanceProccessSpec *InstanceProcessSpec
}

func RPCToInstance(obj *rpc.InstanceResponse) *Instance {
	instance := &Instance{
		Name:               obj.Spec.Name,
		Type:               obj.Spec.Type,
		BackendStoreDriver: obj.Spec.BackendStoreDriver.String(),
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
	State     string `json:"state"`
	ErrorMsg  string `json:"errorMsg"`
	PortStart int32  `json:"portStart"`
	PortEnd   int32  `json:"portEnd"`
}

func RPCToInstanceStatus(obj *rpc.InstanceStatus) InstanceStatus {
	return InstanceStatus{
		State:     obj.State,
		ErrorMsg:  obj.ErrorMsg,
		PortStart: obj.PortStart,
		PortEnd:   obj.PortEnd,
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

func (s *InstanceStream) Recv() (*empty.Empty, error) {
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

func (s *ReplicaStream) Recv() (*empty.Empty, error) {
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

func (s *EngineStream) Recv() (*empty.Empty, error) {
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
