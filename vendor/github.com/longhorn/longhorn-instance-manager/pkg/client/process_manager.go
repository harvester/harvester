package client

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/longhorn/longhorn-instance-manager/pkg/api"
	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
	"github.com/longhorn/longhorn-instance-manager/pkg/meta"
	"github.com/longhorn/longhorn-instance-manager/pkg/types"
	"github.com/longhorn/longhorn-instance-manager/pkg/util"
)

type ProcessManagerServiceContext struct {
	cc *grpc.ClientConn

	ctx  context.Context
	quit context.CancelFunc

	service rpc.ProcessManagerServiceClient
	health  healthpb.HealthClient
}

func (c ProcessManagerServiceContext) Close() error {
	c.quit()
	if c.cc == nil {
		return nil
	}
	if err := c.cc.Close(); err != nil {
		return errors.Wrap(err, "failed to close process manager gRPC connection")
	}
	return nil
}

func (c *ProcessManagerClient) getControllerServiceClient() rpc.ProcessManagerServiceClient {
	return c.service
}

type ProcessManagerClient struct {
	serviceURL string
	tlsConfig  *tls.Config
	ProcessManagerServiceContext
}

func NewProcessManagerClient(ctx context.Context, ctxCancel context.CancelFunc, serviceURL string, tlsConfig *tls.Config) (*ProcessManagerClient, error) {
	getProcessManagerServiceContext := func(serviceUrl string, tlsConfig *tls.Config) (ProcessManagerServiceContext, error) {
		connection, err := util.Connect(serviceUrl, tlsConfig)
		if err != nil {
			return ProcessManagerServiceContext{}, errors.Wrapf(err, "cannot connect to ProcessManagerService %v", serviceUrl)
		}

		return ProcessManagerServiceContext{
			cc:      connection,
			ctx:     ctx,
			quit:    ctxCancel,
			service: rpc.NewProcessManagerServiceClient(connection),
			health:  healthpb.NewHealthClient(connection),
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

func NewProcessManagerClientWithTLS(ctx context.Context, ctxCancel context.CancelFunc, serviceURL, caFile, certFile, keyFile, peerName string) (*ProcessManagerClient, error) {
	tlsConfig, err := util.LoadClientTLS(caFile, certFile, keyFile, peerName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load tls key pair from file")
	}

	return NewProcessManagerClient(ctx, ctxCancel, serviceURL, tlsConfig)
}

func (c *ProcessManagerClient) ProcessCreate(name, binary string, portCount int, args, portArgs []string) (*rpc.ProcessResponse, error) {
	logrus.WithFields(logrus.Fields{
		"name":      name,
		"binary":    binary,
		"args":      args,
		"portCount": portCount,
		"portArgs":  portArgs,
	}).Info("Creating process")

	if name == "" || binary == "" {
		return nil, fmt.Errorf("failed to start process: missing required parameter")
	}

	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	return client.ProcessCreate(ctx, &rpc.ProcessCreateRequest{
		Spec: &rpc.ProcessSpec{
			Name:      name,
			Binary:    binary,
			Args:      args,
			PortCount: int32(portCount),
			PortArgs:  portArgs,
		},
	})
}

func (c *ProcessManagerClient) ProcessDelete(name string) (*rpc.ProcessResponse, error) {
	if name == "" {
		return nil, fmt.Errorf("failed to delete process: missing required parameter name")
	}

	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	return client.ProcessDelete(ctx, &rpc.ProcessDeleteRequest{
		Name: name,
	})
}

func (c *ProcessManagerClient) ProcessGet(name string) (*rpc.ProcessResponse, error) {
	if name == "" {
		return nil, fmt.Errorf("failed to get process: missing required parameter name")
	}

	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	return client.ProcessGet(ctx, &rpc.ProcessGetRequest{
		Name: name,
	})
}

func (c *ProcessManagerClient) ProcessList() (map[string]*rpc.ProcessResponse, error) {
	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	resp, err := client.ProcessList(ctx, &rpc.ProcessListRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list processes")
	}
	return resp.Processes, nil
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
		return nil, errors.Wrapf(err, "failed to get process log of %v", name)
	}
	return api.NewLogStream(stream), nil
}

func (c *ProcessManagerClient) ProcessWatch(ctx context.Context) (*api.ProcessStream, error) {
	client := c.getControllerServiceClient()
	stream, err := client.ProcessWatch(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to open process update stream")
	}

	return api.NewProcessStream(stream), nil
}

func (c *ProcessManagerClient) ProcessReplace(name, binary string, portCount int, args, portArgs []string, terminateSignal string) (*rpc.ProcessResponse, error) {
	if name == "" || binary == "" {
		return nil, fmt.Errorf("failed to start process: missing required parameter")
	}
	if terminateSignal != "SIGHUP" {
		return nil, fmt.Errorf("unsupported terminate signal %v", terminateSignal)
	}

	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	return client.ProcessReplace(ctx, &rpc.ProcessReplaceRequest{
		Spec: &rpc.ProcessSpec{
			Name:      name,
			Binary:    binary,
			Args:      args,
			PortCount: int32(portCount),
			PortArgs:  portArgs,
		},
		TerminateSignal: terminateSignal,
	})
}

func (c *ProcessManagerClient) VersionGet() (*meta.VersionOutput, error) {

	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	resp, err := client.VersionGet(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get version")
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

func (c *ProcessManagerClient) CheckConnection() error {
	req := &healthpb.HealthCheckRequest{}
	_, err := c.health.Check(getContextWithGRPCTimeout(c.ctx), req)
	return err
}
