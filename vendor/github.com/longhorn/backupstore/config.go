package backupstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/honestbee/jobq"
	"github.com/sirupsen/logrus"

	. "github.com/longhorn/backupstore/logging"
	"github.com/longhorn/backupstore/util"
)

const (
	VOLUME_SEPARATE_LAYER1 = 2
	VOLUME_SEPARATE_LAYER2 = 4

	VOLUME_DIRECTORY     = "volumes"
	VOLUME_CONFIG_FILE   = "volume.cfg"
	BACKUP_DIRECTORY     = "backups"
	BACKUP_CONFIG_PREFIX = "backup_"

	CFG_SUFFIX = ".cfg"
)

func getBackupConfigName(id string) string {
	return BACKUP_CONFIG_PREFIX + id + CFG_SUFFIX
}

func LoadConfigInBackupStore(filePath string, driver BackupStoreDriver, v interface{}) error {
	if !driver.FileExists(filePath) {
		return fmt.Errorf("cannot find %v in backupstore", filePath)
	}
	rc, err := driver.Read(filePath)
	if err != nil {
		return err
	}
	defer rc.Close()

	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonStart,
		LogFieldObject:   LogObjectConfig,
		LogFieldKind:     driver.Kind(),
		LogFieldFilepath: filePath,
	}).Debug()
	if err := json.NewDecoder(rc).Decode(v); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonComplete,
		LogFieldObject:   LogObjectConfig,
		LogFieldKind:     driver.Kind(),
		LogFieldFilepath: filePath,
	}).Debug()
	return nil
}

func SaveConfigInBackupStore(filePath string, driver BackupStoreDriver, v interface{}) error {
	j, err := json.Marshal(v)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonStart,
		LogFieldObject:   LogObjectConfig,
		LogFieldKind:     driver.Kind(),
		LogFieldFilepath: filePath,
	}).Debug()
	if err := driver.Write(filePath, bytes.NewReader(j)); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonComplete,
		LogFieldObject:   LogObjectConfig,
		LogFieldKind:     driver.Kind(),
		LogFieldFilepath: filePath,
	}).Debug()
	return nil
}

func SaveLocalFileToBackupStore(localFilePath, backupStoreFilePath string, driver BackupStoreDriver) error {
	log := log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonStart,
		LogFieldObject:   LogObjectConfig,
		LogFieldKind:     driver.Kind(),
		LogFieldFilepath: localFilePath,
		LogFieldDestURL:  backupStoreFilePath,
	})
	log.Debug()

	if driver.FileExists(backupStoreFilePath) {
		return fmt.Errorf("%v already exists", backupStoreFilePath)
	}

	if err := driver.Upload(localFilePath, backupStoreFilePath); err != nil {
		return err
	}

	log.WithField(LogFieldReason, LogReasonComplete).Debug()
	return nil
}

func SaveBackupStoreToLocalFile(backupStoreFileURL, localFilePath string, driver BackupStoreDriver) error {
	log := log.WithFields(logrus.Fields{
		LogFieldReason:    LogReasonStart,
		LogFieldObject:    LogObjectConfig,
		LogFieldKind:      driver.Kind(),
		LogFieldFilepath:  localFilePath,
		LogFieldSourceURL: backupStoreFileURL,
	})
	log.Debug()

	if err := driver.Download(backupStoreFileURL, localFilePath); err != nil {
		return err
	}

	log = log.WithFields(logrus.Fields{
		LogFieldReason: LogReasonComplete,
	})
	log.Debug()
	return nil
}

func volumeExists(volumeName string, driver BackupStoreDriver) bool {
	volumeFile := getVolumeFilePath(volumeName)
	return driver.FileExists(volumeFile)
}

func getVolumePath(volumeName string) string {
	checksum := util.GetChecksum([]byte(volumeName))
	volumeLayer1 := checksum[0:VOLUME_SEPARATE_LAYER1]
	volumeLayer2 := checksum[VOLUME_SEPARATE_LAYER1:VOLUME_SEPARATE_LAYER2]
	return filepath.Join(backupstoreBase, VOLUME_DIRECTORY, volumeLayer1, volumeLayer2, volumeName) + "/"
}

func getVolumeFilePath(volumeName string) string {
	volumePath := getVolumePath(volumeName)
	volumeCfg := VOLUME_CONFIG_FILE
	return filepath.Join(volumePath, volumeCfg)
}

