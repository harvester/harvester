package nfs

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	mount "k8s.io/mount-utils"

	"github.com/longhorn/backupstore"
	"github.com/longhorn/backupstore/fsops"
	"github.com/longhorn/backupstore/util"
)

var (
	log           = logrus.WithFields(logrus.Fields{"pkg": "nfs"})
	MinorVersions = []string{"4.2", "4.1", "4.0"}

	// Ref: https://github.com/longhorn/backupstore/pull/91
	defaultMountInterval = 1 * time.Second
	defaultMountTimeout  = 5 * time.Second
)

type BackupStoreDriver struct {
	destURL    string
	serverPath string
	mountDir   string
	*fsops.FileSystemOperator
}

const (
	KIND = "nfs"

	NfsPath = "nfs.path"

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
		return nil, fmt.Errorf("NFS path must follow format: nfs://<server-address>:/<share-name>/")
	}
	if u.Path == "" {
		return nil, fmt.Errorf("cannot find nfs path")
	}

	b.serverPath = u.Host + u.Path
	b.destURL = KIND + "://" + b.serverPath
	b.mountDir = filepath.Join(util.MountDir, strings.TrimRight(strings.Replace(u.Host, ".", "_", -1), ":"), u.Path)

	if err := b.mount(); err != nil {
		return nil, errors.Wrapf(err, "cannot mount nfs %v", b.serverPath)
	}
	if _, err := b.List(""); err != nil {
		return nil, fmt.Errorf("NFS path %v doesn't exist or is not a directory", b.serverPath)
	}

	log.Infof("Loaded driver for %v", b.destURL)

	return b, nil
}

func (b *BackupStoreDriver) mount() error {
	mounter := mount.New("")

	mounted, err := util.EnsureMountPoint(KIND, b.mountDir, mounter, log)
	if err != nil {
		return err
	}
	if mounted {
		return nil
	}

	retErr := errors.New("cannot mount using NFSv4")

	for _, version := range MinorVersions {
		log.Infof("Attempting mount for nfs path %v with nfsvers %v", b.serverPath, version)

		mountOptions := []string{
			fmt.Sprintf("nfsvers=%v", version),
			"actimeo=1",
			"soft",
			"timeo=300",
			"retry=2",
		}
		sensitiveMountOptions := []string{}

		log.Infof("Mounting NFS share %v on mount point %v with options %+v", b.destURL, b.mountDir, mountOptions)

		err := util.MountWithTimeout(mounter, b.serverPath, b.mountDir, "nfs4", mountOptions, sensitiveMountOptions,
			defaultMountInterval, defaultMountTimeout)
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
