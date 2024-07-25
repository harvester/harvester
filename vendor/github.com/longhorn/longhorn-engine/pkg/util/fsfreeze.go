package util

import (
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/mount-utils"

	lhexec "github.com/longhorn/go-common-libs/exec"
	"github.com/longhorn/go-common-libs/types"
	"github.com/longhorn/go-iscsi-helper/longhorndev"
)

const (
	binaryFsfreeze          = "fsfreeze"
	notFrozenErrorSubstring = "Invalid argument"
	freezePointDirectory    = "/var/lib/longhorn/freeze" // We expect this to be INSIDE the container namespace.
	DevicePathPrefix        = longhorndev.DevPath

	// If the block device is functioning and the filesystem is frozen, fsfreeze -u immediately returns successfully.
	// If the block device is NOT functioning, fsfreeze does not return until I/O errors occur (which can take a long
	// time). In certain situations (e.g. when it is executed during an instance-manager shutdown that has already
	// stopped the associated replica so that I/Os will eventually time out), waiting can impede the shutdown sequence.
	unfreezeTimeout = 5 * time.Second
)

// GetFreezePointFromDevicePath returns the absolute path to the canonical location we will try to mount a filesystem to
// before freezeing it.
func GetFreezePointFromDevicePath(devicePath string) string {
	if devicePath == "" {
		return ""
	}
	return GetFreezePointFromVolumeName(filepath.Base(devicePath))
}

// GetFreezePointFromVolumeName returns the absolute path to the canonical location we will try to mount a filesystem to
// before freezeing it.
func GetFreezePointFromVolumeName(volumeName string) string {
	if volumeName == "" {
		return ""
	}
	return filepath.Join(freezePointDirectory, volumeName)
}

// GetDevicePathFromVolumeName mirrors longhorndev.getDev. It returns the device path that go-iscsi-helper will use.
func GetDevicePathFromVolumeName(volumeName string) string {
	if volumeName == "" {
		return ""
	}
	return filepath.Join(longhorndev.DevPath, volumeName)
}

// FreezeFilesystem attempts to freeze the filesystem mounted at freezePoint.
func FreezeFilesystem(freezePoint string, exec lhexec.ExecuteInterface) error {
	if exec == nil {
		exec = lhexec.NewExecutor()
	}

	// fsfreeze cannot be cancelled. Once it is started, we must wait for it to complete. If we do not, unfreeze will
	// wait for it anyway.
	_, err := exec.Execute([]string{}, binaryFsfreeze, []string{"-f", freezePoint}, types.ExecuteNoTimeout)
	if err != nil {
		return err
	}
	return nil
}

// UnfreezeFilesystem attempts to unfreeze the filesystem mounted at freezePoint. It returns true if it
// successfully unfreezes a filesystem, false if there is no need to unfreeze a filesystem, and an error otherwise.
func UnfreezeFilesystem(freezePoint string, exec lhexec.ExecuteInterface) (bool, error) {
	if exec == nil {
		exec = lhexec.NewExecutor()
	}

	_, err := exec.Execute([]string{}, binaryFsfreeze, []string{"-u", freezePoint}, unfreezeTimeout)
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), notFrozenErrorSubstring) {
		return false, nil
	}
	// It the error message is related to a timeout, there is a decent chance the unfreeze will eventually be
	// successful. While we stop waiting for the unfreeze to complete, the unfreeze process itself cannot be killed.
	// This usually indicates the kernel is locked up waiting for I/O errors to be returned for an iSCSI device that can
	// no longer be reached.
	return false, err
}

// UnfreezeAndUnmountFilesystem attempts to unfreeze the filesystem mounted at freezePoint.
func UnfreezeAndUnmountFilesystem(freezePoint string, exec lhexec.ExecuteInterface,
	mounter mount.Interface) (bool, error) {
	if exec == nil {
		exec = lhexec.NewExecutor()
	}
	if mounter == nil {
		mounter = mount.New("")
	}

	unfroze, err := UnfreezeFilesystem(freezePoint, exec)
	if err != nil {
		return unfroze, err
	}
	return unfroze, mount.CleanupMountPoint(freezePoint, mounter, false)
}

// UnfreezeFilesystemForDevice attempts to identify a mountPoint for the Longhorn volume and unfreeze it. Under normal
// conditions, it will not find a filesystem, and if it finds a filesystem, it will not be frozen.
// UnfreezeFilesystemForDevice does not return an error if there is nothing to do. UnfreezeFilesystemForDevice is only
// relevant for volumes run with a tgt-blockdev frontend, as only these volumes have a Longhorn device on the node to
// format and mount.
func UnfreezeFilesystemForDevice(devicePath string) error {
	// We do not need to switch to the host mount namespace to get mount points here. Usually, longhorn-engine runs in a
	// container that has / bind mounted to /host with at least HostToContainer (rslave) propagation.
	// - If it does not, we likely can't do a namespace swap anyway, since we don't have access to /host/proc.
	// - If it does, we just need to know where in the container we can access the mount points to unfreeze.
	mounter := mount.New("")
	freezePoint := GetFreezePointFromDevicePath(devicePath)

	// First, try to unfreeze and unmount the expected mount point.
	freezePointIsMountPoint, err := mounter.IsMountPoint(freezePoint)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		logrus.WithError(err).Warnf("Failed to determine if %s is a mount point while deciding whether or not to unfreeze", freezePoint)
	}
	if freezePointIsMountPoint {
		unfroze, err := UnfreezeAndUnmountFilesystem(freezePoint, nil, mounter)
		if unfroze {
			logrus.Warnf("Unfroze filesystem mounted at %v", freezePoint)
		}
		return err
	}

	// If a filesystem is not mounted at the expected mount point, try any other mount point of the device.
	mountPoints, err := mounter.List()
	if err != nil {
		return errors.Wrap(err, "failed to list mount points while deciding whether or not unfreeze")
	}
	for _, mountPoint := range mountPoints {
		if mountPoint.Device == devicePath {
			// This one is not ours to unmount.
			unfroze, err := UnfreezeFilesystem(mountPoint.Path, nil)
			if unfroze {
				logrus.Warnf("Unfroze filesystem mounted at %v", freezePoint)
			}
			return err
		}
	}

	return nil
}
