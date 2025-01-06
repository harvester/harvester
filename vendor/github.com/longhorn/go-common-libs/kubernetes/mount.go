package kubernetes

import (
	"k8s.io/mount-utils"
)

func IsMountPointReadOnly(mp mount.MountPoint) bool {
	for _, opt := range mp.Opts {
		if opt == "ro" {
			return true
		}
	}
	return false
}
