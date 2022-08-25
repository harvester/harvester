package client

import (
	"encoding/json"

	"github.com/pkg/errors"

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
	recv, err := c.service.SnapshotBackup(getContextWithGRPCTimeout(c.ctx), req)
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
	recv, err := c.service.SnapshotBackupStatus(getContextWithGRPCTimeout(c.ctx), req)
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
		if _, ok := err.(TaskError); ok {
			return
		}

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
	recv, err := c.service.BackupRestore(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return err
	}

	if recv.TaskError != nil {
		var taskErr TaskError
		if jsonErr := json.Unmarshal(recv.TaskError, &taskErr); jsonErr != nil {
			err = errors.Wrapf(jsonErr, "cannot unmarshal the restore error, maybe it's not caused by the replica restore failure: %s", recv.TaskError)
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
	recv, err := c.service.BackupRestoreStatus(getContextWithGRPCTimeout(c.ctx), req)
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
