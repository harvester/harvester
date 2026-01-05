package longhorn

import (
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
)

// checkVolumeSnapshotError returns an error when a VolumeSnapshot reports one.
func checkVolumeSnapshotError(vs *snapshotv1.VolumeSnapshot) error {
	if vs.Status == nil || vs.Status.Error == nil {
		return nil
	}

	errorMsg := "VolumeSnapshot is in error state"
	if vs.Status.Error.Message != nil {
		errorMsg = *vs.Status.Error.Message
	}
	return fmt.Errorf("%s", errorMsg)
}
