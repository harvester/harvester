package client

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"

	etypes "github.com/longhorn/longhorn-engine/pkg/types"
	eptypes "github.com/longhorn/longhorn-engine/proto/ptypes"

	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
)

func (c *ProxyClient) VolumeGet(dataEngine, engineName, volumeName, serviceAddress string) (info *etypes.VolumeInfo, err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get volume")
	}

	driver, ok := rpc.DataEngine_value[getDataEngine(dataEngine)]
	if !ok {
		return nil, fmt.Errorf("failed to get volume: invalid data engine %v", dataEngine)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get volume", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address:    serviceAddress,
		EngineName: engineName,
		// nolint:all replaced with DataEngine
		BackendStoreDriver: rpc.BackendStoreDriver(driver),
		DataEngine:         rpc.DataEngine(driver),
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
		SnapshotMaxCount:          int(resp.Volume.SnapshotMaxCount),
		SnapshotMaxSize:           resp.Volume.SnapshotMaxSize,
	}
	return info, nil
}

func (c *ProxyClient) VolumeExpand(dataEngine, engineName, volumeName, serviceAddress string,
	size int64) (err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to expand volume")
	}

	driver, ok := rpc.DataEngine_value[getDataEngine(dataEngine)]
	if !ok {
		return fmt.Errorf("failed to expand volume: invalid data engine %v", dataEngine)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to expand volume", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.EngineVolumeExpandRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:    serviceAddress,
			EngineName: engineName,
			// nolint:all replaced with DataEngine
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			DataEngine:         rpc.DataEngine(driver),
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

func (c *ProxyClient) VolumeFrontendStart(dataEngine, engineName, volumeName, serviceAddress, frontendName string) (err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
		"frontendName":   frontendName,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to start volume frontend")
	}

	driver, ok := rpc.DataEngine_value[getDataEngine(dataEngine)]
	if !ok {
		return fmt.Errorf("failed to start volume frontend: invalid data engine %v", dataEngine)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to start volume frontend %v", c.getProxyErrorPrefix(serviceAddress), frontendName)
	}()

	req := &rpc.EngineVolumeFrontendStartRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:    serviceAddress,
			EngineName: engineName,
			// nolint:all replaced with DataEngine
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			DataEngine:         rpc.DataEngine(driver),
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

func (c *ProxyClient) VolumeFrontendShutdown(dataEngine, engineName, volumeName, serviceAddress string) (err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to shutdown volume frontend")
	}

	driver, ok := rpc.DataEngine_value[getDataEngine(dataEngine)]
	if !ok {
		return fmt.Errorf("failed to shutdown volume frontend: invalid data engine %v", dataEngine)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to shutdown volume frontend", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address:    serviceAddress,
		EngineName: engineName,
		// nolint:all replaced with DataEngine
		BackendStoreDriver: rpc.BackendStoreDriver(driver),
		DataEngine:         rpc.DataEngine(driver),
		VolumeName:         volumeName,
	}
	_, err = c.service.VolumeFrontendShutdown(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) VolumeUnmapMarkSnapChainRemovedSet(dataEngine, engineName, volumeName, serviceAddress string, enabled bool) (err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
		"enabled":        strconv.FormatBool(enabled),
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to set volume flag UnmapMarkSnapChainRemoved")
	}

	driver, ok := rpc.DataEngine_value[getDataEngine(dataEngine)]
	if !ok {
		return fmt.Errorf("failed to set volume flag UnmapMarkSnapChainRemoved: invalid data engine %v", dataEngine)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to set UnmapMarkSnapChainRemoved", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.EngineVolumeUnmapMarkSnapChainRemovedSetRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:    serviceAddress,
			EngineName: engineName,
			// nolint:all replaced with DataEngine
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			DataEngine:         rpc.DataEngine(driver),
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

func (c *ProxyClient) VolumeSnapshotMaxCountSet(dataEngine, engineName, volumeName,
	serviceAddress string, count int) (err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
		"count":          strconv.Itoa(count),
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to set volume flag SnapshotMaxCount")
	}

	driver, ok := rpc.DataEngine_value[getDataEngine(dataEngine)]
	if !ok {
		return fmt.Errorf("failed to set volume flag SnapshotMaxCount: invalid data engine %v", dataEngine)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to set SnapshotMaxCount", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.EngineVolumeSnapshotMaxCountSetRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:    serviceAddress,
			EngineName: engineName,
			// nolint:all replaced with DataEngine
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			DataEngine:         rpc.DataEngine(driver),
			VolumeName:         volumeName,
		},
		Count: &eptypes.VolumeSnapshotMaxCountSetRequest{Count: int32(count)},
	}
	_, err = c.service.VolumeSnapshotMaxCountSet(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) VolumeSnapshotMaxSizeSet(dataEngine, engineName, volumeName,
	serviceAddress string, size int64) (err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
		"size":           strconv.FormatInt(size, 10),
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to set volume flag SnapshotMaxSize")
	}

	driver, ok := rpc.DataEngine_value[getDataEngine(dataEngine)]
	if !ok {
		return fmt.Errorf("failed to set volume flag SnapshotMaxSize: invalid data engine %v", dataEngine)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to set SnapshotMaxSize", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.EngineVolumeSnapshotMaxSizeSetRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:    serviceAddress,
			EngineName: engineName,
			// nolint:all replaced with DataEngine
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			DataEngine:         rpc.DataEngine(driver),
			VolumeName:         volumeName,
		},
		Size: &eptypes.VolumeSnapshotMaxSizeSetRequest{Size: size},
	}
	_, err = c.service.VolumeSnapshotMaxSizeSet(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}
