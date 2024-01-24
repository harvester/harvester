package backupstore

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/gammazero/workerpool"

	"github.com/longhorn/backupstore/types"
	"github.com/longhorn/backupstore/util"
)

type VolumeInfo struct {
	Name           string
	Size           int64 `json:",string"`
	Labels         map[string]string
	Created        string
	LastBackupName string
	LastBackupAt   string
	DataStored     int64 `json:",string"`

	Messages map[types.MessageType]string

	Backups map[string]*BackupInfo `json:",omitempty"`

	BackingImageName     string
	BackingImageChecksum string
	StorageClassname     string
	DataEngine           string
}

type BackupInfo struct {
	Name              string
	URL               string
	SnapshotName      string
	SnapshotCreated   string
	Created           string
	Size              int64 `json:",string"`
	Labels            map[string]string
	IsIncremental     bool
	CompressionMethod string `json:",omitempty"`

	VolumeName             string `json:",omitempty"`
	VolumeSize             int64  `json:",string,omitempty"`
	VolumeCreated          string `json:",omitempty"`
	VolumeBackingImageName string `json:",omitempty"`

	Messages map[types.MessageType]string
}

func addListVolume(driver BackupStoreDriver, volumeName string, volumeOnly bool) (*VolumeInfo, error) {
	if volumeName == "" {
		return nil, fmt.Errorf("invalid empty volume Name")
	}

	if !util.ValidateName(volumeName) {
		return nil, fmt.Errorf("invalid volume name %v", volumeName)
	}

	volumeInfo := &VolumeInfo{Messages: make(map[types.MessageType]string)}
	if !volumeExists(driver, volumeName) {
		// If the backup volume folder exist but volume.cfg not exist
		// save the error in Messages field
		volumeInfo.Messages[types.MessageTypeError] = fmt.Sprintf("cannot find %v in backupstore", getVolumeFilePath(volumeName))
		return volumeInfo, nil
	}

	if volumeOnly {
		return volumeInfo, nil
	}

	// try to find all backups for this volume
	backupNames, err := getBackupNamesForVolume(driver, volumeName)
	if err != nil {
		volumeInfo.Messages[types.MessageTypeError] = err.Error()
		return volumeInfo, nil
	}
	volumeInfo.Backups = make(map[string]*BackupInfo)
	for _, backupName := range backupNames {
		volumeInfo.Backups[backupName] = &BackupInfo{}
	}
	return volumeInfo, nil
}

func List(volumeName, destURL string, volumeOnly bool) (map[string]*VolumeInfo, error) {
	driver, err := GetBackupStoreDriver(destURL)
	if err != nil {
		return nil, err
	}

	jobQueues := workerpool.New(runtime.NumCPU() * 16)
	defer jobQueues.StopWait()

	var resp = make(map[string]*VolumeInfo)
	volumeNames := []string{volumeName}
	if volumeName == "" {
		volumeNames, err = getVolumeNames(jobQueues, driver)
		if err != nil {
			return nil, err
		}
	}

	var errs []string
	for _, volumeName := range volumeNames {
		volumeInfo, err := addListVolume(driver, volumeName, volumeOnly)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		resp[volumeName] = volumeInfo
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "\n"))
	}
	return resp, nil
}
