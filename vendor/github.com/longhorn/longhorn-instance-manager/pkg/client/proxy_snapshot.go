package client

import (
	"github.com/pkg/errors"

	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"

	etypes "github.com/longhorn/longhorn-engine/pkg/types"
	eutil "github.com/longhorn/longhorn-engine/pkg/util"
	eptypes "github.com/longhorn/longhorn-engine/proto/ptypes"
)

const (
	VolumeHeadName = "volume-head"
)

func (c *ProxyClient) VolumeSnapshot(serviceAddress, volumeSnapshotName string, labels map[string]string) (snapshotName string, err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return "", errors.Wrap(err, "failed to snapshot volume")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to snapshot volume", c.getProxyErrorPrefix(serviceAddress))
	}()

	for key, value := range labels {
		if errList := eutil.IsQualifiedName(key); len(errList) > 0 {
			err = errors.Errorf("invalid key %v for label: %v", key, errList[0])
			return "", err
		}

		// We don't need to validate the Label value since we're allowing for any form of data to be stored, similar
		// to Kubernetes Annotations. Of course, we should make sure it isn't empty.
		if value == "" {
			err = errors.Errorf("invalid empty value for label with key %v", key)
			return "", err
		}
	}

	req := &rpc.EngineVolumeSnapshotRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address: serviceAddress,
		},
		SnapshotVolume: &eptypes.VolumeSnapshotRequest{
			Name:   volumeSnapshotName,
			Labels: labels,
		},
	}
	recv, err := c.service.VolumeSnapshot(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return "", err
	}

	return recv.Snapshot.Name, nil
}

func (c *ProxyClient) SnapshotList(serviceAddress string) (snapshotDiskInfo map[string]*etypes.DiskInfo, err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to list snapshots")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to list snapshots", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address: serviceAddress,
	}
	resp, err := c.service.SnapshotList(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return nil, err
	}

	snapshotDiskInfo = map[string]*etypes.DiskInfo{}
	for k, v := range resp.Disks {
		if v.Children == nil {
			v.Children = map[string]bool{}
		}
		if v.Labels == nil {
			v.Labels = map[string]string{}
		}
		snapshotDiskInfo[k] = &etypes.DiskInfo{
			Name:        v.Name,
			Parent:      v.Parent,
			Children:    v.Children,
			Removed:     v.Removed,
			UserCreated: v.UserCreated,
			Created:     v.Created,
			Size:        v.Size,
			Labels:      v.Labels,
		}
	}
	return snapshotDiskInfo, nil
}

func (c *ProxyClient) SnapshotClone(serviceAddress, name, fromController string) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
		"name":           name,
		"fromController": fromController,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to clone snapshot")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to clone snapshot %v from %v", c.getProxyErrorPrefix(serviceAddress), name, fromController)
	}()

	req := &rpc.EngineSnapshotCloneRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address: serviceAddress,
		},
		FromController:            fromController,
		SnapshotName:              name,
		ExportBackingImageIfExist: false,
	}
	_, err = c.service.SnapshotClone(getContextWithGRPCLongTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) SnapshotCloneStatus(serviceAddress string) (status map[string]*SnapshotCloneStatus, err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get snapshot clone status")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get snapshot clone status", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address: serviceAddress,
	}
	recv, err := c.service.SnapshotCloneStatus(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return nil, err
	}

	status = map[string]*SnapshotCloneStatus{}
	for k, v := range recv.Status {
		status[k] = &SnapshotCloneStatus{
			IsCloning:          v.IsCloning,
			Error:              v.Error,
			Progress:           int(v.Progress),
			State:              v.State,
			FromReplicaAddress: v.FromReplicaAddress,
			SnapshotName:       v.SnapshotName,
		}
	}
	return status, nil
}

func (c *ProxyClient) SnapshotRevert(serviceAddress string, name string) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
		"name":           name,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to revert volume to snapshot")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to revert volume to snapshot %v", c.getProxyErrorPrefix(serviceAddress), name)
	}()

	if name == VolumeHeadName {
		err = errors.Errorf("invalid operation: cannot revert to %v", VolumeHeadName)
		return err
	}

	req := &rpc.EngineSnapshotRevertRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address: serviceAddress,
		},
		Name: name,
	}
	_, err = c.service.SnapshotRevert(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) SnapshotPurge(serviceAddress string, skipIfInProgress bool) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to purge snapshots")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to purge snapshots", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.EngineSnapshotPurgeRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address: serviceAddress,
		},
		SkipIfInProgress: skipIfInProgress,
	}
	_, err = c.service.SnapshotPurge(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}

func (c *ProxyClient) SnapshotPurgeStatus(serviceAddress string) (status map[string]*SnapshotPurgeStatus, err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get snapshot purge status")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get snapshot purge status", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address: serviceAddress,
	}

	recv, err := c.service.SnapshotPurgeStatus(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return nil, err
	}

	status = make(map[string]*SnapshotPurgeStatus)
	for k, v := range recv.Status {
		status[k] = &SnapshotPurgeStatus{
			Error:     v.Error,
			IsPurging: v.IsPurging,
			Progress:  int(v.Progress),
			State:     v.State,
		}
	}
	return status, nil
}

func (c *ProxyClient) SnapshotRemove(serviceAddress string, names []string) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrapf(err, "failed to remove snapshot %v", names)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to remove snapshot %v", c.getProxyErrorPrefix(serviceAddress), names)
	}()

	req := &rpc.EngineSnapshotRemoveRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address: serviceAddress,
		},
		Names: names,
	}
	_, err = c.service.SnapshotRemove(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	return nil
}
