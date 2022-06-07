package client

import (
	"encoding/json"

	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"

	"github.com/longhorn/backupstore"

	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
)

func (c *ProxyClient) SnapshotBackup(serviceAddress,
	backupName, snapshotName, backupTarget,
	backingImageName, backingImageChecksum string,
	labels map[string]string, envs []string) (backupID, replicaAddress string, err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return "", "", errors.Wrap(err, "failed to backup snapshot")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to backup snapshot %v to %v", c.getProxyErrorPrefix(serviceAddress), snapshotName, backupName)
	}()

	req := &rpc.EngineSnapshotBackupRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address: serviceAddress,
		},
		Envs:                 envs,
		BackupName:           backupName,
		SnapshotName:         snapshotName,
		BackupTarget:         backupTarget,
		BackingImageName:     backingImageName,
		BackingImageChecksum: backingImageChecksum,
		Labels:               labels,
	}
	recv, err := c.service.SnapshotBackup(c.ctx, req)
	if err != nil {
		return "", "", err
	}

	return recv.BackupId, recv.Replica, nil
}

func (c *ProxyClient) SnapshotBackupStatus(serviceAddress, backupName, replicaAddress string) (status *SnapshotBackupStatus, err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
		"backupName":     backupName,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get backup status")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get %v backup status", c.getProxyErrorPrefix(serviceAddress), backupName)
	}()

	req := &rpc.EngineSnapshotBackupStatusRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address: serviceAddress,
		},
		BackupName:     backupName,
		ReplicaAddress: replicaAddress,
	}
	recv, err := c.service.SnapshotBackupStatus(c.ctx, req)
	if err != nil {
		return nil, err
	}

	status = &SnapshotBackupStatus{
		Progress:       int(recv.Progress),
		BackupURL:      recv.BackupUrl,
		Error:          recv.Error,
		SnapshotName:   recv.SnapshotName,
		State:          recv.State,
		ReplicaAddress: recv.ReplicaAddress,
	}
	return status, nil
}

func (c *ProxyClient) BackupRestore(serviceAddress, url, target, volumeName string, envs []string) (err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
		"url":            url,
		"target":         target,
		"volumeName":     volumeName,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to restore backup to volume")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to restore backup %v to volume %v", c.getProxyErrorPrefix(serviceAddress), url, volumeName)
	}()

	req := &rpc.EngineBackupRestoreRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address: serviceAddress,
		},
		Envs:       envs,
		Url:        url,
		Target:     target,
		VolumeName: volumeName,
	}
	recv, err := c.service.BackupRestore(c.ctx, req)
	if err != nil {
		return err
	}

	if recv.TaskError != nil {
		var taskErr TaskError
		if jsonErr := json.Unmarshal(recv.TaskError, &taskErr); jsonErr != nil {
			err = errors.Wrap(jsonErr, "Cannot unmarshal the restore error, maybe it's not caused by the replica restore failure")
			return err
		}

		err = taskErr
		return err
	}

	return nil
}

func (c *ProxyClient) BackupRestoreStatus(serviceAddress string) (status map[string]*BackupRestoreStatus, err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get backup restore status")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get backup restore status", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address: serviceAddress,
	}
	recv, err := c.service.BackupRestoreStatus(c.ctx, req)
	if err != nil {
		return nil, err
	}

	status = map[string]*BackupRestoreStatus{}
	for k, v := range recv.Status {
		status[k] = &BackupRestoreStatus{
			IsRestoring:            v.IsRestoring,
			LastRestored:           v.LastRestored,
			CurrentRestoringBackup: v.CurrentRestoringBackup,
			Progress:               int(v.Progress),
			Error:                  v.Error,
			Filename:               v.Filename,
			State:                  v.State,
			BackupURL:              v.BackupUrl,
		}
	}
	return status, nil
}

func (c *ProxyClient) BackupGet(destURL string, envs []string) (info *EngineBackupInfo, err error) {
	input := map[string]string{
		"destURL": destURL,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get backup")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get backup", c.getProxyErrorPrefix(destURL))
	}()

	req := &rpc.EngineBackupGetRequest{
		Envs:    envs,
		DestUrl: destURL,
	}
	recv, err := c.service.BackupGet(c.ctx, req)
	if err != nil {
		return nil, err
	}

	return parseBackup(recv.Backup), nil
}

