package api

import (
	rpc "github.com/longhorn/types/pkg/generated/imrpc"
)

type Process struct {
	Name      string   `json:"name"`
	Binary    string   `json:"binary"`
	Args      []string `json:"args"`
	PortCount int32    `json:"portCount"`
	PortArgs  []string `json:"portArgs"`

	ProcessStatus ProcessStatus `json:"processStatus"`

	Deleted bool `json:"deleted"`
}

func RPCToProcess(obj *rpc.ProcessResponse) *Process {
	return &Process{
		Name:          obj.Spec.Name,
		Binary:        obj.Spec.Binary,
		Args:          obj.Spec.Args,
		PortCount:     obj.Spec.PortCount,
		PortArgs:      obj.Spec.PortArgs,
		ProcessStatus: RPCToProcessStatus(obj.Status),
	}
}

func RPCToProcessList(obj *rpc.ProcessListResponse) map[string]*Process {
	ret := map[string]*Process{}
	for name, p := range obj.Processes {
		ret[name] = RPCToProcess(p)
	}
	return ret
}

type ProcessStatus struct {
	State      string          `json:"state"`
	ErrorMsg   string          `json:"errorMsg"`
	Conditions map[string]bool `json:"conditions"`
	PortStart  int32           `json:"portStart"`
	PortEnd    int32           `json:"portEnd"`
	UUID       string          `json:"uuid"`
}

func RPCToProcessStatus(obj *rpc.ProcessStatus) ProcessStatus {
	return ProcessStatus{
		State:      obj.State,
		ErrorMsg:   obj.ErrorMsg,
		Conditions: obj.Conditions,
		PortStart:  obj.PortStart,
		PortEnd:    obj.PortEnd,
		UUID:       obj.Uuid,
	}
}

type ProcessStream struct {
	stream rpc.ProcessManagerService_ProcessWatchClient
}

func NewProcessStream(stream rpc.ProcessManagerService_ProcessWatchClient) *ProcessStream {
	return &ProcessStream{
		stream,
	}
}

func (s *ProcessStream) Recv() (*rpc.ProcessResponse, error) {
	return s.stream.Recv()
}
