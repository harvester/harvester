package types

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	SnapPrefix = "snap"
	VolPrefix  = "vol"

	VolumeNameKey   = "volumeName"
	SnapshotNameKey = "snapshotName"
)

func NewVolumeDataSource(volumeDataSourceType longhorn.VolumeDataSourceType, parameters map[string]string) (dataSource longhorn.VolumeDataSource, err error) {
	defer func() {
		err = errors.Wrapf(err, "cannot create new longhorn.VolumeDataSource of type %v with parameters %v", volumeDataSourceType, parameters)
	}()

	switch volumeDataSourceType {
	case longhorn.VolumeDataSourceTypeVolume:
		volumeName := parameters[VolumeNameKey]
		if volumeName == "" {
			return longhorn.VolumeDataSource(""), fmt.Errorf("volume name is empty")
		}
		return NewVolumeDataSourceTypeVolume(volumeName), nil
	case longhorn.VolumeDataSourceTypeSnapshot:
		volumeName := parameters[VolumeNameKey]
		snapshotName := parameters[SnapshotNameKey]
		if volumeName == "" {
			return longhorn.VolumeDataSource(""), fmt.Errorf("volume name is empty")
		}
		if snapshotName == "" {
			return longhorn.VolumeDataSource(""), fmt.Errorf("snapshot name is empty")
		}
		return NewVolumeDataSourceTypeSnapshot(volumeName, snapshotName), nil
	default:
		return longhorn.VolumeDataSource(""), fmt.Errorf("invalid volumeDataSourceType")
	}
}

func NewVolumeDataSourceTypeVolume(volumeName string) longhorn.VolumeDataSource {
	return longhorn.VolumeDataSource(fmt.Sprintf("%v://%s", VolPrefix, volumeName))
}

func NewVolumeDataSourceTypeSnapshot(volumeName, snapshotName string) longhorn.VolumeDataSource {
	return longhorn.VolumeDataSource(fmt.Sprintf("%v://%s/%s", SnapPrefix, volumeName, snapshotName))
}

func IsValidVolumeDataSource(vds longhorn.VolumeDataSource) bool {
	split := strings.Split(string(vds), "://")
	if len(split) != 2 {
		return false
	}
	prefix := split[0]

	switch prefix {
	case VolPrefix:
		volumeName := decodeDataSourceTypeVolume(vds)
		return volumeName != ""
	case SnapPrefix:
		volumeName, snapshotName := decodeDataSourceTypeSnapshot(vds)
		return volumeName != "" && snapshotName != ""
	default:
		return false
	}
}

func getType(vds longhorn.VolumeDataSource) longhorn.VolumeDataSourceType {
	split := strings.Split(string(vds), "://")
	if len(split) != 2 {
		return ""
	}
	prefix := split[0]
	switch prefix {
	case VolPrefix:
		return longhorn.VolumeDataSourceTypeVolume
	case SnapPrefix:
		return longhorn.VolumeDataSourceTypeSnapshot
	default:
		return ""
	}
}

func IsDataFromVolume(vds longhorn.VolumeDataSource) bool {
	t := getType(vds)
	return t == longhorn.VolumeDataSourceTypeVolume || t == longhorn.VolumeDataSourceTypeSnapshot
}

func GetVolumeName(vds longhorn.VolumeDataSource) string {
	switch getType(vds) {
	case longhorn.VolumeDataSourceTypeVolume:
		volName := decodeDataSourceTypeVolume(vds)
		return volName
	case longhorn.VolumeDataSourceTypeSnapshot:
		volName, _ := decodeDataSourceTypeSnapshot(vds)
		return volName
	default:
		return ""
	}
}

func GetSnapshotName(vds longhorn.VolumeDataSource) string {
	switch getType(vds) {
	case longhorn.VolumeDataSourceTypeSnapshot:
		_, snapshotName := decodeDataSourceTypeSnapshot(vds)
		return snapshotName
	default:
		return ""
	}
}

func decodeDataSourceTypeVolume(vds longhorn.VolumeDataSource) (volumeName string) {
	split := strings.Split(string(vds), "://")
	if len(split) != 2 {
		return ""
	}
	return split[1]
}

func decodeDataSourceTypeSnapshot(vds longhorn.VolumeDataSource) (volumeName, snapshotName string) {
	split := strings.Split(string(vds), "://")
	if len(split) != 2 {
		return "", ""
	}
	split = strings.Split(split[1], "/")
	if len(split) != 2 {
		return "", ""
	}
	return split[0], split[1]
}
