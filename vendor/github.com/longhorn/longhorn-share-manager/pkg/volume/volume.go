package volume

import (
	"fmt"
	"os"

	"k8s.io/kubernetes/pkg/volume/util/hostutil"
	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

type Volume struct {
	Name            string
	Passphrase      string
	CryptoKeyCipher string
	CryptoKeyHash   string
	CryptoKeySize   string
	CryptoPBKDF     string
	FsType          string
	MountOptions    []string
}

func (v Volume) IsEncrypted() bool {
	return len(v.Passphrase) > 0
}

func GetDiskFormat(devicePath string) (string, error) {
	mounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: utilexec.New()}
	return mounter.GetDiskFormat(devicePath)
}

func CheckDeviceValid(devicePath string) bool {
	isDevice, err := hostutil.NewHostUtil().PathIsDevice(devicePath)
	return err == nil && isDevice
}

func CheckMountValid(mountPath string) bool {
	notMnt, err := mount.IsNotMountPoint(mount.New(""), mountPath)
	return err == nil && !notMnt
}

func MountVolume(devicePath, mountPath, fsType string, mountOptions []string) error {
	if !CheckDeviceValid(devicePath) {
		return fmt.Errorf("cannot mount device %v to %v invalid device", devicePath, mountPath)
	}

	if CheckMountValid(mountPath) {
		return nil
	}

	mounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: utilexec.New()}

	if exists, err := hostutil.NewHostUtil().PathExists(mountPath); !exists || err != nil {
		if err != nil {
			return err
		}

		if err := makeDir(mountPath); err != nil {
			return err
		}
	}

	return mounter.FormatAndMount(devicePath, mountPath, fsType, mountOptions)
}

func ResizeVolume(devicePath, mountPath string) (bool, error) {
	// check if we need to resize the fs
	// this is important since cloned volumes of bigger size don't trigger NodeExpandVolume
	// therefore NodeExpandVolume is kind of redundant since we have to do this anyway
	// some refs below for more details
	// https://github.com/kubernetes/kubernetes/issues/94929
	// https://github.com/kubernetes-sigs/aws-ebs-csi-driver/pull/753
	resizer := mount.NewResizeFs(utilexec.New())
	if needsResize, err := resizer.NeedResize(devicePath, mountPath); err != nil {
		return false, err
	} else if needsResize {
		return resizer.Resize(devicePath, mountPath)
	}

	return false, nil
}

func SetPermissions(mountPath string, mode os.FileMode) error {
	if !CheckMountValid(mountPath) {
		return fmt.Errorf("cannot set permissions %v for path %v invalid mount point", mode, mountPath)
	}

	return os.Chmod(mountPath, mode)
}

func UnmountVolume(mountPath string) error {
	mounter := mount.New("")
	return mounter.Unmount(mountPath)
}

// makeDir creates a new directory.
// If pathname already exists as a directory, no error is returned.
// If pathname already exists as a file, an error is returned.
func makeDir(pathname string) error {
	err := os.MkdirAll(pathname, os.FileMode(0777))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	return nil
}
