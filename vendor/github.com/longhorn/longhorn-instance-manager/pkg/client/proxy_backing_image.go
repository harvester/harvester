package client

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/longhorn/longhorn-instance-manager/pkg/api"
	"github.com/longhorn/longhorn-instance-manager/pkg/types"

	rpc "github.com/longhorn/types/pkg/generated/imrpc"
)

func (c *ProxyClient) SPDKBackingImageCreate(name, backingImageUUID, diskUUID, checksum, fromAddress, srcLvsUUID string, size uint64) (*api.BackingImage, error) {
	input := map[string]string{
		"name":             name,
		"backingImageUUID": backingImageUUID,
		"checksum":         checksum,
		"diskUUID":         diskUUID,
	}

	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to create backing image")
	}

	if size == 0 {
		return nil, fmt.Errorf("failed to create backing image, size should not be zero")
	}

	client := c.service
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	resp, err := client.SPDKBackingImageCreate(ctx, &rpc.SPDKBackingImageCreateRequest{
		Name:             name,
		BackingImageUuid: backingImageUUID,
		DiskUuid:         diskUUID,
		Size:             size,
		Checksum:         checksum,
		FromAddress:      fromAddress,
		SrcLvsUuid:       srcLvsUUID,
	})
	if err != nil {
		return nil, err
	}

	return api.RPCToBackingImage(resp)
}

func (c *ProxyClient) SPDKBackingImageDelete(name, diskUUID string) error {
	input := map[string]string{
		"name":     name,
		"diskUUID": diskUUID,
	}

	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to delete backing image")
	}

	client := c.service
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	_, err := client.SPDKBackingImageDelete(ctx, &rpc.SPDKBackingImageDeleteRequest{
		Name:     name,
		DiskUuid: diskUUID,
	})
	return err
}

func (c *ProxyClient) SPDKBackingImageGet(name, diskUUID string) (*api.BackingImage, error) {
	input := map[string]string{
		"name":     name,
		"diskUUID": diskUUID,
	}

	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get backing image")
	}

	client := c.service
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	resp, err := client.SPDKBackingImageGet(ctx, &rpc.SPDKBackingImageGetRequest{
		Name:     name,
		DiskUuid: diskUUID,
	})
	if err != nil {
		return nil, err
	}
	return api.RPCToBackingImage(resp)
}

func (c *ProxyClient) SPDKBackingImageList() (map[string]*api.BackingImage, error) {
	client := c.service
	ctx, cancel := context.WithTimeout(context.Background(), types.GRPCServiceTimeout)
	defer cancel()

	resp, err := client.SPDKBackingImageList(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list backing images")
	}
	return api.RPCToBackingImageList(resp), nil
}

func (c *ProxyClient) SPDKBackingImageWatch(ctx context.Context) (*api.BackingImageStream, error) {
	client := c.service
	stream, err := client.SPDKBackingImageWatch(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to open backing image update stream")
	}

	return api.NewBackingImageStream(stream), nil
}
