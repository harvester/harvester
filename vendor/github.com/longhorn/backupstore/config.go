package backupstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/slok/goresilience/timeout"

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

	taskTimeout = 90 * time.Second
)

func getBackupConfigName(id string) string {
	return BACKUP_CONFIG_PREFIX + id + CFG_SUFFIX
}

func LoadConfigInBackupStore(driver BackupStoreDriver, filePath string, v interface{}) error {
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
	}).Info("Loading config in backupstore")

	if err := json.NewDecoder(rc).Decode(v); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonComplete,
		LogFieldObject:   LogObjectConfig,
		LogFieldKind:     driver.Kind(),
		LogFieldFilepath: filePath,
	}).Info("Loaded config in backupstore")
	return nil
}

func SaveConfigInBackupStore(driver BackupStoreDriver, filePath string, v interface{}) error {
	j, err := json.Marshal(v)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonStart,
		LogFieldObject:   LogObjectConfig,
		LogFieldKind:     driver.Kind(),
		LogFieldFilepath: filePath,
	}).Info("Saving config in backupstore")

	if err := driver.Write(filePath, bytes.NewReader(j)); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonComplete,
		LogFieldObject:   LogObjectConfig,
		LogFieldKind:     driver.Kind(),
		LogFieldFilepath: filePath,
	}).Info("Saved config in backupstore")
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

func SaveBackupStoreToLocalFile(driver BackupStoreDriver, backupStoreFileURL, localFilePath string) error {
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

func volumeExists(driver BackupStoreDriver, volumeName string) bool {
	return driver.FileExists(getVolumeFilePath(volumeName))
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
func getVolumeNames(jobQueues *workerpool.WorkerPool, driver BackupStoreDriver) ([]string, error) {
	names := []string{}
	volumePathBase := filepath.Join(backupstoreBase, VOLUME_DIRECTORY)
	lv1Dirs, err := driver.List(volumePathBase)
	if err != nil {
		log.WithError(err).Warnf("Failed to list first level dirs for path %v", volumePathBase)
		return names, err
	}

	var errs []string
	lv1Trackers := make(chan JobResult)
	lv2Trackers := make(chan JobResult)
	defer close(lv1Trackers)
	defer close(lv2Trackers)

	runner := timeout.New(timeout.Config{
		Timeout: taskTimeout,
	})

	for _, lv1Dir := range lv1Dirs {
		path := filepath.Join(volumePathBase, lv1Dir)
		jobQueues.Submit(func() {
			lv2Paths := make([]string, 0)
			err := runner.Run(context.TODO(), func(_ context.Context) error {
				lv2Dirs, err := driver.List(path)
				if err != nil {
					logrus.WithError(err).Warnf("Failed to list second level dirs for path %v", path)
					return errors.Wrapf(err, "failed to list second level dirs for path %v", path)
				}
				for _, lv2Dir := range lv2Dirs {
					lv2Paths = append(lv2Paths, filepath.Join(path, lv2Dir))
				}
				return nil
			})
			if err != nil {
				lv1Trackers <- JobResult{nil, err}
				return
			}
			lv1Trackers <- JobResult{lv2Paths, nil}
			return
		})
	}

	lv2PathsNum := 0
	for i := 0; i < len(lv1Dirs); i++ {
		lv1Tracker := <-lv1Trackers
		payload, err := lv1Tracker.payload, lv1Tracker.err
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}

		lv2Paths := payload.([]string)
		lv2PathsNum += len(lv2Paths)
		for _, lv2Path := range lv2Paths {
			path := lv2Path
			jobQueues.Submit(func() {
				var volumeNames []string
				err := runner.Run(context.TODO(), func(_ context.Context) error {
					volumeNames, err = driver.List(path)
					if err != nil {
						logrus.WithError(err).Warnf("Failed to list volume names for path %v", path)
						return errors.Wrapf(err, "failed to list second level dirs for path %v", path)
					}
					return nil
				})
				if err != nil {
					lv2Trackers <- JobResult{nil, err}
					return
				}
				lv2Trackers <- JobResult{volumeNames, nil}
				return
			})
		}
	}

	for i := 0; i < lv2PathsNum; i++ {
		lv2Tracker := <-lv2Trackers
		payload, err := lv2Tracker.payload, lv2Tracker.err
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

func loadVolume(driver BackupStoreDriver, volumeName string) (*Volume, error) {
	v := &Volume{}
	file := getVolumeFilePath(volumeName)
	if err := LoadConfigInBackupStore(driver, file, v); err != nil {
		return nil, err
	}
	// Backward compatibility
	if v.CompressionMethod == "" {
		log.Infof("Falling back compression method to %v for volume %v", LEGACY_COMPRESSION_METHOD, v.Name)
		v.CompressionMethod = LEGACY_COMPRESSION_METHOD
	}
	return v, nil
}

func saveVolume(driver BackupStoreDriver, v *Volume) error {
	return SaveConfigInBackupStore(driver, getVolumeFilePath(v.Name), v)
}

func getBackupNamesForVolume(driver BackupStoreDriver, volumeName string) ([]string, error) {
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

func loadBackup(bsDriver BackupStoreDriver, backupName, volumeName string) (*Backup, error) {
	backup := &Backup{}
	if err := LoadConfigInBackupStore(bsDriver, getBackupConfigPath(backupName, volumeName), backup); err != nil {
		return nil, err
	}
	// Backward compatibility
	if backup.CompressionMethod == "" {
		log.Infof("Fall back compression method to %v for backup %v", LEGACY_COMPRESSION_METHOD, backup.Name)
		backup.CompressionMethod = LEGACY_COMPRESSION_METHOD
	}
	return backup, nil
}

func saveBackup(bsDriver BackupStoreDriver, backup *Backup) error {
	if backup.VolumeName == "" {
		return fmt.Errorf("missing volume specifier for backup: %v", backup.Name)
	}
	filePath := getBackupConfigPath(backup.Name, backup.VolumeName)
	return SaveConfigInBackupStore(bsDriver, filePath, backup)
}

func removeBackup(backup *Backup, bsDriver BackupStoreDriver) error {
	filePath := getBackupConfigPath(backup.Name, backup.VolumeName)
	if err := bsDriver.Remove(filePath); err != nil {
		return err
	}
	log.Infof("Removed %v on backupstore", filePath)
	return nil
}
