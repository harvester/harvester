package engineapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/clock"

	lhbackup "github.com/longhorn/go-common-libs/backup"

	"github.com/longhorn/backupstore/backupbackingimage"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

// parseBackupBackingImageConfig parses a backup backing image config
func parseBackupBackingImageConfig(output string) (*backupbackingimage.BackupInfo, error) {
	backupBackingImageInfo := &backupbackingimage.BackupInfo{}
	if err := json.Unmarshal([]byte(output), backupBackingImageInfo); err != nil {
		return nil, errors.Wrapf(err, "failed to parse config of backup backing image: \n%s", output)
	}
	return backupBackingImageInfo, nil
}

// parseBackupBackingImageNamesList parses a list of backup backing image names into a sorted string slice
func parseBackupBackingImageNamesList(output string) ([]string, error) {
	backingImageNames := []string{}
	if err := json.Unmarshal([]byte(output), &backingImageNames); err != nil {
		return nil, errors.Wrapf(err, "failed to parse backup backing image names: \n%s", output)
	}

	sort.Strings(backingImageNames)
	return backingImageNames, nil
}

// BackupBackingImageGet inspects a backup config with the given backup config URL
func (btc *BackupTargetClient) BackupBackingImageGet(backupBackingImageURL string) (*backupbackingimage.BackupInfo, error) {
	output, err := btc.ExecuteEngineBinary("backup", "inspect-backing-image", backupBackingImageURL)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to get backup backing image config %s", backupBackingImageURL)
	}
	return parseBackupBackingImageConfig(output)
}

// BackupBackingImageNameList returns a list of backup backing image names
func (btc *BackupTargetClient) BackupBackingImageNameList() ([]string, error) {
	output, err := btc.ExecuteEngineBinary("backup", "ls-backing-image", btc.URL)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to list backup backing image names")
	}
	return parseBackupBackingImageNamesList(output)
}

// BackupBackingImageDelete deletes the backup volume from the remote backup target
func (btc *BackupTargetClient) BackupBackingImageDelete(backupURL string) error {
	_, err := btc.ExecuteEngineBinaryWithoutTimeout("backup", "rm-backing-image", backupURL)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to delete backup backing image")
	}
	return nil
}

type BackupBackingImageMonitor struct {
	logger logrus.FieldLogger

	backupBackingImageName string
	client                 *BackingImageManagerClient

	backupBackingImageStatus longhorn.BackupBackingImageStatus
	backingImageName         string
	backupStatusLock         sync.RWMutex

	syncCallback func(key string)
	callbackKey  string

	ctx  context.Context
	quit context.CancelFunc
}

func NewBackupBackingImageMonitor(logger logrus.FieldLogger, ds *datastore.DataStore, bbi *longhorn.BackupBackingImage, backingImage *longhorn.BackingImage, backupTargetClient *BackupTargetClient,
	compressionMethod longhorn.BackupCompressionMethod, concurrentLimit int, bimClient *BackingImageManagerClient, syncCallback func(key string)) (*BackupBackingImageMonitor, error) {
	ctx, quit := context.WithCancel(context.Background())
	biName := backingImage.Name
	m := &BackupBackingImageMonitor{
		logger: logger.WithFields(logrus.Fields{"backupBackingImage": bbi.Name}),

		backupBackingImageName: bbi.Name,
		backingImageName:       biName,
		client:                 bimClient,

		backupStatusLock: sync.RWMutex{},

		syncCallback: syncCallback,
		callbackKey:  bbi.Namespace + "/" + bbi.Name,

		ctx:  ctx,
		quit: quit,
	}

	backupBackingImageParameters := getBackupBackingImageParameters(backingImage)

	// Call backing image manager API snapshot backup
	if bbi.Status.State == longhorn.BackupStateNew {
		err := m.client.BackupCreate(biName, backingImage.Status.UUID, bbi.Status.Checksum,
			backupTargetClient.URL, bbi.Spec.Labels, backupTargetClient.Credential, string(compressionMethod), concurrentLimit, backupBackingImageParameters)
		if err != nil {
			if !strings.Contains(err.Error(), "DeadlineExceeded") {
				m.logger.WithError(err).Warn("failed to take backing image backup")
				return nil, err
			}

			m.logger.WithError(err).Warnf("backup backing image request timeout")
			bbi.Status.State = longhorn.BackupStatePending
		}

		m.backupBackingImageStatus = bbi.Status
	}

	go m.monitorBackupBackingImage()

	return m, nil
}

func (m *BackupBackingImageMonitor) monitorBackupBackingImage() {
	useLinerTimer := true
	if m.backupBackingImageStatus.State == longhorn.BackupStatePending {
		useLinerTimer = m.exponentialBackOffTimer()
	}
	if useLinerTimer {
		m.linerTimer()
	}
}

