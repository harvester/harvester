package cdi

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestFetchImageProbesTimeOutOnStalledServer(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	client := &http.Client{Timeout: 200 * time.Millisecond}
	probes := map[string]func() (int64, error){
		"fetchImageSize": func() (int64, error) {
			return fetchImageSize(context.Background(), client, srv.URL)
		},
		"fetchImageVirtualSize": func() (int64, error) {
			return fetchImageVirtualSize(context.Background(), client, srv.URL)
		},
	}
	for name, probe := range probes {
		t.Run(name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() {
				_, err := probe()
				done <- err
			}()
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("probe did not honor the client timeout")
			}
		})
	}
}

func TestFetchImageProbes(t *testing.T) {
	const size = 1024
	const virtualSize = uint64(10 << 30)
	header := make([]byte, size)
	copy(header, "QFI\xfb")
	binary.BigEndian.PutUint64(header[24:32], virtualSize)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "image.qcow2", time.Time{}, bytes.NewReader(header))
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	gotSize, err := fetchImageSize(context.Background(), client, srv.URL)
	require.NoError(t, err)
	require.Equal(t, int64(size), gotSize)

	gotVirtualSize, err := fetchImageVirtualSize(context.Background(), client, srv.URL)
	require.NoError(t, err)
	require.Equal(t, int64(virtualSize), gotVirtualSize)
}
