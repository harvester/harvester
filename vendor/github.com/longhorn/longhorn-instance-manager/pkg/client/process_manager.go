package client

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/longhorn/longhorn-instance-manager/pkg/api"
	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
	"github.com/longhorn/longhorn-instance-manager/pkg/meta"
	"github.com/longhorn/longhorn-instance-manager/pkg/types"
	"github.com/longhorn/longhorn-instance-manager/pkg/util"
)

type ProcessManagerServiceContext struct {
	cc      *grpc.ClientConn
	service rpc.ProcessManagerServiceClient
}

func (c ProcessManagerServiceContext) Close() error {
	if c.cc == nil {
		return nil
	}
	return c.cc.Close()
}

func (c *ProcessManagerClient) getControllerServiceClient() rpc.ProcessManagerServiceClient {
	return c.service
}

type ProcessManagerClient struct {
	serviceURL string
	tlsConfig  *tls.Config
	ProcessManagerServiceContext
}

func NewProcessManagerClient(serviceURL string, tlsConfig *tls.Config) (*ProcessManagerClient, error) {
	getProcessManagerServiceContext := func(serviceUrl string, tlsConfig *tls.Config) (ProcessManagerServiceContext, error) {
		connection, err := util.Connect(serviceUrl, tlsConfig)
		if err != nil {
			return ProcessManagerServiceContext{}, fmt.Errorf("cannot connect to ProcessManagerService %v: %v", serviceUrl, err)
		}

		return ProcessManagerServiceContext{
			cc:      connection,
			service: rpc.NewProcessManagerServiceClient(connection),
		}, nil
	}

	serviceContext, err := getProcessManagerServiceContext(serviceURL, tlsConfig)
	if err != nil {
		return nil, err
	}

	return &ProcessManagerClient{
		serviceURL:                   serviceURL,
		tlsConfig:                    tlsConfig,
		ProcessManagerServiceContext: serviceContext,
	}, nil
}

func NewProcessManagerClientWithTLS(serviceURL, caFile, certFile, keyFile, peerName string) (*ProcessManagerClient, error) {
	tlsConfig, err := util.LoadClientTLS(caFile, certFile, keyFile, peerName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load tls key pair from file")
	}

	return NewProcessManagerClient(serviceURL, tlsConfig)
}

func (c *ProcessManagerClient) ProcessCreate(name, binary string, portCount int, args, portArgs []string) (*api.Process, error) {
	if name == "" || binary == "" {
		return nil, fmt.Errorf("failed to start process: missing required parameter")
	}

	client := c.getControllerServiceClient()
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

func (c *ProcessManagerClient) ProcessDelete(name string) (*api.Process, error) {
	if name == "" {
		return nil, fmt.Errorf("failed to delete process: missing required parameter name")
	}

	client := c.getControllerServiceClient()
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

func (c *ProcessManagerClient) ProcessGet(name string) (*api.Process, error) {
	if name == "" {
		return nil, fmt.Errorf("failed to get process: missing required parameter name")
	}

	client := c.getControllerServiceClient()
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

func (c *ProcessManagerClient) ProcessList() (map[string]*api.Process, error) {
	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	ps, err := client.ProcessList(ctx, &rpc.ProcessListRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list processes: %v", err)
	}
	return api.RPCToProcessList(ps), nil
}

func (c *ProcessManagerClient) ProcessLog(ctx context.Context, name string) (*api.LogStream, error) {
	if name == "" {
		return nil, fmt.Errorf("failed to get process: missing required parameter name")
	}

	client := c.getControllerServiceClient()
	stream, err := client.ProcessLog(ctx, &rpc.LogRequest{
		Name: name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get process log of %v: %v", name, err)
	}
	return api.NewLogStream(stream), nil
}

func (c *ProcessManagerClient) ProcessWatch(ctx context.Context) (*api.ProcessStream, error) {
	client := c.getControllerServiceClient()
	stream, err := client.ProcessWatch(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open process update stream")
	}

	return api.NewProcessStream(stream), nil
}

func (c *ProcessManagerClient) ProcessReplace(name, binary string, portCount int, args, portArgs []string, terminateSignal string) (*api.Process, error) {
	if name == "" || binary == "" {
		return nil, fmt.Errorf("failed to start process: missing required parameter")
	}
	if terminateSignal != "SIGHUP" {
		return nil, fmt.Errorf("Unsupported terminate signal %v", terminateSignal)
	}

	client := c.getControllerServiceClient()
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

func (c *ProcessManagerClient) VersionGet() (*meta.VersionOutput, error) {

	client := c.getControllerServiceClient()
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

		InstanceManagerProxyAPIVersion:    int(resp.InstanceManagerProxyAPIVersion),
		InstanceManagerProxyAPIMinVersion: int(resp.InstanceManagerProxyAPIMinVersion),
	}, nil
}