func (c *ProxyClient) BackupVolumeGet(destURL string, envs []string) (info *EngineBackupVolumeInfo, err error) {
	input := map[string]string{
		"destURL": destURL,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get backup volume")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get backup volume", c.getProxyErrorPrefix(destURL))
	}()

	req := &rpc.EngineBackupVolumeGetRequest{
		Envs:    envs,
		DestUrl: destURL,
	}
	recv, err := c.service.BackupVolumeGet(c.ctx, req)
	if err != nil {
		return nil, err
	}

	info = &EngineBackupVolumeInfo{
		Name:                 recv.Volume.Name,
		Size:                 recv.Volume.Size,
		Labels:               recv.Volume.Labels,
		Created:              recv.Volume.Created,
		LastBackupName:       recv.Volume.LastBackupName,
		LastBackupAt:         recv.Volume.LastBackupAt,
		DataStored:           recv.Volume.DataStored,
		Messages:             recv.Volume.Messages,
		Backups:              parseBackups(recv.Volume.Backups),
		BackingImageName:     recv.Volume.BackingImageName,
		BackingImageChecksum: recv.Volume.BackingImageChecksum,
	}
	return info, nil
}

func (c *ProxyClient) BackupVolumeList(destURL, volumeName string, volumeOnly bool, envs []string) (info map[string]*EngineBackupVolumeInfo, err error) {
	input := map[string]string{
		"destURL": destURL,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to list backup for volumes")
	}

	defer func() {
		if volumeName == "" {
			err = errors.Wrapf(err, "%v failed to list backup for volumes", c.getProxyErrorPrefix(destURL))
		} else {
			err = errors.Wrapf(err, "%v failed to list backup for volume %v", c.getProxyErrorPrefix(destURL), volumeName)
		}
	}()

	req := &rpc.EngineBackupVolumeListRequest{
		Envs:       envs,
		DestUrl:    destURL,
		VolumeName: volumeName,
		VolumeOnly: volumeOnly,
	}
	recv, err := c.service.BackupVolumeList(c.ctx, req)
	if err != nil {
		return nil, err
	}

	info = map[string]*EngineBackupVolumeInfo{}
	for k, v := range recv.Volumes {
		info[k] = &EngineBackupVolumeInfo{
			Name:                 v.Name,
			Size:                 v.Size,
			Labels:               v.Labels,
			Created:              v.Created,
			LastBackupName:       v.LastBackupName,
			LastBackupAt:         v.LastBackupAt,
			DataStored:           v.DataStored,
			Messages:             v.Messages,
			Backups:              parseBackups(v.Backups),
			BackingImageName:     v.BackingImageName,
			BackingImageChecksum: v.BackingImageChecksum,
		}
	}
	return info, nil
}

func parseBackups(in map[string]*rpc.EngineBackupInfo) (out map[string]*EngineBackupInfo) {
	out = map[string]*EngineBackupInfo{}
	for k, v := range in {
		out[k] = parseBackup(v)
	}
	return out
}

func parseBackup(in *rpc.EngineBackupInfo) (out *EngineBackupInfo) {
	return &EngineBackupInfo{
		Name:                   in.Name,
		URL:                    in.Url,
		SnapshotName:           in.SnapshotName,
		SnapshotCreated:        in.SnapshotCreated,
		Created:                in.Created,
		Size:                   in.Size,
		Labels:                 in.Labels,
		IsIncremental:          in.IsIncremental,
		VolumeName:             in.VolumeName,
		VolumeSize:             in.VolumeSize,
		VolumeCreated:          in.VolumeCreated,
		VolumeBackingImageName: in.VolumeBackingImageName,
		Messages:               in.Messages,
	}
}

func (c *ProxyClient) BackupConfigMetaGet(destURL string, envs []string) (meta *backupstore.ConfigMetadata, err error) {
	input := map[string]string{
		"destURL": destURL,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get backup config metadata")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get backup config metadata", c.getProxyErrorPrefix(destURL))
	}()

	req := &rpc.EngineBackupConfigMetaGetRequest{
		Envs:    envs,
		DestUrl: destURL,
	}
	recv, err := c.service.BackupConfigMetaGet(c.ctx, req)
	if err != nil {
		return nil, err
	}

	ts, err := ptypes.Timestamp(recv.ModificationTime)
	if err != nil {
		return nil, err
	}

	return &backupstore.ConfigMetadata{
		ModificationTime: ts,
	}, nil
}

func (c *ProxyClient) BackupRemove(destURL, volumeName string, envs []string) (err error) {
	input := map[string]string{
		"destURL": destURL,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to remove backup")
	}

	defer func() {
		if volumeName == "" {
			err = errors.Wrapf(err, "%v failed to remove backup", c.getProxyErrorPrefix(destURL))
		} else {
			err = errors.Wrapf(err, "%v failed to remove backup for volume %v", c.getProxyErrorPrefix(destURL), volumeName)
		}
	}()

	req := &rpc.EngineBackupRemoveRequest{
		Envs:       envs,
		DestUrl:    destURL,
		VolumeName: volumeName,
	}
	_, err = c.service.BackupRemove(c.ctx, req)
	if err != nil {
		return err
	}

	return nil
}
