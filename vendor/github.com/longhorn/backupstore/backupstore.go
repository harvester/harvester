package backupstore

import (
	"fmt"
	"net/url"

	"github.com/longhorn/backupstore/util"
)

type Volume struct {
	Name                 string
	Size                 int64 `json:",string"`
	Labels               map[string]string
	CreatedTime          string
	LastBackupName       string
	LastBackupAt         string
	BlockCount           int64  `json:",string"`
	BackingImageName     string `json:",string"`
	BackingImageChecksum string `json:",string"`
}

type Snapshot struct {
	Name        string
	CreatedTime string
}

type Backup struct {
	Name              string
	VolumeName        string
	SnapshotName      string
	SnapshotCreatedAt string
	CreatedTime       string
	Size              int64 `json:",string"`
	Labels            map[string]string
	IsIncremental     bool

	Blocks     []BlockMapping `json:",omitempty"`
	SingleFile BackupFile     `json:",omitempty"`
}

var (
	backupstoreBase = "backupstore"
)

func SetBackupstoreBase(base string) {
	backupstoreBase = base
}

func GetBackupstoreBase() string {
	return backupstoreBase
}

func addVolume(volume *Volume, driver BackupStoreDriver) error {
	if volumeExists(volume.Name, driver) {
		return nil
	}

	if !util.ValidateName(volume.Name) {
		return fmt.Errorf("Invalid volume name %v", volume.Name)
	}

	if err := saveVolume(volume, driver); err != nil {
		log.Error("Fail add volume ", volume.Name)
		return err
	}
	log.Debug("Added backupstore volume ", volume.Name)

	return nil
}

func removeVolume(volumeName string, driver BackupStoreDriver) error {
	if !util.ValidateName(volumeName) {
		return fmt.Errorf("Invalid volume name %v", volumeName)
	}

	volumeDir := getVolumePath(volumeName)
	volumeBlocksDirectory := getBlockPath(volumeName)
	volumeBackupsDirectory := getBackupPath(volumeName)
	volumeLocksDirectory := getLockPath(volumeName)
	if err := driver.Remove(volumeBackupsDirectory); err != nil {
		return fmt.Errorf("failed to remove all the backups for volume %v: %v", volumeName, err)
	}
	if err := driver.Remove(volumeBlocksDirectory); err != nil {
		return fmt.Errorf("failed to remove all the blocks for volume %v: %v", volumeName, err)
	}
	if err := driver.Remove(volumeLocksDirectory); err != nil {
		return fmt.Errorf("failed to remove all the locks for volume %v: %v", volumeName, err)
	}
	if err := driver.Remove(volumeDir); err != nil {
		return fmt.Errorf("failed to remove backup volume %v directory in backupstore: %v", volumeName, err)
	}

	log.Debug("Removed volume directory in backupstore: ", volumeDir)
	log.Debug("Removed backupstore volume ", volumeName)

	return nil
}

func EncodeBackupURL(backupName, volumeName, destURL string) string {
	v := url.Values{}
	v.Add("volume", volumeName)
	if backupName != "" {
		v.Add("backup", backupName)
	}
	return destURL + "?" + v.Encode()
}

func DecodeBackupURL(backupURL string) (string, string, string, error) {
	u, err := url.Parse(backupURL)
	if err != nil {
		return "", "", "", err
	}
	v := u.Query()
	volumeName := v.Get("volume")
	backupName := v.Get("backup")
	if !util.ValidateName(volumeName) {
		return "", "", "", fmt.Errorf("Invalid volume name parsed, got %v", volumeName)
	}
	if backupName != "" && !util.ValidateName(backupName) {
		return "", "", "", fmt.Errorf("Invalid backup name parsed, got %v", backupName)
	}
	u.RawQuery = ""
	destURL := u.String()
	return backupName, volumeName, destURL, nil
}

func LoadVolume(backupURL string) (*Volume, error) {
	_, volumeName, _, err := DecodeBackupURL(backupURL)
	if err != nil {
		return nil, err
	}
	driver, err := GetBackupStoreDriver(backupURL)
	if err != nil {
		return nil, err
	}
	return loadVolume(volumeName, driver)
}
