package client

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/longhorn/longhorn-instance-manager/pkg/api"
	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
	"github.com/longhorn/longhorn-instance-manager/pkg/meta"
	"github.com/longhorn/longhorn-instance-manager/pkg/types"
	"github.com/longhorn/longhorn-instance-manager/pkg/util"
)

type InstanceServiceContext struct {
	cc *grpc.ClientConn

	ctx  context.Context
	quit context.CancelFunc

	service rpc.InstanceServiceClient
	health  healthpb.HealthClient
}

func (c InstanceServiceContext) Close() error {
	c.quit()
	if c.cc == nil {
		return nil
	}
	if err := c.cc.Close(); err != nil {
		return errors.Wrap(err, "failed to close instance gRPC connection")
	}
	return nil
}

func (c *InstanceServiceClient) getControllerServiceClient() rpc.InstanceServiceClient {
	return c.service
}

type InstanceServiceClient struct {
	serviceURL string
	tlsConfig  *tls.Config
	InstanceServiceContext
}

func NewInstanceServiceClient(ctx context.Context, ctxCancel context.CancelFunc, serviceURL string, tlsConfig *tls.Config) (*InstanceServiceClient, error) {
	getInstanceServiceContext := func(serviceUrl string, tlsConfig *tls.Config) (InstanceServiceContext, error) {
		connection, err := util.Connect(serviceUrl, tlsConfig)
		if err != nil {
			return InstanceServiceContext{}, errors.Wrapf(err, "cannot connect to Instance Service %v", serviceUrl)
		}

		return InstanceServiceContext{
			cc:      connection,
			ctx:     ctx,
			quit:    ctxCancel,
			service: rpc.NewInstanceServiceClient(connection),
			health:  healthpb.NewHealthClient(connection),
		}, nil
	}

	serviceContext, err := getInstanceServiceContext(serviceURL, tlsConfig)
	if err != nil {
		return nil, err
	}

	return &InstanceServiceClient{
		serviceURL:             serviceURL,
		tlsConfig:              tlsConfig,
		InstanceServiceContext: serviceContext,
	}, nil
}

func NewInstanceServiceClientWithTLS(ctx context.Context, ctxCancel context.CancelFunc, serviceURL, caFile, certFile, keyFile, peerName string) (*InstanceServiceClient, error) {
	tlsConfig, err := util.LoadClientTLS(caFile, certFile, keyFile, peerName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load tls key pair from file")
	}

	return NewInstanceServiceClient(ctx, ctxCancel, serviceURL, tlsConfig)
}

type EngineCreateRequest struct {
	ReplicaAddressMap map[string]string
	Frontend          string
}

type ReplicaCreateRequest struct {
	DiskName       string
	DiskUUID       string
	ExposeRequired bool
}

type InstanceCreateRequest struct {
	BackendStoreDriver string
	Name               string
	InstanceType       string
	VolumeName         string
	Size               uint64
	PortCount          int
	PortArgs           []string

	Binary     string
	BinaryArgs []string

	Engine  EngineCreateRequest
	Replica ReplicaCreateRequest
}

