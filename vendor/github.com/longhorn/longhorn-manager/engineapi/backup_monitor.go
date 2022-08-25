package engineapi

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/clock"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	BackupMonitorSyncPeriod = 2 * time.Second

	// BackupMonitorMaxRetryPeriod is the maximum retry period when backup monitor routine
	// encounters an error and backup stays in Pending state
	BackupMonitorMaxRetryPeriod = 24 * time.Hour
)

type BackupMonitor struct {
	logger logrus.FieldLogger

	backupName     string
	snapshotName   string
	replicaAddress string

	engine            *longhorn.Engine
	engineClientProxy EngineClientProxy

	backupStatus     longhorn.BackupStatus
	backupStatusLock sync.RWMutex

	syncCallback func(key string)
	callbackKey  string

	ctx  context.Context
	quit context.CancelFunc

	retryCount int
}

func NewBackupMonitor(logger logrus.FieldLogger,
	backup *longhorn.Backup, volume *longhorn.Volume, backupTargetClient *BackupTargetClient,
	biChecksum string, engine *longhorn.Engine, engineClientProxy EngineClientProxy,
	syncCallback func(key string)) (*BackupMonitor, error) {
	ctx, quit := context.WithCancel(context.Background())
	m := &BackupMonitor{
		logger: logger.WithFields(logrus.Fields{"backup": backup.Name}),

		backupName:   backup.Name,
		snapshotName: backup.Spec.SnapshotName,

		engine:            engine,
		engineClientProxy: engineClientProxy,

		backupStatusLock: sync.RWMutex{},

		syncCallback: syncCallback,
		callbackKey:  backup.Namespace + "/" + backup.Name,

		ctx:  ctx,
		quit: quit,
	}

	// Call engine API snapshot backup
	if backup.Status.State == longhorn.BackupStateNew {
		_, replicaAddress, err := engineClientProxy.SnapshotBackup(engine,
			backup.Spec.SnapshotName, backup.Name, backupTargetClient.URL,
			volume.Spec.BackingImage, biChecksum,
			backup.Spec.Labels, backupTargetClient.Credential)
		if err != nil {
			if !strings.Contains(err.Error(), "DeadlineExceeded") {
				m.logger.WithError(err).Warn("Cannot take snapshot backup")
				m.Close()
				return nil, err
			}

			// [Workaround]
			// Special handling the RPC call return code DeadlineExceeded, mark it as Pending state.
			// The snapshot backup initialization _probably_ succeeded in the replica sync agent server.
			// Use the backup monitor routine to monitor the backup status stays in Error state or change
			// to InProgress/Completed state with the maximum retry count mechanism.
			// [TODO]
			// Since API engineclient.SnapshotBackup is not idempotent, this controller cannot blindly
			// retry the call when error DeadlineExceeded is triggered.
			// Instead, it has to mark the backup as a kind of special state Pending, then relies on the
			// backup monitor routine periodically checking if the backup creation actually started.
			// After making the API call idempotent and being able to deprecate the old version
			// engine image, we can remove this part.
			// https://github.com/longhorn/longhorn/issues/3545
			m.logger.WithError(err).Warnf("Snapshot backup timeout")
			backup.Status.State = longhorn.BackupStatePending
		}

		m.backupStatus = backup.Status
		m.replicaAddress = replicaAddress
	}

	// Create a goroutine to monitor the replica backup state/progress
	go m.monitorBackups()

	return m, nil
}

func (m *BackupMonitor) monitorBackups() {
	// If backup.status.state = Pending, use exponential backoff timer to monitor engine/replica backup status
	// Otherwise, use liner timer to monitor engine/replica backup status
	useLinerTimer := true
	if m.backupStatus.State == longhorn.BackupStatePending {
		useLinerTimer = m.exponentialBackOffTimer()
	}
	if useLinerTimer {
		m.linerTimer()
	}
}

// linerTimer runs a periodically liner timer to sync backup status from engine/replica
func (m *BackupMonitor) linerTimer() {
	wait.PollUntil(BackupMonitorSyncPeriod, func() (done bool, err error) {
		currentBackupStatus, err := m.syncBackupStatusFromEngineReplica()
		if err != nil {
			currentBackupStatus.State = longhorn.BackupStateError
			currentBackupStatus.Error = err.Error()
		}
		m.syncCallBack(currentBackupStatus)
		return false, nil
	}, m.ctx.Done())
}

