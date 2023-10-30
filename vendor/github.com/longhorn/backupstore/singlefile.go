package backupstore

import (
	"path/filepath"

	"github.com/longhorn/backupstore/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	. "github.com/longhorn/backupstore/logging"
)

const (
	BACKUP_FILES_DIRECTORY = "BackupFiles"
)

type BackupFile struct {
	FilePath string
}

func getSingleFileBackupFilePath(sfBackup *Backup) string {
	backupFileName := sfBackup.Name + ".bak"
	return filepath.Join(getVolumePath(sfBackup.VolumeName), BACKUP_FILES_DIRECTORY, backupFileName)
}

func CreateSingleFileBackup(volume *Volume, snapshot *Snapshot, filePath, destURL string) (string, error) {
	driver, err := GetBackupStoreDriver(destURL)
	if err != nil {
		return "", err
	}

	if err := addVolume(volume, driver); err != nil {
		return "", err
	}

	volume, err = loadVolume(volume.Name, driver)
	if err != nil {
		return "", err
	}

	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonStart,
		LogFieldEvent:    LogEventBackup,
		LogFieldObject:   LogObjectSnapshot,
		LogFieldSnapshot: snapshot.Name,
		LogFieldFilepath: filePath,
	}).Debug("Creating backup")

	backup := &Backup{
		Name:              util.GenerateName("backup"),
		VolumeName:        volume.Name,
		SnapshotName:      snapshot.Name,
		SnapshotCreatedAt: snapshot.CreatedTime,
	}
	backup.SingleFile.FilePath = getSingleFileBackupFilePath(backup)

	if err := driver.Upload(filePath, backup.SingleFile.FilePath); err != nil {
		return "", err
	}

	backup.CreatedTime = util.Now()
	if err := saveBackup(backup, driver); err != nil {
		return "", err
	}

	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonComplete,
		LogFieldEvent:    LogEventBackup,
		LogFieldObject:   LogObjectSnapshot,
		LogFieldSnapshot: snapshot.Name,
	}).Debug("Created backup")

	return EncodeBackupURL(backup.Name, volume.Name, destURL), nil
}

func RestoreSingleFileBackup(backupURL, path string) (string, error) {
	driver, err := GetBackupStoreDriver(backupURL)
	if err != nil {
		return "", err
	}

	srcBackupName, srcVolumeName, _, err := DecodeBackupURL(backupURL)
	if err != nil {
		return "", err
	}

	if _, err := loadVolume(srcVolumeName, driver); err != nil {
		return "", generateError(logrus.Fields{
			LogFieldVolume:    srcVolumeName,
			LogEventBackupURL: backupURL,
		}, "Volume doesn't exist in backupstore: %v", err)
	}

	backup, err := loadBackup(srcBackupName, srcVolumeName, driver)
	if err != nil {
		return "", err
	}

	dstFile := filepath.Join(path, filepath.Base(backup.SingleFile.FilePath))
	if err := driver.Download(backup.SingleFile.FilePath, dstFile); err != nil {
		return "", err
	}

	return dstFile, nil
}

func DeleteSingleFileBackup(backupURL string) error {
	driver, err := GetBackupStoreDriver(backupURL)
	if err != nil {
		return err
	}

	backupName, volumeName, _, err := DecodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	_, err = loadVolume(volumeName, driver)
	if err != nil {
		return errors.Wrapf(err, "cannot find volume %v in backupstore", volumeName)
	}

	backup, err := loadBackup(backupName, volumeName, driver)
	if err != nil {
		return err
	}

	if err := driver.Remove(backup.SingleFile.FilePath); err != nil {
		return err
	}

	return removeBackup(backup, driver)
}
