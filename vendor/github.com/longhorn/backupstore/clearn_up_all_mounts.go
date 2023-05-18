package backupstore

import (
	"os"

	"github.com/longhorn/backupstore/util"
	mount "k8s.io/mount-utils"
)

func CleanUpAllMounts() (err error) {
	mounter := mount.New("")

	if _, err := os.Stat(util.MountDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	err = util.CleanUpMountPoints(mounter, log)
	return err
}
