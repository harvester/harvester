package volume

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_IsResizing(t *testing.T) {
	var testCases = []struct {
		Name       string
		PVC        *corev1.PersistentVolumeClaim
		IsResizing bool
	}{
		{
			Name: "longhorn pvc which is not resizing",
			PVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"volume.kubernetes.io/storage-provisioner": "driver.longhorn.io",
					},
				},
				Status: corev1.PersistentVolumeClaimStatus{},
			},
			IsResizing: false,
		},
		{
			Name: "longhorn pvc which is resizing",
			PVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"volume.kubernetes.io/storage-provisioner": "driver.longhorn.io",
					},
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Conditions: []corev1.PersistentVolumeClaimCondition{
						{
							Type:   corev1.PersistentVolumeClaimResizing,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			IsResizing: true,
		},
		{
			Name: "lvm pvc which is resizing",
			PVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"volume.kubernetes.io/storage-provisioner": "lvm.driver.harvesterhci.io",
					},
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Conditions: []corev1.PersistentVolumeClaimCondition{
						{
							Type:   corev1.PersistentVolumeClaimResizing,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.PersistentVolumeClaimFileSystemResizePending,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			IsResizing: false,
		},
		{
			Name: "lvm pvc which is not resizing",
			PVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"volume.kubernetes.io/storage-provisioner": "lvm.driver.harvesterhci.io",
					},
				},
				Status: corev1.PersistentVolumeClaimStatus{},
			},
			IsResizing: false,
		},
		{
			Name: "pending pvc",
			PVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimPending,
				},
			},
			IsResizing: false,
		},
	}

	assert := require.New(t)
	for _, tc := range testCases {
		ok := IsResizing(tc.PVC, nil)
		if tc.IsResizing {
			assert.True(ok, tc.Name)
		} else {
			assert.False(ok, tc.Name)
		}
	}

}
