package backupstore

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/longhorn/backupstore/util"
)

const (
	LOCKS_DIRECTORY       = "locks"
	LOCK_PREFIX           = "lock"
	LOCK_SUFFIX           = ".lck"
	LOCK_DURATION         = time.Second * 150
	LOCK_REFRESH_INTERVAL = time.Second * 60
	LOCK_CHECK_WAIT_TIME  = time.Second * 2
)

type LockType int

const UNTYPED_LOCK LockType = 0
const BACKUP_LOCK LockType = 1
const RESTORE_LOCK LockType = 1
const DELETION_LOCK LockType = 2

type FileLock struct {
	Name       string
	Type       LockType
	Acquired   bool
	driver     BackupStoreDriver
	volume     string
	count      int32
	serverTime time.Time // UTC time
	keepAlive  chan struct{}
	mutex      sync.Mutex
}

func New(driver BackupStoreDriver, volumeName string, lockType LockType) (*FileLock, error) {
	return &FileLock{driver: driver, volume: volumeName,
		Type: lockType, Name: util.GenerateName(LOCK_PREFIX)}, nil
}

// isExpired checks whether the current lock is expired
func (lock *FileLock) isExpired() bool {
	// server time is always in UTC
	isExpired := time.Now().UTC().Sub(lock.serverTime) > LOCK_DURATION
	return isExpired
}

func (lock *FileLock) String() string {
	return fmt.Sprintf("{ volume: %v, name: %v, type: %v, acquired: %v, serverTime: %v }",
		lock.volume, lock.Name, lock.Type, lock.Acquired, lock.serverTime)
}

func (lock *FileLock) canAcquire() bool {
	canAcquire := true
	locks := getLocksForVolume(lock.volume, lock.driver)
	file := getLockFilePath(lock.volume, lock.Name)
	log.WithField("lock", lock).Infof("Trying to acquire lock %v", file)
	log.Infof("backupstore volume %v contains locks %v", lock.volume, locks)

	for _, serverLock := range locks {
		serverLockHasDifferentType := serverLock.Type != lock.Type
		serverLockHasPriority := compareLocks(serverLock, lock) < 0
		if serverLockHasDifferentType && serverLockHasPriority && !serverLock.isExpired() {
			canAcquire = false
			break
		}
	}

	return canAcquire
}

func (lock *FileLock) Lock() error {
	lock.mutex.Lock()
	defer lock.mutex.Unlock()
	if lock.Acquired {
		atomic.AddInt32(&lock.count, 1)
		_ = saveLock(lock)
		return nil
	}

	// we create first then retrieve all locks
	// because this way if another client creates at the same time
	// one of us will be first in the times array
	// the servers modification time is only the initial lock creation time
	// and we do not need to start lock refreshing till after we acquired the lock
	// since lock expiration is based on the serverTime + LOCK_DURATION
	if err := saveLock(lock); err != nil {
		return err
	}

	// since the node times might not be perfectly in sync and the servers file time has second precision
	// we wait 2 seconds before retrieving the current set of locks, this eliminates a race condition
	// where 2 processes request a lock at the same time
	time.Sleep(LOCK_CHECK_WAIT_TIME)

	// we only try to acquire once, since backup operations generally take a long time
	// there is no point in trying to wait for lock acquisition, better to throw an error
	// and let the calling code retry with an exponential backoff.
	if !lock.canAcquire() {
		file := getLockFilePath(lock.volume, lock.Name)
		_ = removeLock(lock)
		return fmt.Errorf("failed lock %v type %v acquisition", file, lock.Type)
	}

	file := getLockFilePath(lock.volume, lock.Name)
	log.Infof("Acquired lock %v type %v on backupstore", file, lock.Type)
	lock.Acquired = true
	atomic.AddInt32(&lock.count, 1)
	if err := saveLock(lock); err != nil {
		_ = removeLock(lock)
		return errors.Wrapf(err, "failed to store updated lock %v type %v after acquisition", file, lock.Type)
	}

	// enable lock refresh
	lock.keepAlive = make(chan struct{})
	go func() {
		refreshTimer := time.NewTicker(LOCK_REFRESH_INTERVAL)
		defer refreshTimer.Stop()
		for {
			select {
			case <-lock.keepAlive:
				return
			case <-refreshTimer.C:
				lock.mutex.Lock()
				if lock.Acquired {
					if err := saveLock(lock); err != nil {
						// nothing we can do here, that's why the lock acquisition time is 2x lock refresh interval
						log.WithError(err).Warnf("Failed to refresh acquired lock %v type %v", file, lock.Type)
					}
				}
				lock.mutex.Unlock()
			}
		}
	}()

	return nil
}

