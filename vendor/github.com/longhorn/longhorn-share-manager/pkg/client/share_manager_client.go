package client

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	rpc "github.com/longhorn/types/pkg/generated/smrpc"

	"github.com/longhorn/longhorn-share-manager/pkg/types"
)

type ShareManagerClient struct {
	address string
	conn    *grpc.ClientConn
	client  rpc.ShareManagerServiceClient
}

func NewShareManagerClient(address string) (*ShareManagerClient, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect share manager service to %v", address)
	}

	return &ShareManagerClient{
		address: address,
		conn:    conn,
		client:  rpc.NewShareManagerServiceClient(conn),
	}, nil
}

func (c *ShareManagerClient) Close() error {
	if c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func (c *ShareManagerClient) FilesystemTrim(encryptedDevice bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	_, err := c.client.FilesystemTrim(ctx, &rpc.FilesystemTrimRequest{EncryptedDevice: encryptedDevice})
	return err
}

func (c *ShareManagerClient) FilesystemResize() error {
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	_, err := c.client.FilesystemResize(ctx, &emptypb.Empty{})
	return err
}

func (c *ShareManagerClient) Unmount() error {
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	_, err := c.client.Unmount(ctx, &emptypb.Empty{})
	return err
}

func (c *ShareManagerClient) Mount() error {
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	_, err := c.client.Mount(ctx, &emptypb.Empty{})
	return err
}
