package client

import (
	"crypto/tls"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
	"github.com/longhorn/longhorn-instance-manager/pkg/meta"
	"github.com/longhorn/longhorn-instance-manager/pkg/util"

	emeta "github.com/longhorn/longhorn-engine/pkg/meta"
	eclient "github.com/longhorn/longhorn-engine/pkg/replica/client"
)

var (
	ErrParameterFmt = "missing required %v parameter"
)

func validateProxyMethodParameters(input map[string]string) error {
	for k, v := range input {
		if v == "" {
			return errors.Errorf(ErrParameterFmt, k)
		}
	}
	return nil
}

type ServiceContext struct {
	cc *grpc.ClientConn

	ctx  context.Context
	quit context.CancelFunc

	service rpc.ProxyEngineServiceClient
	health  healthpb.HealthClient
}

func (s ServiceContext) GetConnectionState() connectivity.State {
	return s.cc.GetState()
}

func (c *ProxyClient) Close() error {
	c.quit()
	if err := c.cc.Close(); err != nil {
		return errors.Wrap(err, "failed to close proxy gRPC connection")
	}
	return nil
}

type ProxyClient struct {
	ServiceURL string
	ServiceContext

	Version int
}

func NewProxyClient(ctx context.Context, ctxCancel context.CancelFunc, address string, port int, tlsConfig *tls.Config) (*ProxyClient, error) {
	getServiceCtx := func(serviceUrl string) (ServiceContext, error) {
		connection, err := util.Connect(serviceUrl, tlsConfig)
		if err != nil {
			return ServiceContext{}, errors.Wrapf(err, "cannot connect to ProxyService %v", serviceUrl)
		}
		return ServiceContext{
			cc:      connection,
			ctx:     ctx,
			quit:    ctxCancel,
			service: rpc.NewProxyEngineServiceClient(connection),
			health:  healthpb.NewHealthClient(connection),
		}, nil
	}

	serviceURL := util.GetURL(address, port)
	serviceCtx, err := getServiceCtx(serviceURL)
	if err != nil {
		return nil, err
	}
	logrus.Tracef("Connected to proxy service on %v", serviceURL)

	return &ProxyClient{
		ServiceURL:     serviceURL,
		ServiceContext: serviceCtx,
		Version:        meta.InstanceManagerProxyAPIVersion,
	}, nil
}

func NewProxyClientWithTLS(ctx context.Context, ctxCancel context.CancelFunc, address string, port int, caFile, certFile, keyFile, peerName string) (*ProxyClient, error) {
	tlsConfig, err := util.LoadClientTLS(caFile, certFile, keyFile, peerName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load tls key pair from file")
	}

	return NewProxyClient(ctx, ctxCancel, address, port, tlsConfig)
}

const (
	GRPCServiceTimeout     = eclient.GRPCServiceCommonTimeout * 2
	GRPCServiceLongTimeout = eclient.GRPCServiceLongTimeout + GRPCServiceTimeout
)

func getContextWithGRPCTimeout(parent context.Context) context.Context {
	ctx, _ := context.WithTimeout(parent, GRPCServiceTimeout)
	return ctx
}

func getContextWithGRPCLongTimeout(parent context.Context) context.Context {
	ctx, _ := context.WithTimeout(parent, GRPCServiceLongTimeout)
	return ctx
}

func (c *ProxyClient) getProxyErrorPrefix(destination string) string {
	return fmt.Sprintf("proxyServer=%v destination=%v:", c.ServiceURL, destination)
}

func (c *ProxyClient) ServerVersionGet(serviceAddress string) (version *emeta.VersionOutput, err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get server version")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get server version", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address: serviceAddress,
	}
	resp, err := c.service.ServerVersionGet(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return nil, err
	}

	serverVersion := resp.Version
	version = &emeta.VersionOutput{
		Version:                 serverVersion.Version,
		GitCommit:               serverVersion.GitCommit,
		BuildDate:               serverVersion.BuildDate,
		CLIAPIVersion:           int(serverVersion.CliAPIVersion),
		CLIAPIMinVersion:        int(serverVersion.CliAPIMinVersion),
		ControllerAPIVersion:    int(serverVersion.ControllerAPIVersion),
		ControllerAPIMinVersion: int(serverVersion.ControllerAPIMinVersion),
		DataFormatVersion:       int(serverVersion.DataFormatVersion),
		DataFormatMinVersion:    int(serverVersion.DataFormatMinVersion),
	}
	return version, nil
}

func (c *ProxyClient) ClientVersionGet() (version emeta.VersionOutput) {
	logrus.Trace("Getting client version")
	return emeta.GetVersion()
}

func (c *ProxyClient) CheckConnection() error {
	req := &healthpb.HealthCheckRequest{}
	_, err := c.health.Check(getContextWithGRPCTimeout(c.ctx), req)
	return err
}