func (m *BackupBackingImageMonitor) linerTimer() {
	wait.PollUntilContextCancel(m.ctx, BackupMonitorSyncPeriod, true, func(context.Context) (done bool, err error) { //nolint:errcheck
		currentBackupStatus, err := m.syncBackupStatusFromBackingImageManager()
		if err != nil {
			currentBackupStatus.State = longhorn.BackupStateError
			currentBackupStatus.Error = err.Error()
		}
		m.syncCallBack(currentBackupStatus)
		return false, nil
	})
}

func (m *BackupBackingImageMonitor) exponentialBackOffTimer() bool {
	var (
		initBackoff   = BackupMonitorSyncPeriod
		maxBackoff    = 10 * time.Minute
		resetDuration = BackupMonitorMaxRetryPeriod
		backoffFactor = 2.0
		jitter        = 0.0
		clock         = clock.RealClock{}
		retryCount    = 0
	)

	delayFn := wait.Backoff{
		Duration: initBackoff,
		Cap:      maxBackoff,
		Steps:    int(math.Ceil(float64(maxBackoff) / float64(initBackoff))), // now a required argument
		Factor:   backoffFactor,
		Jitter:   jitter,
	}.DelayWithReset(clock, resetDuration)
	defer delayFn.Timer(clock).Stop()

	ctx, cancel := context.WithTimeout(context.Background(), BackupMonitorMaxRetryPeriod)
	defer cancel()

	m.logger.Info("Exponential backoff timer")

	for {
		select {
		case <-delayFn.Timer(clock).C():
			retryCount++
			m.logger.Debugf("Within exponential backoff timer retry count %d", retryCount)
			_, err := m.syncBackupStatusFromBackingImageManager()
			if err == nil {
				m.logger.Info("Change to liner timer to monitor it")
				return true
			}
		case <-ctx.Done():
			currentBackupStatus, err := m.syncBackupStatusFromBackingImageManager()
			if err == nil {
				m.logger.Info("Change to liner timer to monitor it")
				return true
			}

			err = fmt.Errorf("max retry period %s reached in exponential backoff timer", BackupMonitorMaxRetryPeriod)
			m.logger.Error(err)

			currentBackupStatus.State = longhorn.BackupStateError
			currentBackupStatus.Error = err.Error()
			m.syncCallBack(currentBackupStatus)
		case <-m.ctx.Done():
			m.logger.Info("Close backup monitor routine")
			return false
		}
	}
}

func (m *BackupBackingImageMonitor) syncCallBack(currentBackupStatus longhorn.BackupBackingImageStatus) {
	m.backupStatusLock.Lock()
	defer m.backupStatusLock.Unlock()
	if !reflect.DeepEqual(m.backupBackingImageStatus, currentBackupStatus) {
		m.backupBackingImageStatus = currentBackupStatus
		m.syncCallback(m.callbackKey)
	}
}

func (m *BackupBackingImageMonitor) syncBackupStatusFromBackingImageManager() (currentBackupStatus longhorn.BackupBackingImageStatus, err error) {
	currentBackupStatus = longhorn.BackupBackingImageStatus{}
	var backupBackingImageStatus *longhorn.BackupBackingImageStatus

	defer func() {
		if err == nil && backupBackingImageStatus != nil {
			currentBackupStatus.Progress = backupBackingImageStatus.Progress
			currentBackupStatus.URL = backupBackingImageStatus.URL
			currentBackupStatus.Error = backupBackingImageStatus.Error
			currentBackupStatus.State = backupBackingImageStatus.State
		}
	}()

	m.backupStatusLock.RLock()
	m.backupBackingImageStatus.DeepCopyInto(&currentBackupStatus)
	m.backupStatusLock.RUnlock()

	// Use backing image name instead of the backup backing image name because the backup backing image name is a random name after the feature `multiple backup target``.
	backupBackingImageStatus, err = m.client.BackupStatus(m.backingImageName)
	if err != nil {
		return currentBackupStatus, err
	}
	if backupBackingImageStatus == nil {
		err = fmt.Errorf("failed to find backup backing image %s status in backing image manager", m.backupBackingImageName)
		return currentBackupStatus, err
	}

	return currentBackupStatus, nil
}

func (m *BackupBackingImageMonitor) GetBackupBackingImageStatus() longhorn.BackupBackingImageStatus {
	m.backupStatusLock.RLock()
	defer m.backupStatusLock.RUnlock()
	return m.backupBackingImageStatus
}

func (m *BackupBackingImageMonitor) Stop() {
	m.quit()
}

func getBackupBackingImageParameters(backingImage *longhorn.BackingImage) map[string]string {
	parameters := map[string]string{}
	parameters[lhbackup.LonghornBackupBackingImageParameterSecret] = string(backingImage.Spec.Secret)
	parameters[lhbackup.LonghornBackupBackingImageParameterSecretNamespace] = string(backingImage.Spec.SecretNamespace)
	return parameters
}
