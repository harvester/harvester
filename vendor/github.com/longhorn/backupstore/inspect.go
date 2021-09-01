package backupstore

import (
	"fmt"

	"github.com/sirupsen/logrus"

	. "github.com/longhorn/backupstore/logging"
)

func InspectVolume(volumeURL string) (*VolumeInfo, error) {
	driver, err := GetBackupStoreDriver(volumeURL)
	if err != nil {
		return nil, err
	}

	_, volumeName, _, err := DecodeBackupURL(volumeURL)
	if err != nil {
		return nil, err
	}

	volume, err := loadVolume(volumeName, driver)
	if err != nil {
		return nil, err
	}

	return fillVolumeInfo(volume), nil
}

func InspectBackup(backupURL string) (*BackupInfo, error) {
	driver, err := GetBackupStoreDriver(backupURL)
	if err != nil {
		return nil, err
	}

	backupName, volumeName, _, err := DecodeBackupURL(backupURL)
	if err != nil {
		return nil, err
	}

	volume, err := loadVolume(volumeName, driver)
	if err != nil {
		return nil, err
	}

	backup, err := loadBackup(backupName, volumeName, driver)
	if err != nil {
		log.WithFields(logrus.Fields{
			LogFieldReason: LogReasonFallback,
			LogFieldEvent:  LogEventList,
			LogFieldObject: LogObjectBackup,
			LogFieldBackup: backupName,
			LogFieldVolume: volumeName,
		}).Info("Failed to load backup in backupstore")
		return nil, err
	} else if isBackupInProgress(backup) {
		// for now we don't return in progress backups to the ui
		return nil, fmt.Errorf("backup %v is still in progress", backup.Name)
	}

	return fillFullBackupInfo(backup, volume, driver.GetURL()), nil
}

func fillVolumeInfo(volume *Volume) *VolumeInfo {
	return &VolumeInfo{
		Name:                 volume.Name,
		Size:                 volume.Size,
		Labels:               volume.Labels,
		Created:              volume.CreatedTime,
		LastBackupName:       volume.LastBackupName,
		LastBackupAt:         volume.LastBackupAt,
		DataStored:           int64(volume.BlockCount * DEFAULT_BLOCK_SIZE),
		Messages:             make(map[MessageType]string),
		Backups:              make(map[string]*BackupInfo),
		BackingImageName:     volume.BackingImageName,
		BackingImageChecksum: volume.BackingImageChecksum,
	}
}

func fillBackupInfo(backup *Backup, destURL string) *BackupInfo {
	return &BackupInfo{
		Name:            backup.Name,
		URL:             EncodeBackupURL(backup.Name, backup.VolumeName, destURL),
		SnapshotName:    backup.SnapshotName,
		SnapshotCreated: backup.SnapshotCreatedAt,
		Created:         backup.CreatedTime,
		Size:            backup.Size,
		Labels:          backup.Labels,
		IsIncremental:   backup.IsIncremental,
	}
}

func fillFullBackupInfo(backup *Backup, volume *Volume, destURL string) *BackupInfo {
	info := fillBackupInfo(backup, destURL)
	info.VolumeName = volume.Name
	info.VolumeSize = volume.Size
	info.VolumeCreated = volume.CreatedTime
	info.VolumeBackingImageName = volume.BackingImageName
	return info
}
