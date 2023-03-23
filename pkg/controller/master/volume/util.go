package volume

import (
	lhv1beta1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
)

func isVolumeDetached(volume *lhv1beta1.Volume) bool {
	return volume.Status.State == lhv1beta1.VolumeStateDetached || volume.Status.State == lhv1beta1.VolumeStateDetaching
}