func (c *InstanceServiceClient) InstanceCreate(req *InstanceCreateRequest) (*api.Instance, error) {
	if req.Name == "" || req.InstanceType == "" {
		return nil, fmt.Errorf("failed to create instance: missing required parameter")
	}

	driver, ok := rpc.BackendStoreDriver_value[req.BackendStoreDriver]
	if !ok {
		return nil, fmt.Errorf("failed to delete instance: invalid backend store driver %v", req.BackendStoreDriver)
	}

	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	var processInstanceSpec *rpc.ProcessInstanceSpec
	var spdkInstanceSpec *rpc.SpdkInstanceSpec
	if rpc.BackendStoreDriver(driver) == rpc.BackendStoreDriver_v1 {
		processInstanceSpec = &rpc.ProcessInstanceSpec{
			Binary: req.Binary,
			Args:   req.BinaryArgs,
		}
	} else {
		switch req.InstanceType {
		case types.InstanceTypeEngine:
			spdkInstanceSpec = &rpc.SpdkInstanceSpec{
				Size:              req.Size,
				ReplicaAddressMap: req.Engine.ReplicaAddressMap,
				Frontend:          req.Engine.Frontend,
			}
		case types.InstanceTypeReplica:
			spdkInstanceSpec = &rpc.SpdkInstanceSpec{
				Size:           req.Size,
				DiskName:       req.Replica.DiskName,
				DiskUuid:       req.Replica.DiskUUID,
				ExposeRequired: req.Replica.ExposeRequired,
			}
		default:
			return nil, fmt.Errorf("failed to create instance: invalid instance type %v", req.InstanceType)
		}
	}

	p, err := client.InstanceCreate(ctx, &rpc.InstanceCreateRequest{
		Spec: &rpc.InstanceSpec{
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			Name:               req.Name,
			Type:               req.InstanceType,
			VolumeName:         req.VolumeName,
			PortCount:          int32(req.PortCount),
			PortArgs:           req.PortArgs,

			ProcessInstanceSpec: processInstanceSpec,
			SpdkInstanceSpec:    spdkInstanceSpec,
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create instance")
	}

	return api.RPCToInstance(p), nil
}

func (c *InstanceServiceClient) InstanceDelete(backendStoreDriver, name, instanceType, diskUUID string, cleanupRequired bool) (*api.Instance, error) {
	if name == "" {
		return nil, fmt.Errorf("failed to delete instance: missing required parameter name")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return nil, fmt.Errorf("failed to delete instance: invalid backend store driver %v", backendStoreDriver)
	}

	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	p, err := client.InstanceDelete(ctx, &rpc.InstanceDeleteRequest{
		Name:               name,
		Type:               instanceType,
		BackendStoreDriver: rpc.BackendStoreDriver(driver),
		DiskUuid:           diskUUID,
		CleanupRequired:    cleanupRequired,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to delete instance %v", name)
	}
	return api.RPCToInstance(p), nil
}

func (c *InstanceServiceClient) InstanceGet(backendStoreDriver, name, instanceType string) (*api.Instance, error) {
	if name == "" {
		return nil, fmt.Errorf("failed to get instance: missing required parameter name")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return nil, fmt.Errorf("failed to get instance: invalid backend store driver %v", backendStoreDriver)
	}

	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	p, err := client.InstanceGet(ctx, &rpc.InstanceGetRequest{
		Name:               name,
		Type:               instanceType,
		BackendStoreDriver: rpc.BackendStoreDriver(driver),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get instance %v", name)
	}
	return api.RPCToInstance(p), nil
}

func (c *InstanceServiceClient) InstanceList() (map[string]*api.Instance, error) {
	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	instances, err := client.InstanceList(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list instances")
	}
	return api.RPCToInstanceList(instances), nil
}

func (c *InstanceServiceClient) InstanceLog(ctx context.Context, backendStoreDriver, name, instanceType string) (*api.LogStream, error) {
	if name == "" {
		return nil, fmt.Errorf("failed to get instance: missing required parameter name")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return nil, fmt.Errorf("failed to log instance: invalid backend store driver %v", backendStoreDriver)
	}

	client := c.getControllerServiceClient()
	stream, err := client.InstanceLog(ctx, &rpc.InstanceLogRequest{
		Name:               name,
		Type:               instanceType,
		BackendStoreDriver: rpc.BackendStoreDriver(driver),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get instance log of %v", name)
	}
	return api.NewLogStream(stream), nil
}

func (c *InstanceServiceClient) InstanceWatch(ctx context.Context) (*api.InstanceStream, error) {
	client := c.getControllerServiceClient()
	stream, err := client.InstanceWatch(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to open instance update stream")
	}

	return api.NewInstanceStream(stream), nil
}

func (c *InstanceServiceClient) InstanceReplace(backendStoreDriver, name, instanceType, binary string, portCount int, args, portArgs []string, terminateSignal string) (*api.Instance, error) {
	if name == "" || binary == "" {
		return nil, fmt.Errorf("failed to replace instance: missing required parameter")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return nil, fmt.Errorf("failed to replace instance: invalid backend store driver %v", backendStoreDriver)
	}

	if terminateSignal != "SIGHUP" {
		return nil, fmt.Errorf("unsupported terminate signal %v", terminateSignal)
	}

	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	p, err := client.InstanceReplace(ctx, &rpc.InstanceReplaceRequest{
		Spec: &rpc.InstanceSpec{
			Name:               name,
			Type:               instanceType,
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			ProcessInstanceSpec: &rpc.ProcessInstanceSpec{
				Binary: binary,
				Args:   args,
			},
			PortCount: int32(portCount),
			PortArgs:  portArgs,
		},
		TerminateSignal: terminateSignal,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to replace instance")
	}
	return api.RPCToInstance(p), nil
}

func (c *InstanceServiceClient) VersionGet() (*meta.VersionOutput, error) {
	client := c.getControllerServiceClient()
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	resp, err := client.VersionGet(ctx, &empty.Empty{})
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

func (c *InstanceServiceClient) CheckConnection() error {
	req := &healthpb.HealthCheckRequest{}
	_, err := c.health.Check(getContextWithGRPCTimeout(c.ctx), req)
	return err
}
