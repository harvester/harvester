package nfs

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/longhorn/backupstore"
	"github.com/longhorn/backupstore/fsops"
	"github.com/longhorn/backupstore/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	log           = logrus.WithFields(logrus.Fields{"pkg": "nfs"})
	MinorVersions = []string{"4.2", "4.1", "4.0"}
)

type BackupStoreDriver struct {
	destURL    string
	serverPath string
	mountDir   string
	*fsops.FileSystemOperator
}

const (
	KIND = "nfs"

	NfsPath  = "nfs.path"
	MountDir = "/var/lib/longhorn-backupstore-mounts"

	MaxCleanupLevel = 10

	UnsupportedProtocolError = "Protocol not supported"
)

func init() {
	if err := backupstore.RegisterDriver(KIND, initFunc); err != nil {
		panic(err)
	}
}

func initFunc(destURL string) (backupstore.BackupStoreDriver, error) {
	b := &BackupStoreDriver{}
	b.FileSystemOperator = fsops.NewFileSystemOperator(b)

	u, err := url.Parse(destURL)
	if err != nil {
		return nil, err
	}

	if u.Scheme != KIND {
		return nil, fmt.Errorf("BUG: Why dispatch %v to %v?", u.Scheme, KIND)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("NFS path must follow: nfs://server:/path/ format")
	}
	if u.Path == "" {
		return nil, fmt.Errorf("Cannot find nfs path")
	}

	b.serverPath = u.Host + u.Path
	b.mountDir = filepath.Join(MountDir, strings.TrimRight(strings.Replace(u.Host, ".", "_", -1), ":"), u.Path)
	if err := os.MkdirAll(b.mountDir, os.ModeDir|0700); err != nil {
		return nil, fmt.Errorf("Cannot create mount directory %v for NFS server: %v", b.mountDir, err)
	}

	if err := b.mount(); err != nil {
		return nil, fmt.Errorf("Cannot mount nfs %v: %v", b.serverPath, err)
	}
	if _, err := b.List(""); err != nil {
		return nil, fmt.Errorf("NFS path %v doesn't exist or is not a directory", b.serverPath)
	}

	b.destURL = KIND + "://" + b.serverPath
	log.Debugf("Loaded driver for %v", b.destURL)
	return b, nil
}

func (b *BackupStoreDriver) mount() (err error) {
	if util.IsMounted(b.mountDir) {
		return nil
	}

	retErr := errors.New("Cannot mount using NFSv4")

	for _, version := range MinorVersions {
		log.Debugf("attempting mount for nfs path %v with nfsvers %v", b.serverPath, version)
		_, err = util.Execute("mount", []string{"-t", "nfs4", "-o", fmt.Sprintf("nfsvers=%v", version), "-o", "actimeo=1", b.serverPath, b.mountDir})
		if err == nil {
			return nil
		}

		retErr = errors.Wrapf(retErr, "vers=%s: %v", version, err.Error())
	}

	return retErr
}

func (b *BackupStoreDriver) Kind() string {
	return KIND
}

func (b *BackupStoreDriver) GetURL() string {
	return b.destURL
}

func (b *BackupStoreDriver) LocalPath(path string) string {
	return filepath.Join(b.mountDir, path)
}
