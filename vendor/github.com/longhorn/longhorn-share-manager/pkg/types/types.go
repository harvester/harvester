package types

import (
	"path"
	"path/filepath"
	"time"
)

const (
	GRPCServiceTimeout = 1 * time.Minute

	DevPath       = "/dev"
	MapperDevPath = "/dev/mapper"

	ExportPath = "/export"
)

func GetVolumeDevicePath(volumeName string, EncryptedDevice bool) string {
	if EncryptedDevice {
		return path.Join(MapperDevPath, volumeName)
	}
	return filepath.Join(DevPath, "longhorn", volumeName)
}

func GetMountPath(volumeName string) string {
	return filepath.Join(ExportPath, volumeName)
}
