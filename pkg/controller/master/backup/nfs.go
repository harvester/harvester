package backup

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/longhorn/backupstore/fsops"
	"github.com/longhorn/backupstore/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// nfs class help to validate the nfs server
var (
	MinorVersions = []string{"4.2", "4.1", "4.0"}
)

type StoreDriver struct {
	destURL    string
	serverPath string
	mountDir   string
	*fsops.FileSystemOperator
}

const (
	KIND                     = "nfs"
	MountDir                 = "/var/lib/rancher/harvester-backupstore-mounts"
	UnsupportedProtocolError = "Protocol not supported"
)

func (b *StoreDriver) mount() (err error) {
	defer func() {
		if err != nil {
			if _, err := util.Execute("mount", []string{"-t", "nfs", "-o", "nfsvers=3", "-o", "nolock", b.serverPath, b.mountDir}); err != nil {
				return
			}
			err = errors.Wrapf(err, "nfsv4 mount failed but nfsv3 mount succeeded, may be due to server only supporting nfsv3")
			if err := b.unmount(); err != nil {
				logrus.Errorf("failed to clean up nfsv3 test mount: %v", err)
			}
		}
	}()

	if !util.IsMounted(b.mountDir) {
		for _, version := range MinorVersions {
			logrus.Debugf("attempting mount for nfs path %v with nfsvers %v", b.serverPath, version)
			_, err = util.Execute("mount", []string{"-t", "nfs4", "-o", fmt.Sprintf("nfsvers=%v", version), b.serverPath, b.mountDir})
			if err == nil || !strings.Contains(err.Error(), UnsupportedProtocolError) {
				break
			}
		}
	}
	return err
}

func (b *StoreDriver) unmount() error {
	var err error
	if util.IsMounted(b.mountDir) {
		_, err = util.Execute("umount", []string{b.mountDir})
	}
	return err
}

func (b *StoreDriver) Kind() string {
	return KIND
}

func (b *StoreDriver) GetURL() string {
	return b.destURL
}

func (b *StoreDriver) LocalPath(path string) string {
	return filepath.Join(b.mountDir, path)
}
