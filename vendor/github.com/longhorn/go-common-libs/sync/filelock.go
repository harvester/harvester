package sync

import (
	"os"
	"syscall"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// LockFile is responsible for locking a file and returning a file handle.
func LockFile(filePath string) (file *os.File, err error) {
	log := logrus.WithField("file", filePath)

	defer func() {
		err = errors.Wrapf(err, "failed to lock file %s", filePath)
		if err != nil && file != nil {
			e := file.Close()
			if e != nil {
				err = errors.Wrapf(err, "failed to close file: %v", e)
			}
		}
	}()

	log.Trace("Locking file")

	file, err = os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return file, syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
}

// UnlockFile is responsible for unlocking a file and closing the file handle.
func UnlockFile(file *os.File) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to unlock file %s", file.Name())
	}()

	log := logrus.WithField("file", file.Name())

	err = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	if err != nil {
		log.WithError(err).Warn("Failed to unlock file")
	}

	log.Trace("Unlocked file")
	return file.Close()
}
