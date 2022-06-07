package client

import (
	"github.com/pkg/errors"

	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"

	etypes "github.com/longhorn/longhorn-engine/pkg/types"
	eptypes "github.com/longhorn/longhorn-engine/proto/ptypes"
)

func (c *ProxyClient) VolumeGet(serviceAddress string) (info *etypes.VolumeInfo, err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get volume")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get volume", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address: serviceAddress,
	}
	resp, err := c.service.VolumeGet(c.ctx, req)
	if err != nil {
		return nil, err
	}

	info = &etypes.VolumeInfo{
		Name:                  resp.Volume.Name,
		Size:                  resp.Volume.Size,
		ReplicaCount:          int(resp.Volume.ReplicaCount),
		Endpoint:              resp.Volume.Endpoint,
		Frontend:              resp.Volume.Frontend,
		FrontendState:         resp.Volume.FrontendState,
		IsExpanding:           resp.Volume.IsExpanding,
		LastExpansionError:    resp.Volume.LastExpansionError,
		LastExpansionFailedAt: resp.Volume.LastExpansionFailedAt,
	}
	return info, nil
}

func (c *ProxyClient) VolumeExpand(serviceAddress string, size int64) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to expand volume")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to expand volume", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.EngineVolumeExpandRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address: serviceAddress,
		},
		Expand: &eptypes.VolumeExpandRequest{
			Size: size,
		},
	}
	_, err = c.service.VolumeExpand(c.ctx, req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) VolumeFrontendStart(serviceAddress, frontendName string) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
		"frontendName":   frontendName,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to start volume frontend")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to start volume frontend %v", c.getProxyErrorPrefix(serviceAddress), frontendName)
	}()

	req := &rpc.EngineVolumeFrontendStartRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address: serviceAddress,
		},
		FrontendStart: &eptypes.VolumeFrontendStartRequest{
			Frontend: frontendName,
		},
	}
	_, err = c.service.VolumeFrontendStart(c.ctx, req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) VolumeFrontendShutdown(serviceAddress string) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to shutdown volume frontend")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to shutdown volume frontend", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address: serviceAddress,
	}
	_, err = c.service.VolumeFrontendShutdown(c.ctx, req)
	if err != nil {
		return err
	}

	return nil
}
