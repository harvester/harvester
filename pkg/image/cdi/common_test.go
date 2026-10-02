package cdi

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

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
