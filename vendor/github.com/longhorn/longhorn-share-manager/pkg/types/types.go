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

	// The suffix for dm device name of the encrypted v2 volume.
	MapperV2VolumeSuffix = "-encrypted"

	ExportPath = "/export"

	DataEngineTypeV1 = "v1"
	DataEngineTypeV2 = "v2"
)

func GetEncryptVolumeName(volume, dataEngine string) string {
	if dataEngine == DataEngineTypeV2 {
		return volume + MapperV2VolumeSuffix
	}
	return volume
}

func GetVolumeDevicePath(volumeName, dataEngine string, EncryptedDevice bool) string {
	if EncryptedDevice {
		if dataEngine == DataEngineTypeV2 {
			// v2 volume will use a dm device as default to control IO path when attaching. This dm device will be created with the same name as the volume name.
			// The encrypted volume will be created with the volume name with "-encrypted" suffix to resolve the naming conflict.
			return path.Join(MapperDevPath, GetEncryptVolumeName(volumeName, dataEngine))
		}
		return path.Join(MapperDevPath, volumeName)
	}
	return filepath.Join(DevPath, "longhorn", volumeName)
}

func GetMountPath(volumeName string) string {
	return filepath.Join(ExportPath, volumeName)
}
