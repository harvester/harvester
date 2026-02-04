package backupstore

import (
	"fmt"
	"time"
)

type ConfigMetadata struct {
	ModificationTime time.Time
}

func GetConfigMetadata(url string) (*ConfigMetadata, error) {
	driver, err := GetBackupStoreDriver(url)
	if err != nil {
		return nil, err
	}

	backupName, volumeName, _, err := DecodeBackupURL(url)
	if err != nil {
		return nil, err
	}

	var filePath string
	if backupName != "" {
		filePath = getBackupConfigPath(backupName, volumeName)
	} else {
		filePath = getVolumeFilePath(volumeName)
	}

	if !driver.FileExists(filePath) {
		return &ConfigMetadata{}, fmt.Errorf("cannot find %v in backupstore", filePath)
	}

	modificationTime := driver.FileTime(filePath)
	return &ConfigMetadata{ModificationTime: modificationTime}, nil
}
