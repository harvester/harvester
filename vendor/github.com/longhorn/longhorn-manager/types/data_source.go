package types

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

const (
	SnapPrefix = "snap"
	VolPrefix  = "vol"

	VolumeNameKey   = "volumeName"
	SnapshotNameKey = "snapshotName"
)

func NewVolumeDataSource(volumeDataSourceType string, parameters map[string]string) (dataSource VolumeDataSource, err error) {
	defer func() {
		err = errors.Wrapf(err, "cannot create new VolumeDataSource of type %v with parameters %v", volumeDataSourceType, parameters)
	}()

	switch volumeDataSourceType {
	case VolumeDataSourceTypeVolume:
		volumeName := parameters[VolumeNameKey]
		if volumeName == "" {
			return VolumeDataSource(""), fmt.Errorf("volume name is empty")
		}
		return NewVolumeDataSourceTypeVolume(volumeName), nil
	case VolumeDataSourceTypeSnapshot:
		volumeName := parameters[VolumeNameKey]
		snapshotName := parameters[SnapshotNameKey]
		if volumeName == "" {
			return VolumeDataSource(""), fmt.Errorf("volume name is empty")
		}
		if snapshotName == "" {
			return VolumeDataSource(""), fmt.Errorf("snapshot name is empty")
		}
		return NewVolumeDataSourceTypeSnapshot(volumeName, snapshotName), nil
	default:
		return VolumeDataSource(""), fmt.Errorf("invalid volumeDataSourceType")
	}
}

func NewVolumeDataSourceTypeVolume(volumeName string) VolumeDataSource {
	return VolumeDataSource(fmt.Sprintf("%v://%s", VolPrefix, volumeName))
}

func NewVolumeDataSourceTypeSnapshot(volumeName, snapshotName string) VolumeDataSource {
	return VolumeDataSource(fmt.Sprintf("%v://%s/%s", SnapPrefix, volumeName, snapshotName))
}

func IsValidVolumeDataSource(vds VolumeDataSource) bool {
	split := strings.Split(string(vds), "://")
	if len(split) != 2 {
		return false
	}
	prefix := split[0]

	switch prefix {
	case VolPrefix:
		volumeName := vds.decodeDataSourceTypeVolume()
		return volumeName != ""
	case SnapPrefix:
		volumeName, snapshotName := vds.decodeDataSourceTypeSnapshot()
		return volumeName != "" && snapshotName != ""
	default:
		return false
	}
}

func (vds VolumeDataSource) GetType() string {
	split := strings.Split(string(vds), "://")
	if len(split) != 2 {
		return ""
	}
	prefix := split[0]
	switch prefix {
	case VolPrefix:
		return VolumeDataSourceTypeVolume
	case SnapPrefix:
		return VolumeDataSourceTypeSnapshot
	default:
		return ""
	}
}

func (vds VolumeDataSource) IsDataFromVolume() bool {
	t := vds.GetType()
	return t == VolumeDataSourceTypeVolume || t == VolumeDataSourceTypeSnapshot
}

func (vds VolumeDataSource) GetVolumeName() string {
	switch vds.GetType() {
	case VolumeDataSourceTypeVolume:
		volName := vds.decodeDataSourceTypeVolume()
		return volName
	case VolumeDataSourceTypeSnapshot:
		volName, _ := vds.decodeDataSourceTypeSnapshot()
		return volName
	default:
		return ""
	}
}

func (vds VolumeDataSource) GetSnapshotName() string {
	switch vds.GetType() {
	case VolumeDataSourceTypeSnapshot:
		_, snapshotName := vds.decodeDataSourceTypeSnapshot()
		return snapshotName
	default:
		return ""
	}
}

func (vds VolumeDataSource) decodeDataSourceTypeVolume() (volumeName string) {
	split := strings.Split(string(vds), "://")
	if len(split) != 2 {
		return ""
	}
	return split[1]
}

func (vds VolumeDataSource) decodeDataSourceTypeSnapshot() (volumeName, snapshotName string) {
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

func (vds VolumeDataSource) ToString() string {
	return string(vds)
}
