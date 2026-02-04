package manager

import (
	corev1 "k8s.io/api/core/v1"
)

func (m *VolumeManager) GetLonghornEventList() (*corev1.EventList, error) {
	return m.ds.GetLonghornEventList()
}
