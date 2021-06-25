package backupstore

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/honestbee/jobq"
	"github.com/sirupsen/logrus"

	. "github.com/longhorn/backupstore/logging"
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

	Messages map[MessageType]string

	Backups map[string]*BackupInfo `json:",omitempty"`

	BackingImageName string
	BackingImageURL  string
}

type BackupInfo struct {
	Name            string
	URL             string
	SnapshotName    string
	SnapshotCreated string
	Created         string
	Size            int64 `json:",string"`
	Labels          map[string]string
	IsIncremental   bool

	VolumeName             string `json:",omitempty"`
	VolumeSize             int64  `json:",string,omitempty"`
	VolumeCreated          string `json:",omitempty"`
	VolumeBackingImageName string `json:",omitempty"`
	VolumeBackingImageURL  string `json:",omitempty"`

	Messages map[MessageType]string
}

func addListVolume(volumeName string, driver BackupStoreDriver, volumeOnly bool) (*VolumeInfo, error) {
	if volumeName == "" {
		return nil, fmt.Errorf("Invalid empty volume Name")
	}

	if !util.ValidateName(volumeName) {
		return nil, fmt.Errorf("Invalid volume name %v", volumeName)
	}

	volume, err := loadVolume(volumeName, driver)
	if err != nil {
		return &VolumeInfo{
			Name:     volumeName,
			Messages: map[MessageType]string{MessageTypeError: err.Error()},
			Backups:  make(map[string]*BackupInfo),
		}, nil
	}

	volumeInfo := fillVolumeInfo(volume)
	if volumeOnly {
		return volumeInfo, nil
	}

	// try to find all backups for this volume
	backupNames, err := getBackupNamesForVolume(volumeName, driver)
	if err != nil {
		volumeInfo.Messages[MessageTypeError] = err.Error()
		return volumeInfo, nil
	}

	for _, backupName := range backupNames {
		var info *BackupInfo
		backup, err := loadBackup(backupName, volumeName, driver)
		if err != nil {
			log.WithFields(logrus.Fields{
				LogFieldReason: LogReasonFallback,
				LogFieldEvent:  LogEventList,
				LogFieldObject: LogObjectBackup,
				LogFieldBackup: backupName,
				LogFieldVolume: volumeName,
			}).Warn("Failed to load backup in backupstore")
			info = failedBackupInfo(backupName, volumeName, driver.GetURL(), err)
		} else if isBackupInProgress(backup) {
			// for now we don't return in progress backups to the ui
			continue
		} else {
			info = fillBackupInfo(backup, driver.GetURL())
		}
		volumeInfo.Backups[info.URL] = info
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
		// init worker pool size to 256
		jobq.WorkerPoolSize(256),
	)
	defer jobQueues.Stop()
	jobQueueTimeout := 30 * time.Second

	volumeNames := []string{volumeName}
	if volumeName == "" {
		volumeNames, err = getVolumeNames(jobQueues, jobQueueTimeout, driver)
		if err != nil {
			return nil, err
		}
	}

	var trackers []jobq.JobTracker
	for _, volumeName := range volumeNames {
		volumeName := volumeName
		tracker := jobQueues.QueueTimedFunc(context.Background(), func(ctx context.Context) (interface{}, error) {
			return addListVolume(volumeName, driver, volumeOnly)
		}, jobQueueTimeout)
		trackers = append(trackers, tracker)
	}

	var (
		resp = make(map[string]*VolumeInfo)
		errs []string
	)
	for _, tracker := range trackers {
		payload, err := tracker.Result()
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		volumeInfo := payload.(*VolumeInfo)
		resp[volumeInfo.Name] = volumeInfo
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "\n"))
	}
	return resp, nil
}

func fillVolumeInfo(volume *Volume) *VolumeInfo {
	return &VolumeInfo{
		Name:             volume.Name,
		Size:             volume.Size,
		Labels:           volume.Labels,
		Created:          volume.CreatedTime,
		LastBackupName:   volume.LastBackupName,
		LastBackupAt:     volume.LastBackupAt,
		DataStored:       int64(volume.BlockCount * DEFAULT_BLOCK_SIZE),
		Messages:         make(map[MessageType]string),
		Backups:          make(map[string]*BackupInfo),
		BackingImageName: volume.BackingImageName,
		BackingImageURL:  volume.BackingImageURL,
	}
}

func failedBackupInfo(backupName string, volumeName string,
	destURL string, err error) *BackupInfo {
	return &BackupInfo{
		Name:       backupName,
		URL:        encodeBackupURL(backupName, volumeName, destURL),
		VolumeName: volumeName,
		Messages:   map[MessageType]string{MessageTypeError: err.Error()},
	}
}

func fillBackupInfo(backup *Backup, destURL string) *BackupInfo {
	return &BackupInfo{
		Name:            backup.Name,
		URL:             encodeBackupURL(backup.Name, backup.VolumeName, destURL),
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
	info.VolumeBackingImageURL = volume.BackingImageURL
	return info
}

func InspectBackup(backupURL string) (*BackupInfo, error) {
	driver, err := GetBackupStoreDriver(backupURL)
	if err != nil {
		return nil, err
	}
	backupName, volumeName, err := decodeBackupURL(backupURL)
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