// exponentialBackOffTimer runs a exponential backoff timer to sync backup status from engine/replica
func (m *BackupMonitor) exponentialBackOffTimer() bool {
	var (
		initBackoff   = BackupMonitorSyncPeriod
		maxBackoff    = 10 * time.Minute
		resetDuration = BackupMonitorMaxRetryPeriod
		backoffFactor = 2.0
		jitter        = 0.0
		clock         = clock.RealClock{}
		retryCount    = 0
	)
	// The exponential backoff timer 2s/4s/8s/16s/.../10mins
	backoffMgr := wait.NewExponentialBackoffManager(initBackoff, maxBackoff, resetDuration, backoffFactor, jitter, clock)
	defer backoffMgr.Backoff().Stop()

	ctx, cancel := context.WithTimeout(context.Background(), BackupMonitorMaxRetryPeriod)
	defer cancel()

	m.logger.Info("Exponential backoff timer")

	for {
		select {
		case <-backoffMgr.Backoff().C():
			retryCount++
			m.logger.Debugf("Within exponential backoff timer retry count %d", retryCount)
			_, err := m.syncBackupStatusFromEngineReplica()
			if err == nil {
				m.logger.Info("Change to liner timer to monitor it")
				// Change to liner timer to monitor it
				return true
			}
			// Keep in exponential backoff timer
		case <-ctx.Done():
			// Give it the last try to prevent if the snapshot backup succeed between
			// the last trigged backoff time and the max retry period
			currentBackupStatus, err := m.syncBackupStatusFromEngineReplica()
			if err == nil {
				m.logger.Info("Change to liner timer to monitor it")
				// Change to liner timer to monitor it
				return true
			}

			// Set to Error state
			err = fmt.Errorf("Max retry period %s reached in exponential backoff timer", BackupMonitorMaxRetryPeriod)
			m.logger.Error(err)

			currentBackupStatus.State = longhorn.BackupStateError
			currentBackupStatus.Error = err.Error()
			m.syncCallBack(currentBackupStatus)
			// Let the backup controller closes this backup monitor routine
		case <-m.ctx.Done():
			m.logger.Info("Close backup monitor routine")
			return false
		}
	}
}

func (m *BackupMonitor) syncCallBack(currentBackupStatus longhorn.BackupStatus) {
	// new information, request a resync for this backup
	m.backupStatusLock.Lock()
	defer m.backupStatusLock.Unlock()
	if !reflect.DeepEqual(m.backupStatus, currentBackupStatus) {
		m.backupStatus = currentBackupStatus
		m.syncCallback(m.callbackKey)
	}
}

func (m *BackupMonitor) syncBackupStatusFromEngineReplica() (longhorn.BackupStatus, error) {
	currentBackupStatus := longhorn.BackupStatus{}

	m.backupStatusLock.RLock()
	m.backupStatus.DeepCopyInto(&currentBackupStatus)
	m.backupStatusLock.RUnlock()

	engineBackupStatus, err := m.engineClientProxy.SnapshotBackupStatus(m.engine, m.backupName, m.replicaAddress)
	if err != nil {
		return currentBackupStatus, err
	}
	if engineBackupStatus == nil {
		err = fmt.Errorf("cannot find backup %s status in longhorn engine", m.backupName)
		return currentBackupStatus, err
	}
	if engineBackupStatus.SnapshotName != m.snapshotName {
		err = fmt.Errorf("cannot find matched snapshot %s/%s of backup %s status in longhorn engine", engineBackupStatus.SnapshotName, m.snapshotName, m.backupName)
		return currentBackupStatus, err
	}

	currentBackupStatus.Progress = engineBackupStatus.Progress
	currentBackupStatus.URL = engineBackupStatus.BackupURL
	currentBackupStatus.Error = engineBackupStatus.Error
	currentBackupStatus.SnapshotName = engineBackupStatus.SnapshotName
	currentBackupStatus.State = ConvertEngineBackupState(engineBackupStatus.State)
	currentBackupStatus.ReplicaAddress = engineBackupStatus.ReplicaAddress
	return currentBackupStatus, nil
}

func (m *BackupMonitor) GetBackupStatus() longhorn.BackupStatus {
	m.backupStatusLock.RLock()
	defer m.backupStatusLock.RUnlock()
	return m.backupStatus
}

func (m *BackupMonitor) Close() {
	m.engineClientProxy.Close()
	m.quit()
}
