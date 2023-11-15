package client

import (
	"fmt"

	"github.com/pkg/errors"

	etypes "github.com/longhorn/longhorn-engine/pkg/types"
	eptypes "github.com/longhorn/longhorn-engine/proto/ptypes"

	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
)

func (c *ProxyClient) ReplicaAdd(backendStoreDriver, engineName, serviceAddress, replicaName, replicaAddress string, restore bool, size, currentSize int64, fileSyncHTTPClientTimeout int, fastSync bool) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
		"engineName":     engineName,
		"replicaName":    replicaName,
		"replicaAddress": replicaAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to add replica for volume")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return fmt.Errorf("failed to add replica for volume: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		if restore {
			err = errors.Wrapf(err, "%v failed to add restore replica %v for volume", c.getProxyErrorPrefix(serviceAddress), replicaAddress)
		} else {
			err = errors.Wrapf(err, "%v failed to add replica %v for volume", c.getProxyErrorPrefix(serviceAddress), replicaAddress)
		}
	}()

	req := &rpc.EngineReplicaAddRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:            serviceAddress,
			EngineName:         engineName,
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
		},
		ReplicaName:               replicaName,
		ReplicaAddress:            replicaAddress,
		Restore:                   restore,
		Size:                      size,
		CurrentSize:               currentSize,
		FastSync:                  fastSync,
		FileSyncHttpClientTimeout: int32(fileSyncHTTPClientTimeout),
	}
	_, err = c.service.ReplicaAdd(getContextWithGRPCLongTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) ReplicaList(backendStoreDriver, engineName, serviceAddress string) (rInfoList []*etypes.ControllerReplicaInfo, err error) {
	input := map[string]string{
		"engineName":     engineName,
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to list replicas for volume")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return nil, fmt.Errorf("failed to list replicas for volume: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to list replicas for volume", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address:            serviceAddress,
		EngineName:         engineName,
		BackendStoreDriver: rpc.BackendStoreDriver(driver),
	}
	resp, err := c.service.ReplicaList(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return nil, err
	}

	for _, cr := range resp.ReplicaList.Replicas {
		rInfoList = append(rInfoList, &etypes.ControllerReplicaInfo{
			Address: cr.Address.Address,
			Mode:    eptypes.GRPCReplicaModeToReplicaMode(cr.Mode),
		})
	}

	return rInfoList, nil
}

func (c *ProxyClient) ReplicaRebuildingStatus(backendStoreDriver, engineName, serviceAddress string) (status map[string]*ReplicaRebuildStatus, err error) {
	input := map[string]string{
		"engineName":     engineName,
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get replicas rebuilding status")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return nil, fmt.Errorf("failed to get replicas rebuilding status: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get replicas rebuilding status", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address:            serviceAddress,
		EngineName:         engineName,
		BackendStoreDriver: rpc.BackendStoreDriver(driver),
	}
	recv, err := c.service.ReplicaRebuildingStatus(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return status, err
	}

	status = make(map[string]*ReplicaRebuildStatus)
	for k, v := range recv.Status {
		status[k] = &ReplicaRebuildStatus{
			Error:              v.Error,
			IsRebuilding:       v.IsRebuilding,
			Progress:           int(v.Progress),
			State:              v.State,
			FromReplicaAddress: v.FromReplicaAddress,
		}
	}
	return status, nil
}

func (c *ProxyClient) ReplicaVerifyRebuild(backendStoreDriver, serviceAddress, replicaAddress string) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
		"replicaAddress": replicaAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to verify replica rebuild")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return fmt.Errorf("failed to verify replica rebuild: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to verify replica %v rebuild", c.getProxyErrorPrefix(serviceAddress), replicaAddress)
	}()

	req := &rpc.EngineReplicaVerifyRebuildRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:            serviceAddress,
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
		},
		ReplicaAddress: replicaAddress,
	}
	_, err = c.service.ReplicaVerifyRebuild(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) ReplicaRemove(backendStoreDriver, serviceAddress, engineName, replicaAddress, replicaName string) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
		"engineName":     engineName,
		"replicaAddress": replicaAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to remove replica for volume")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return fmt.Errorf("failed to remove replica for volume: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to remove replica %v for volume", c.getProxyErrorPrefix(serviceAddress), replicaAddress)
	}()

	req := &rpc.EngineReplicaRemoveRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:            serviceAddress,
			EngineName:         engineName,
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
		},
		ReplicaAddress: replicaAddress,
		ReplicaName:    replicaName,
	}
	_, err = c.service.ReplicaRemove(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) ReplicaModeUpdate(backendStoreDriver, serviceAddress, replicaAddress string, mode string) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
		"replicaAddress": replicaAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to remove replica for volume")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return fmt.Errorf("failed to remove replica for volume: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to update replica %v mode for volume", c.getProxyErrorPrefix(serviceAddress), replicaAddress)
	}()

	req := &rpc.EngineReplicaModeUpdateRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:            serviceAddress,
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
		},
		ReplicaAddress: replicaAddress,
		Mode:           eptypes.ReplicaModeToGRPCReplicaMode(etypes.Mode(mode)),
	}
	_, err = c.service.ReplicaModeUpdate(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}
