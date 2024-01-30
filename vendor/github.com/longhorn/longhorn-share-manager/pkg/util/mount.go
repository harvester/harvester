package util

import (
	"golang.org/x/sys/unix"
)

// DeviceNumber represents a combined major:minor device number.
type DeviceNumber uint64

// GetDeviceNumber returns the device number of the device node at the given
// path.  If there is a symlink at the path, it is dereferenced.
func GetDeviceNumber(path string) (DeviceNumber, error) {
	var stat unix.Stat_t
	if err := unix.Stat(path, &stat); err != nil {
		return 0, err
	}
	return DeviceNumber(stat.Rdev), nil
}
