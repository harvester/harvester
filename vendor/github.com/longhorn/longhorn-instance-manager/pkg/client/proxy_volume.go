package client

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"

	etypes "github.com/longhorn/longhorn-engine/pkg/types"
	eptypes "github.com/longhorn/longhorn-engine/proto/ptypes"

	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
)

func (c *ProxyClient) VolumeGet(backendStoreDriver, engineName, volumeName, serviceAddress string) (info *etypes.VolumeInfo, err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get volume")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return nil, fmt.Errorf("failed to get volume: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get volume", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address:            serviceAddress,
		EngineName:         engineName,
		BackendStoreDriver: rpc.BackendStoreDriver(driver),
		VolumeName:         volumeName,
	}
	resp, err := c.service.VolumeGet(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return nil, err
	}

	info = &etypes.VolumeInfo{
		Name:                      resp.Volume.Name,
		Size:                      resp.Volume.Size,
		ReplicaCount:              int(resp.Volume.ReplicaCount),
		Endpoint:                  resp.Volume.Endpoint,
		Frontend:                  resp.Volume.Frontend,
		FrontendState:             resp.Volume.FrontendState,
		IsExpanding:               resp.Volume.IsExpanding,
		LastExpansionError:        resp.Volume.LastExpansionError,
		LastExpansionFailedAt:     resp.Volume.LastExpansionFailedAt,
		UnmapMarkSnapChainRemoved: resp.Volume.UnmapMarkSnapChainRemoved,
	}
	return info, nil
}

func (c *ProxyClient) VolumeExpand(backendStoreDriver, engineName, volumeName, serviceAddress string,
	size int64) (err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to expand volume")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return fmt.Errorf("failed to expand volume: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to expand volume", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.EngineVolumeExpandRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:            serviceAddress,
			EngineName:         engineName,
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			VolumeName:         volumeName,
		},
		Expand: &eptypes.VolumeExpandRequest{
			Size: size,
		},
	}
	_, err = c.service.VolumeExpand(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) VolumeFrontendStart(backendStoreDriver, engineName, volumeName, serviceAddress, frontendName string) (err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
		"frontendName":   frontendName,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to start volume frontend")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return fmt.Errorf("failed to start volume frontend: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to start volume frontend %v", c.getProxyErrorPrefix(serviceAddress), frontendName)
	}()

	req := &rpc.EngineVolumeFrontendStartRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:            serviceAddress,
			EngineName:         engineName,
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			VolumeName:         volumeName,
		},
		FrontendStart: &eptypes.VolumeFrontendStartRequest{
			Frontend: frontendName,
		},
	}
	_, err = c.service.VolumeFrontendStart(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) VolumeFrontendShutdown(backendStoreDriver, engineName, volumeName,
	serviceAddress string) (err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to shutdown volume frontend")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return fmt.Errorf("failed to shutdown volume frontend: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to shutdown volume frontend", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address:            serviceAddress,
		EngineName:         engineName,
		BackendStoreDriver: rpc.BackendStoreDriver(driver),
		VolumeName:         volumeName,
	}
	_, err = c.service.VolumeFrontendShutdown(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) VolumeUnmapMarkSnapChainRemovedSet(backendStoreDriver, engineName, volumeName,
	serviceAddress string, enabled bool) (err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
		"enabled":        strconv.FormatBool(enabled),
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to set volume flag UnmapMarkSnapChainRemoved")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return fmt.Errorf("failed to set volume flag UnmapMarkSnapChainRemoved: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to set UnmapMarkSnapChainRemoved", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.EngineVolumeUnmapMarkSnapChainRemovedSetRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:            serviceAddress,
			EngineName:         engineName,
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			VolumeName:         volumeName,
		},
		UnmapMarkSnap: &eptypes.VolumeUnmapMarkSnapChainRemovedSetRequest{Enabled: enabled},
	}
	_, err = c.service.VolumeUnmapMarkSnapChainRemovedSet(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}
