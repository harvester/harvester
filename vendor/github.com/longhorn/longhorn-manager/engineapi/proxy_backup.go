package engineapi

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/pkg/errors"

	"github.com/longhorn/backupstore"

	imclient "github.com/longhorn/longhorn-instance-manager/pkg/client"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
)

func (p *Proxy) SnapshotBackup(e *longhorn.Engine,
	snapshotName, backupName, backupTarget,
	backingImageName, backingImageChecksum string,
	labels, credential map[string]string) (string, string, error) {
	if snapshotName == VolumeHeadName {
		return "", "", fmt.Errorf("invalid operation: cannot backup %v", VolumeHeadName)
	}

	snap, err := p.SnapshotGet(e, snapshotName)
	if err != nil {
		return "", "", errors.Wrapf(err, "error getting snapshot '%s', engine '%s'", snapshotName, e.Name)
	}

	if snap == nil {
		return "", "", errors.Errorf("could not find snapshot '%s' to backup, engine '%s'", snapshotName, e.Name)
	}

	// get environment variables if backup for s3
	credentialEnv, err := getBackupCredentialEnv(backupTarget, credential)
	if err != nil {
		return "", "", err
	}

	backupID, replicaAddress, err := p.grpcClient.SnapshotBackup(p.DirectToURL(e),
		backupName, snapshotName, backupTarget,
		backingImageName, backingImageChecksum,
		labels, credentialEnv,
	)
	if err != nil {
		return "", "", err
	}

	return backupID, replicaAddress, nil
}

func (p *Proxy) SnapshotBackupStatus(e *longhorn.Engine, backupName, replicaAddress string) (status *longhorn.EngineBackupStatus, err error) {
	recv, err := p.grpcClient.SnapshotBackupStatus(p.DirectToURL(e), backupName, replicaAddress)
	if err != nil {
		return nil, err
	}

	return (*longhorn.EngineBackupStatus)(recv), nil
}

func (p *Proxy) BackupRestore(e *longhorn.Engine, backupTarget, backupName, backupVolumeName, lastRestored string, credential map[string]string) error {
	backupURL := backupstore.EncodeBackupURL(backupName, backupVolumeName, backupTarget)

	// get environment variables if backup for s3
	envs, err := getBackupCredentialEnv(backupTarget, credential)
	if err != nil {
		return err
	}

	return p.grpcClient.BackupRestore(p.DirectToURL(e), backupURL, backupTarget, backupVolumeName, envs)
}

func (p *Proxy) BackupRestoreStatus(e *longhorn.Engine) (status map[string]*longhorn.RestoreStatus, err error) {
	recv, err := p.grpcClient.BackupRestoreStatus(p.DirectToURL(e))
	if err != nil {
		return nil, err
	}

	status = map[string]*longhorn.RestoreStatus{}
	for k, v := range recv {
		status[k] = (*longhorn.RestoreStatus)(v)
	}
	return status, nil
}

func (p *Proxy) BackupGet(destURL string, credential map[string]string) (*Backup, error) {
	// get environment variables if backup for s3
	envs, err := getBackupCredentialEnv(destURL, credential)
	if err != nil {
		return nil, err
	}

	recv, err := p.grpcClient.BackupGet(destURL, envs)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return parseBackup(recv), nil
}

func (p *Proxy) BackupVolumeGet(destURL string, credential map[string]string) (volume *BackupVolume, err error) {
	// get environment variables if backup for s3
	envs, err := getBackupCredentialEnv(destURL, credential)
	if err != nil {
		return nil, err
	}

	recv, err := p.grpcClient.BackupVolumeGet(destURL, envs)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	parseBackups := func(in map[string]*imclient.EngineBackupInfo) (out map[string]*Backup) {
		out = map[string]*Backup{}
		for k, v := range in {
			out[k] = parseBackup(v)
		}
		return out
	}

	volume = &BackupVolume{
		Name:                 recv.Name,
		Size:                 strconv.FormatInt(recv.Size, 10),
		Labels:               recv.Labels,
		Created:              recv.Created,
		LastBackupName:       recv.LastBackupName,
		LastBackupAt:         recv.LastBackupAt,
		DataStored:           strconv.FormatInt(recv.DataStored, 10),
		Messages:             recv.Messages,
		Backups:              parseBackups(recv.Backups),
		BackingImageName:     recv.BackingImageName,
		BackingImageChecksum: recv.BackingImageChecksum,
	}
	return volume, nil
}

func parseBackup(in *imclient.EngineBackupInfo) (out *Backup) {
	return &Backup{
		Name:                   in.Name,
		State:                  "",
		URL:                    in.URL,
		SnapshotName:           in.SnapshotName,
		SnapshotCreated:        in.SnapshotCreated,
		Created:                in.Created,
		Size:                   strconv.FormatInt(in.Size, 10),
		Labels:                 in.Labels,
		VolumeName:             in.VolumeName,
		VolumeSize:             strconv.FormatInt(in.VolumeSize, 10),
		VolumeCreated:          in.VolumeCreated,
		VolumeBackingImageName: in.VolumeBackingImageName,
		Messages:               in.Messages,
	}
}

func (p *Proxy) BackupNameList(destURL, volumeName string, credential map[string]string) (names []string, err error) {
	if volumeName == "" {
		return nil, nil
	}

	// get environment variables if backup for s3
	envs, err := getBackupCredentialEnv(destURL, credential)
	if err != nil {
		return nil, err
	}

	recv, err := p.grpcClient.BackupVolumeList(destURL, volumeName, false, envs)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	volume, ok := recv[volumeName]
	if !ok {
		return nil, fmt.Errorf("cannot find %s in the backups", volumeName)
	}

	names = []string{}
	if volume.Messages[string(backupstore.MessageTypeError)] != "" {
		return names, errors.New(volume.Messages[string(backupstore.MessageTypeError)])
	}

	for backupName := range volume.Backups {
		names = append(names, backupName)
	}
	sort.Strings(names)
	return names, nil
}

func (p *Proxy) BackupVolumeNameList(destURL string, credential map[string]string) (names []string, err error) {
	// get environment variables if backup for s3
	envs, err := getBackupCredentialEnv(destURL, credential)
	if err != nil {
		return nil, err
	}

	recv, err := p.grpcClient.BackupVolumeList(destURL, "", true, envs)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	names = []string{}
	for volumeName := range recv {
		names = append(names, volumeName)
	}
	sort.Strings(names)
	return names, nil
}

func (p *Proxy) BackupDelete(destURL string, credential map[string]string) (err error) {
	// get environment variables if backup for s3
	envs, err := getBackupCredentialEnv(destURL, credential)
	if err != nil {
		return err
	}

	err = p.grpcClient.BackupRemove(destURL, "", envs)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil
		}
		return err
	}

	return nil
}

func (p *Proxy) BackupVolumeDelete(destURL, volumeName string, credential map[string]string) (err error) {
	// get environment variables if backup for s3
	envs, err := getBackupCredentialEnv(destURL, credential)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil
		}
		return err
	}

	return p.grpcClient.BackupRemove(destURL, volumeName, envs)
}

func (p *Proxy) BackupConfigMetaGet(destURL string, credential map[string]string) (*ConfigMetadata, error) {
	envs, err := getBackupCredentialEnv(destURL, credential)
	if err != nil {
		return nil, err
	}

	recv, err := p.grpcClient.BackupConfigMetaGet(destURL, envs)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return (*ConfigMetadata)(recv), nil
}
