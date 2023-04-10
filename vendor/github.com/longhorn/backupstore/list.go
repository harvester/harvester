package backupstore

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/honestbee/jobq"

	"github.com/longhorn/backupstore/util"
)

const jobQueueTimeout = time.Minute

type VolumeInfo struct {
	Name           string
	Size           int64 `json:",string"`
	Labels         map[string]string
	Created        string
	LastBackupName string
	LastBackupAt   string
	DataStored     int64 `json:",string"`

	Messages map[MessageType]string

	Backups map[string]*BackupInfo `json:",omitempty"`

	BackingImageName     string
	BackingImageChecksum string
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

	Messages map[MessageType]string
}

func addListVolume(driver BackupStoreDriver, volumeName string, volumeOnly bool) (*VolumeInfo, error) {
	if volumeName == "" {
		return nil, fmt.Errorf("invalid empty volume Name")
	}

	if !util.ValidateName(volumeName) {
		return nil, fmt.Errorf("invalid volume name %v", volumeName)
	}

	volumeInfo := &VolumeInfo{Messages: make(map[MessageType]string)}
	if !volumeExists(driver, volumeName) {
		// If the backup volume folder exist but volume.cfg not exist
		// save the error in Messages field
		volumeInfo.Messages[MessageTypeError] = fmt.Sprintf("cannot find %v in backupstore", getVolumeFilePath(volumeName))
		return volumeInfo, nil
	}

	if volumeOnly {
		return volumeInfo, nil
	}

	// try to find all backups for this volume
	backupNames, err := getBackupNamesForVolume(driver, volumeName)
	if err != nil {
		volumeInfo.Messages[MessageTypeError] = err.Error()
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

	jobQueues := jobq.NewWorkerDispatcher(
		// init #cpu*16 workers
		jobq.WorkerN(runtime.NumCPU()*16),
		// init worker pool size to 256 (same as max folders 16*16)
		jobq.WorkerPoolSize(256),
	)
	defer jobQueues.Stop()

	var resp = make(map[string]*VolumeInfo)
	volumeNames := []string{volumeName}
	if volumeName == "" {
		volumeNames, err = getVolumeNames(jobQueues, jobQueueTimeout, driver)
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