func (lock *FileLock) Unlock() error {
	lock.mutex.Lock()
	defer lock.mutex.Unlock()
	if atomic.AddInt32(&lock.count, -1) <= 0 {
		lock.Acquired = false
		if lock.keepAlive != nil {
			close(lock.keepAlive)
		}
		if err := removeLock(lock); err != nil {
			return err
		}
		return nil
	}

	return nil
}

func loadLock(volumeName string, name string, driver BackupStoreDriver) (*FileLock, error) {
	lock := &FileLock{}
	file := getLockFilePath(volumeName, name)
	if err := LoadConfigInBackupStore(driver, file, lock); err != nil {
		return nil, err
	}
	lock.serverTime = driver.FileTime(file)
	log.Infof("Loaded lock %v type %v on backupstore", file, lock.Type)
	return lock, nil
}

func removeLock(lock *FileLock) error {
	file := getLockFilePath(lock.volume, lock.Name)
	if err := lock.driver.Remove(file); err != nil {
		return err
	}
	log.Infof("Removed lock %v type %v on backupstore", file, lock.Type)
	return nil
}

func saveLock(lock *FileLock) error {
	file := getLockFilePath(lock.volume, lock.Name)
	if err := SaveConfigInBackupStore(lock.driver, file, lock); err != nil {
		return err
	}
	lock.serverTime = lock.driver.FileTime(file)
	log.Infof("Stored lock %v type %v on backupstore", file, lock.Type)
	return nil
}

// compareLocks compares the locks by Acquired
// then by serverTime (UTC) followed by Name
func compareLocks(a *FileLock, b *FileLock) int {
	if a.Acquired == b.Acquired {
		if a.serverTime.UTC().Equal(b.serverTime.UTC()) {
			return strings.Compare(a.Name, b.Name)
		} else if a.serverTime.UTC().Before(b.serverTime.UTC()) {
			return -1
		} else {
			return 1
		}
	} else if a.Acquired {
		return -1
	} else {
		return 1
	}
}

func getLockNamesForVolume(volumeName string, driver BackupStoreDriver) []string {
	fileList, err := driver.List(getLockPath(volumeName))
	if err != nil {
		// path doesn't exist
		return []string{}
	}
	names := util.ExtractNames(fileList, "", LOCK_SUFFIX)
	return names
}

func getLocksForVolume(volumeName string, driver BackupStoreDriver) []*FileLock {
	names := getLockNamesForVolume(volumeName, driver)
	locks := make([]*FileLock, 0, len(names))
	for _, name := range names {
		lock, err := loadLock(volumeName, name, driver)
		if err != nil {
			file := getLockFilePath(volumeName, name)
			log.WithError(err).Warnf("Failed to load lock %v on backupstore", file)
			continue
		}
		locks = append(locks, lock)
	}

	return locks
}

func getLockPath(volumeName string) string {
	return filepath.Join(getVolumePath(volumeName), LOCKS_DIRECTORY) + "/"
}

func getLockFilePath(volumeName string, name string) string {
	path := getLockPath(volumeName)
	fileName := name + LOCK_SUFFIX
	return filepath.Join(path, fileName)
}
