package backupstore

import (
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"

	. "github.com/longhorn/backupstore/logging"
)

type InitFunc func(destURL string) (BackupStoreDriver, error)

type BackupStoreDriver interface {
	Kind() string
	GetURL() string
	FileExists(filePath string) bool
	FileSize(filePath string) int64
	FileTime(filePath string) time.Time     // Needs to be returned in UTC
	Remove(path string) error               // Behavior like "rm -rf"
	Read(src string) (io.ReadCloser, error) // Caller needs to close
	Write(dst string, rs io.ReadSeeker) error
	List(path string) ([]string, error) // Behavior like "ls", not like "find"
	Upload(src, dst string) error
	Download(src, dst string) error
}

var (
	initializers map[string]InitFunc
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "backupstore"})
)

func GetLog() logrus.FieldLogger {
	return log
}

func generateError(fields logrus.Fields, format string, v ...interface{}) error {
	return ErrorWithFields("backupstore", fields, format, v...)
}

func init() {
	initializers = make(map[string]InitFunc)
}

func RegisterDriver(kind string, initFunc InitFunc) error {
	if _, exists := initializers[kind]; exists {
		return fmt.Errorf("%s has already been registered", kind)
	}
	initializers[kind] = initFunc
	return nil
}

func unregisterDriver(kind string) error {
	if _, exists := initializers[kind]; !exists {
		return fmt.Errorf("%s has not been registered", kind)
	}
	delete(initializers, kind)
	return nil
}

func GetBackupStoreDriver(destURL string) (BackupStoreDriver, error) {
	if destURL == "" {
		return nil, fmt.Errorf("destination URL hasn't been specified")
	}
	u, err := url.Parse(destURL)
	if err != nil {
		return nil, err
	}
	if _, exists := initializers[u.Scheme]; !exists {
		return nil, fmt.Errorf("driver %v is not supported", u.Scheme)
	}
	return initializers[u.Scheme](destURL)
}
