package backupstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/longhorn/backupstore/util"
	"github.com/sirupsen/logrus"

	. "github.com/longhorn/backupstore/logging"
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

func loadConfigInBackupStore(filePath string, driver BackupStoreDriver, v interface{}) error {
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

func saveConfigInBackupStore(filePath string, driver BackupStoreDriver, v interface{}) error {
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
func getVolumeNames(driver BackupStoreDriver) []string {
	names := []string{}
	volumePathBase := filepath.Join(backupstoreBase, VOLUME_DIRECTORY)
	lv1Dirs, _ := driver.List(volumePathBase)
	for _, lv1 := range lv1Dirs {
		lv1Path := filepath.Join(volumePathBase, lv1)
		lv2Dirs, err := driver.List(lv1Path)
		if err != nil {
			log.Warnf("failed to list second level dirs for path: %v reason: %v", lv1Path, err)
			continue
		}
		for _, lv2 := range lv2Dirs {
			lv2Path := filepath.Join(lv1Path, lv2)
			volumeNames, err := driver.List(lv2Path)
			if err != nil {
				log.Warnf("failed to list volume names for path: %v reason: %v", lv2Path, err)
				continue
			}
			names = append(names, volumeNames...)
		}
	}
	return names
}

func loadVolume(volumeName string, driver BackupStoreDriver) (*Volume, error) {
	v := &Volume{}
	file := getVolumeFilePath(volumeName)
	if err := loadConfigInBackupStore(file, driver, v); err != nil {
		return nil, err
	}
	return v, nil
}

func saveVolume(v *Volume, driver BackupStoreDriver) error {
	file := getVolumeFilePath(v.Name)
	if err := saveConfigInBackupStore(file, driver, v); err != nil {
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
	if err := loadConfigInBackupStore(getBackupConfigPath(backupName, volumeName), bsDriver, backup); err != nil {
		return nil, err
	}
	return backup, nil
}

func saveBackup(backup *Backup, bsDriver BackupStoreDriver) error {
	if backup.VolumeName == "" {
		return fmt.Errorf("missing volume specifier for backup: %v", backup.Name)
	}
	filePath := getBackupConfigPath(backup.Name, backup.VolumeName)
	if err := saveConfigInBackupStore(filePath, bsDriver, backup); err != nil {
		return err
	}
	return nil
}

func removeBackup(backup *Backup, bsDriver BackupStoreDriver) error {
	filePath := getBackupConfigPath(backup.Name, backup.VolumeName)
	if err := bsDriver.Remove(filePath); err != nil {
		return err
	}
	log.Debugf("Removed %v on backupstore", filePath)
	return nil
}