// getVolumeNames returns all volume names based on the folders on the backupstore
func getVolumeNames(jobQueues *jobq.WorkerDispatcher, jobQueueTimeout time.Duration, driver BackupStoreDriver) ([]string, error) {
	names := []string{}
	volumePathBase := filepath.Join(backupstoreBase, VOLUME_DIRECTORY)
	lv1Dirs, err := driver.List(volumePathBase)
	if err != nil {
		log.WithError(err).Warnf("Failed to list first level dirs for path %v", volumePathBase)
		return names, err
	}

	var (
		lv1Trackers []jobq.JobTracker
		lv2Trackers []jobq.JobTracker
		errs        []string
	)
	for _, lv1Dir := range lv1Dirs {
		path := filepath.Join(volumePathBase, lv1Dir)
		lv1Tracker := jobQueues.QueueTimedFunc(context.Background(), func(ctx context.Context) (interface{}, error) {
			lv2Dirs, err := driver.List(path)
			if err != nil {
				log.WithError(err).Warnf("Failed to list second level dirs for path %v", path)
				return nil, err
			}

			lv2Paths := make([]string, len(lv2Dirs))
			for i := range lv2Dirs {
				lv2Paths[i] = filepath.Join(path, lv2Dirs[i])
			}
			return lv2Paths, nil
		}, jobQueueTimeout)
		lv1Trackers = append(lv1Trackers, lv1Tracker)
	}

	for _, lv1Tracker := range lv1Trackers {
		payload, err := lv1Tracker.Result()
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}

		lv2Paths := payload.([]string)
		for _, lv2Path := range lv2Paths {
			path := lv2Path
			lv2Tracker := jobQueues.QueueTimedFunc(context.Background(), func(ctx context.Context) (interface{}, error) {
				volumeNames, err := driver.List(path)
				if err != nil {
					log.WithError(err).Warnf("Failed to list volume names for path %v", path)
					return nil, err
				}
				return volumeNames, nil
			}, jobQueueTimeout)
			lv2Trackers = append(lv2Trackers, lv2Tracker)
		}
	}
	for _, lv2Tracker := range lv2Trackers {
		payload, err := lv2Tracker.Result()
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		volumeNames := payload.([]string)
		names = append(names, volumeNames...)
	}

	if len(errs) > 0 {
		return names, errors.New(strings.Join(errs, "\n"))
	}
	return names, nil
}

func loadVolume(volumeName string, driver BackupStoreDriver) (*Volume, error) {
	v := &Volume{}
	file := getVolumeFilePath(volumeName)
	if err := LoadConfigInBackupStore(file, driver, v); err != nil {
		return nil, err
	}
	return v, nil
}

func saveVolume(v *Volume, driver BackupStoreDriver) error {
	file := getVolumeFilePath(v.Name)
	if err := SaveConfigInBackupStore(file, driver, v); err != nil {
		return err
	}
	return nil
}

func getBackupNamesForVolume(volumeName string, driver BackupStoreDriver) ([]string, error) {
	result := []string{}
	fileList, err := driver.List(getBackupPath(volumeName))
	if err != nil {
		// path doesn't exist
		return result, nil
	}
	return util.ExtractNames(fileList, BACKUP_CONFIG_PREFIX, CFG_SUFFIX), nil
}

func getBackupPath(volumeName string) string {
	return filepath.Join(getVolumePath(volumeName), BACKUP_DIRECTORY) + "/"
}

func getBackupConfigPath(backupName, volumeName string) string {
	path := getBackupPath(volumeName)
	fileName := getBackupConfigName(backupName)
	return filepath.Join(path, fileName)
}

func isBackupInProgress(backup *Backup) bool {
	return backup != nil && backup.CreatedTime == ""
}

func backupExists(backupName, volumeName string, bsDriver BackupStoreDriver) bool {
	return bsDriver.FileExists(getBackupConfigPath(backupName, volumeName))
}

func loadBackup(backupName, volumeName string, bsDriver BackupStoreDriver) (*Backup, error) {
	backup := &Backup{}
	if err := LoadConfigInBackupStore(getBackupConfigPath(backupName, volumeName), bsDriver, backup); err != nil {
		return nil, err
	}
	return backup, nil
}

func saveBackup(backup *Backup, bsDriver BackupStoreDriver) error {
	if backup.VolumeName == "" {
		return fmt.Errorf("missing volume specifier for backup: %v", backup.Name)
	}
	filePath := getBackupConfigPath(backup.Name, backup.VolumeName)
	if err := SaveConfigInBackupStore(filePath, bsDriver, backup); err != nil {
		return err
	}
	return nil
}

func removeBackup(backup *Backup, bsDriver BackupStoreDriver) error {
	filePath := getBackupConfigPath(backup.Name, backup.VolumeName)
	if err := bsDriver.Remove(filePath); err != nil {
		return err
	}
	log.Infof("Removed %v on backupstore", filePath)
	return nil
}
