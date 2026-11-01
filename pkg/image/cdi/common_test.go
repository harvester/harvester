package cdi

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

func TestGenerateDVTargetStorageAccessModes(t *testing.T) {
	base := harvesterv1.VirtualMachineImage{
		Spec:   harvesterv1.VirtualMachineImageSpec{TargetStorageClassName: "longhorn-static"},
		Status: harvesterv1.VirtualMachineImageStatus{VirtualSize: 1073741824},
	}

	cases := []struct {
		name        string
		annotations map[string]string
		wantRWO     bool
	}{
		{
			name:        "upgrade image sets RWO",
			annotations: map[string]string{util.AnnotationUpgradeImage: "True"},
			wantRWO:     true,
		},
		{
			name:        "non-upgrade image leaves AccessModes unset",
			annotations: nil,
			wantRWO:     false,
		},
		{
			name:        "annotation value not True is ignored",
			annotations: map[string]string{util.AnnotationUpgradeImage: "false"},
			wantRWO:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vmi := base.DeepCopy()
			if tc.annotations != nil {
				vmi.Annotations = tc.annotations
			}

			storage, err := generateDVTargetStorage(vmi)
			require.NoError(t, err)
			require.Nil(t, storage.VolumeMode, "volume mode must not be forced")
			if tc.wantRWO {
				require.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, storage.AccessModes)
			} else {
				require.Empty(t, storage.AccessModes)
			}
		})
	}
}

func TestGenerateDVTargetStorageUsesExactImageSize(t *testing.T) {
	vmi := &harvesterv1.VirtualMachineImage{
		Spec: harvesterv1.VirtualMachineImageSpec{
			TargetStorageClassName: "test-storage-class",
		},
		Status: harvesterv1.VirtualMachineImageStatus{
			VirtualSize: 2361393152,
			Size:        735465984,
		},
	}

	storage, err := generateDVTargetStorage(vmi)
	require.NoError(t, err)
	require.Equal(t, "test-storage-class", *storage.StorageClassName)
	request := storage.Resources.Requests[corev1.ResourceStorage]
	require.Equal(t, int64(2361393152), request.Value())
}
