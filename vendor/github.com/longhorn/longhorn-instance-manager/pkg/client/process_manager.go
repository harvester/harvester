package client

import (
	"fmt"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/longhorn/longhorn-instance-manager/pkg/api"
	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
	"github.com/longhorn/longhorn-instance-manager/pkg/meta"
	"github.com/longhorn/longhorn-instance-manager/pkg/types"
)

type ProcessManagerClient struct {
	Address string
}

func NewProcessManagerClient(address string) *ProcessManagerClient {
	return &ProcessManagerClient{
		Address: address,
	}
}

func (cli *ProcessManagerClient) ProcessCreate(name, binary string, portCount int, args, portArgs []string) (*api.Process, error) {
	if name == "" || binary == "" {
		return nil, fmt.Errorf("failed to start process: missing required parameter")
	}

	conn, err := grpc.Dial(cli.Address, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("cannot connect process manager service to %v: %v", cli.Address, err)
	}
	defer conn.Close()

	client := rpc.NewProcessManagerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	p, err := client.ProcessCreate(ctx, &rpc.ProcessCreateRequest{
		Spec: &rpc.ProcessSpec{
			Name:      name,
			Binary:    binary,
			Args:      args,
			PortCount: int32(portCount),
			PortArgs:  portArgs,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start process: %v", err)
	}
	return api.RPCToProcess(p), nil
}

func (cli *ProcessManagerClient) ProcessDelete(name string) (*api.Process, error) {
	if name == "" {
		return nil, fmt.Errorf("failed to delete process: missing required parameter name")
	}

	conn, err := grpc.Dial(cli.Address, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("cannot connect process manager service to %v: %v", cli.Address, err)
	}
	defer conn.Close()

	client := rpc.NewProcessManagerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	p, err := client.ProcessDelete(ctx, &rpc.ProcessDeleteRequest{
		Name: name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to delete process %v: %v", name, err)
	}
	return api.RPCToProcess(p), nil
}

func (cli *ProcessManagerClient) ProcessGet(name string) (*api.Process, error) {
	if name == "" {
		return nil, fmt.Errorf("failed to get process: missing required parameter name")
	}

	conn, err := grpc.Dial(cli.Address, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("cannot connect process manager service to %v: %v", cli.Address, err)
	}
	defer conn.Close()

	client := rpc.NewProcessManagerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	p, err := client.ProcessGet(ctx, &rpc.ProcessGetRequest{
		Name: name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get process %v: %v", name, err)
	}
	return api.RPCToProcess(p), nil
}

func (cli *ProcessManagerClient) ProcessList() (map[string]*api.Process, error) {
	conn, err := grpc.Dial(cli.Address, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("cannot connect process manager service to %v: %v", cli.Address, err)
	}
	defer conn.Close()

	client := rpc.NewProcessManagerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	ps, err := client.ProcessList(ctx, &rpc.ProcessListRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list processes: %v", err)
	}
	return api.RPCToProcessList(ps), nil
}

func (cli *ProcessManagerClient) ProcessLog(name string) (*api.LogStream, error) {
	if name == "" {
		return nil, fmt.Errorf("failed to get process: missing required parameter name")
	}

	var err error
	conn, err := grpc.Dial(cli.Address, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("cannot connect process manager service to %v: %v", cli.Address, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer func() {
		if err != nil {
			cancel()
			conn.Close()
		}
	}()

	client := rpc.NewProcessManagerServiceClient(conn)
	stream, err := client.ProcessLog(ctx, &rpc.LogRequest{
		Name: name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get process log of %v: %v", name, err)
	}
	return api.NewLogStream(conn, cancel, stream), nil
}

func (cli *ProcessManagerClient) ProcessWatch() (*api.ProcessStream, error) {
	var err error
	conn, err := grpc.Dial(cli.Address, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("cannot connect process manager service to %v: %v", cli.Address, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			cancel()
			conn.Close()
		}
	}()

	// Don't cleanup the Client here, we don't know when the user will be done with the Stream. Pass it to the wrapper
	// and allow the user to take care of it.
	client := rpc.NewProcessManagerServiceClient(conn)
	stream, err := client.ProcessWatch(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open process update stream")
	}

	return api.NewProcessStream(conn, cancel, stream), nil
}

func (cli *ProcessManagerClient) ProcessReplace(name, binary string, portCount int, args, portArgs []string, terminateSignal string) (*api.Process, error) {
	if name == "" || binary == "" {
		return nil, fmt.Errorf("failed to start process: missing required parameter")
	}
	if terminateSignal != "SIGHUP" {
		return nil, fmt.Errorf("Unsupported terminate signal %v", terminateSignal)
	}

	conn, err := grpc.Dial(cli.Address, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("cannot connect process manager service to %v: %v", cli.Address, err)
	}
	defer conn.Close()

	client := rpc.NewProcessManagerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	p, err := client.ProcessReplace(ctx, &rpc.ProcessReplaceRequest{
		Spec: &rpc.ProcessSpec{
			Name:      name,
			Binary:    binary,
			Args:      args,
			PortCount: int32(portCount),
			PortArgs:  portArgs,
		},
		TerminateSignal: terminateSignal,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start process: %v", err)
	}
	return api.RPCToProcess(p), nil
}

func (cli *ProcessManagerClient) VersionGet() (*meta.VersionOutput, error) {
	conn, err := grpc.Dial(cli.Address, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("cannot connect process manager service to %v: %v", cli.Address, err)
	}
	defer conn.Close()

	client := rpc.NewProcessManagerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	resp, err := client.VersionGet(ctx, &empty.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %v", err)
	}
	return &meta.VersionOutput{
		Version:   resp.Version,
		GitCommit: resp.GitCommit,
		BuildDate: resp.BuildDate,

		InstanceManagerAPIVersion:    int(resp.InstanceManagerAPIVersion),
		InstanceManagerAPIMinVersion: int(resp.InstanceManagerAPIMinVersion),
	}, nil
}
