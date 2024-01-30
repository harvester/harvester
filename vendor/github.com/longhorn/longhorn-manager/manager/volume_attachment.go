package manager

import (
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (m *VolumeManager) GetVolumeAttachment(volumeName string) (*longhorn.VolumeAttachment, error) {
	return m.ds.GetLHVolumeAttachmentByVolumeName(volumeName)
}
