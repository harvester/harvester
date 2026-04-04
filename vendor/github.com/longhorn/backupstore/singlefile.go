package backupstore

import (
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	. "github.com/longhorn/backupstore/logging"
	"github.com/longhorn/backupstore/util"
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

	if err := addVolume(driver, volume); err != nil {
		return "", err
	}

	volume, err = loadVolume(driver, volume.Name)
	if err != nil {
		return "", err
	}

	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonStart,
		LogFieldEvent:    LogEventBackup,
		LogFieldObject:   LogObjectSnapshot,
		LogFieldSnapshot: snapshot.Name,
		LogFieldFilepath: filePath,
	}).Info("Creating backup")

	backup := &Backup{
		Name:              util.GenerateName("backup"),
		VolumeName:        volume.Name,
		SnapshotName:      snapshot.Name,
		SnapshotCreatedAt: snapshot.CreatedTime,
		CompressionMethod: volume.CompressionMethod,
	}
	backup.SingleFile.FilePath = getSingleFileBackupFilePath(backup)

	if err := driver.Upload(filePath, backup.SingleFile.FilePath); err != nil {
		return "", err
	}

	backup.CreatedTime = util.Now()
	if err := saveBackup(driver, backup); err != nil {
		return "", err
	}

	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonComplete,
		LogFieldEvent:    LogEventBackup,
		LogFieldObject:   LogObjectSnapshot,
		LogFieldSnapshot: snapshot.Name,
	}).Info("Created backup")

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

	if _, err := loadVolume(driver, srcVolumeName); err != nil {
		return "", generateError(logrus.Fields{
			LogFieldVolume:    srcVolumeName,
			LogFieldBackupURL: backupURL,
		}, "Volume doesn't exist in backupstore: %v", err)
	}

	backup, err := loadBackup(driver, srcBackupName, srcVolumeName)
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

	_, err = loadVolume(driver, volumeName)
	if err != nil {
		return errors.Wrapf(err, "cannot find volume %v in backupstore", volumeName)
	}

	backup, err := loadBackup(driver, backupName, volumeName)
	if err != nil {
		return err
	}

	if err := driver.Remove(backup.SingleFile.FilePath); err != nil {
		return err
	}

	return removeBackup(backup, driver)
}
