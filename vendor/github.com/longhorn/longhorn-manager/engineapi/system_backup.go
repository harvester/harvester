package engineapi

import (
	"encoding/json"

	"github.com/pkg/errors"

	systembackupstore "github.com/longhorn/backupstore/systembackup"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type SystemBackupOperationInterface interface {
	DeleteSystemBackup(systemBackup *longhorn.SystemBackup) (string, error)
	DownloadSystemBackup(name, version, downloadPath string) error
	GetSystemBackupConfig(name, version string) (*systembackupstore.Config, error)
	ListSystemBackup() (systembackupstore.SystemBackups, error)
	UploadSystemBackup(name, localFile, longhornVersion, longhornGitCommit, managerImage, engineImage string) (string, error)
}

// DeleteSystemBackup deletes system backup in the backup target
func (btc *BackupTargetClient) DeleteSystemBackup(systemBackup *longhorn.SystemBackup) (string, error) {
	systemBackupURL, err := systembackupstore.GetSystemBackupURL(systemBackup.Name, systemBackup.Status.Version, btc.URL)
	if err != nil {
		return "", err
	}

	output, err := btc.ExecuteEngineBinary(
		"system-backup", "delete", systemBackupURL,
	)
	if err != nil {
		return "", errors.Wrapf(err, "error deleting system backup %v", systemBackupURL)
	}
	return output, nil
}

// DownloadSystemBackup downloads system backup from the backup target
func (btc *BackupTargetClient) DownloadSystemBackup(name, version, downloadPath string) error {
	systemBackupURL, err := systembackupstore.GetSystemBackupURL(name, version, btc.URL)
	if err != nil {
		return err
	}

	_, err = btc.ExecuteEngineBinaryWithTimeout(
		datastore.SystemRestoreTimeout,
		"system-backup", "download", systemBackupURL, downloadPath,
	)
	if err != nil {
		return errors.Wrapf(err, "error downloading system backup %v", systemBackupURL)
	}
	return nil
}

// GetSystemBackupConfig returns the system backup config from the backup target
func (btc *BackupTargetClient) GetSystemBackupConfig(name, version string) (*systembackupstore.Config, error) {
	systemBackupURL, err := systembackupstore.GetSystemBackupURL(name, version, btc.URL)
	if err != nil {
		return nil, err
	}

	output, err := btc.ExecuteEngineBinary(
		"system-backup", "get-config", systemBackupURL,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting config for system backup %v", systemBackupURL)
	}

	cfg := &systembackupstore.Config{}
	if err := json.Unmarshal([]byte(output), &cfg); err != nil {
		return nil, errors.Wrapf(err, "error parsing config for system backup %v: %v", name, output)
	}

	return cfg, nil
}

// ListSystemBackup returns a list of system backups in backup target
func (btc *BackupTargetClient) ListSystemBackup() (systembackupstore.SystemBackups, error) {
	output, err := btc.ExecuteEngineBinary("system-backup", "list", btc.URL)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil, nil
		}

		return nil, errors.Wrapf(err, "error listing system backup in %v", btc.URL)
	}

	systemBackups := systembackupstore.SystemBackups{}
	if err := json.Unmarshal([]byte(output), &systemBackups); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal system backup: %s", output)
	}
	return systemBackups, nil
}

// UploadSystemBackup uploads system backup to the backup target
func (btc *BackupTargetClient) UploadSystemBackup(name, localFile, longhornVersion, longhornGitCommit, managerImage, engineImage string) (string, error) {
	systemBackupURL, err := systembackupstore.GetSystemBackupURL(name, longhornVersion, btc.URL)
	if err != nil {
		return "", err
	}

	output, err := btc.ExecuteEngineBinaryWithTimeout(
		datastore.SystemBackupTimeout,
		"system-backup", "upload", localFile, systemBackupURL,
		"--git-commit", longhornGitCommit,
		"--manager-image", managerImage,
		"--engine-image", engineImage,
	)
	if err != nil {
		return "", errors.Wrapf(err, "error uploading system backup from %v to %v", localFile, systemBackupURL)
	}
	return output, nil
}
