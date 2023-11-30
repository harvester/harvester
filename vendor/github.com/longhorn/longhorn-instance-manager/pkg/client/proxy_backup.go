package client

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
)

func (c *ProxyClient) SnapshotBackup(backendStoreDriver, engineName, volumeName, serviceAddress, backupName,
	snapshotName, backupTarget, backingImageName, backingImageChecksum, compressionMethod string, concurrentLimit int,
	storageClassName string, labels map[string]string, envs []string) (backupID, replicaAddress string, err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return "", "", errors.Wrap(err, "failed to backup snapshot")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return "", "", fmt.Errorf("failed to backup snapshot: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to backup snapshot %v to %v", c.getProxyErrorPrefix(serviceAddress), snapshotName, backupName)
	}()

	req := &rpc.EngineSnapshotBackupRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:            serviceAddress,
			EngineName:         engineName,
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			VolumeName:         volumeName,
		},
		Envs:                 envs,
		BackupName:           backupName,
		SnapshotName:         snapshotName,
		BackupTarget:         backupTarget,
		BackingImageName:     backingImageName,
		BackingImageChecksum: backingImageChecksum,
		CompressionMethod:    compressionMethod,
		ConcurrentLimit:      int32(concurrentLimit),
		StorageClassName:     storageClassName,
		Labels:               labels,
	}
	recv, err := c.service.SnapshotBackup(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return "", "", err
	}

	return recv.BackupId, recv.Replica, nil
}

func (c *ProxyClient) SnapshotBackupStatus(backendStoreDriver, engineName, volumeName, serviceAddress, backupName,
	replicaAddress, replicaName string) (status *SnapshotBackupStatus, err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
		"backupName":     backupName,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get backup status")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return nil, fmt.Errorf("failed to get backup status: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get %v backup status", c.getProxyErrorPrefix(serviceAddress), backupName)
	}()

	req := &rpc.EngineSnapshotBackupStatusRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:            serviceAddress,
			EngineName:         engineName,
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			VolumeName:         volumeName,
		},
		BackupName:     backupName,
		ReplicaAddress: replicaAddress,
		// For now, it is unlikely we actually know replicaName. Pass it anyway, as an empty string will not cause a
		// validation failure and this may change in the future.
		ReplicaName: replicaName,
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

func (c *ProxyClient) BackupRestore(backendStoreDriver, engineName, volumeName, serviceAddress, url, target,
	backupVolumeName string, envs []string, concurrentLimit int) (err error) {
	input := map[string]string{
		"engineName":       engineName,
		"volumeName":       volumeName,
		"serviceAddress":   serviceAddress,
		"url":              url,
		"target":           target,
		"backupVolumeName": backupVolumeName,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return errors.Wrap(err, "failed to restore backup to volume")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return fmt.Errorf("failed to restore backup to volume: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		if _, ok := err.(TaskError); ok {
			return
		}

		err = errors.Wrapf(err, "%v failed to restore backup %v to volume %v", c.getProxyErrorPrefix(serviceAddress), url, volumeName)
	}()

	req := &rpc.EngineBackupRestoreRequest{
		ProxyEngineRequest: &rpc.ProxyEngineRequest{
			Address:            serviceAddress,
			EngineName:         engineName,
			BackendStoreDriver: rpc.BackendStoreDriver(driver),
			// This is the name we will use for validation when communicating with the restoring engine.
			VolumeName: volumeName,
		},
		Envs:   envs,
		Url:    url,
		Target: target,
		// Historically, we have passed backupVolumeName as VolumeName here.
		VolumeName:      backupVolumeName,
		ConcurrentLimit: int32(concurrentLimit),
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

func (c *ProxyClient) BackupRestoreStatus(backendStoreDriver, engineName, volumeName,
	serviceAddress string) (status map[string]*BackupRestoreStatus, err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get backup restore status")
	}

	driver, ok := rpc.BackendStoreDriver_value[backendStoreDriver]
	if !ok {
		return nil, fmt.Errorf("failed to get backup restore status: invalid backend store driver %v", backendStoreDriver)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get backup restore status", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address:            serviceAddress,
		BackendStoreDriver: rpc.BackendStoreDriver(driver),
		EngineName:         engineName,
		VolumeName:         volumeName,
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
