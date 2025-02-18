package cdi

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

var (
	ErrHeaderContentLengthNotFound = errors.New("Content-Length is not found in the header")
)

const (
	VolumeModeBlock = "Block"
	AccessModeRWO   = "ReadWriteOnce"
	tickTimeout     = 60 * time.Second
	tickPolling     = 1 * time.Second
	kindPod         = "Pod"
)

type ProgressUpdater struct {
	totalBytes  int64
	targetBytes int64
	lastTime    time.Time
	rwlock      sync.RWMutex

	vmImgUpdateLocker *sync.Mutex
	vmImgCond         *sync.Cond
}

// Write method for ProgressWriter, which updates the progress
func (pu *ProgressUpdater) Write(p []byte) (n int, err error) {
	pu.rwlock.Lock()
	defer pu.rwlock.Unlock()

	// Update the total bytes written
	pu.totalBytes += int64(len(p))
	almostCompleted := pu.totalBytes >= pu.targetBytes

	now := time.Now()
	if now.Sub(pu.lastTime) > 1*time.Second || almostCompleted {
		logrus.Infof("Downloaded %d bytes", pu.totalBytes)
		pu.vmImgUpdateLocker.Lock()
		pu.vmImgCond.Signal()
		pu.vmImgUpdateLocker.Unlock()
		pu.lastTime = now
	}

	return len(p), nil
}

func (pu *ProgressUpdater) GetCurrentBytesNoLock() int64 {
	return pu.totalBytes
}

func generateDVSource(vmi *harvesterv1.VirtualMachineImage) (*cdiv1.DataVolumeSource, error) {
	dvSource := &cdiv1.DataVolumeSource{}
	sourceType := vmi.Spec.SourceType
	switch sourceType {
	case harvesterv1.VirtualMachineImageSourceTypeDownload:
		dvSourceHTTP := generateDVSourceHTTP(vmi)
		dvSource.HTTP = dvSourceHTTP
		return dvSource, nil
	case harvesterv1.VirtualMachineImageSourceTypeUpload:
		dvSourceUpload := &cdiv1.DataVolumeSourceUpload{}
		dvSource.Upload = dvSourceUpload
		return dvSource, nil
	default:
		return dvSource, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

func generateDVSourceHTTP(vmi *harvesterv1.VirtualMachineImage) *cdiv1.DataVolumeSourceHTTP {
	return &cdiv1.DataVolumeSourceHTTP{
		URL: vmi.Spec.URL,
	}
}

func generateDVTargetStorage(vmi *harvesterv1.VirtualMachineImage) (*cdiv1.StorageSpec, error) {
	targetDVStorage := &cdiv1.StorageSpec{}
	targetDVStorage.StorageClassName = &vmi.Spec.TargetStorageClassName
	targetDVStorage.Resources.Requests = make(corev1.ResourceList)
	if vmi.Status.VirtualSize == 0 {
		return nil, fmt.Errorf("virtual size is not set")
	}
	targetDVStorage.Resources.Requests[corev1.ResourceStorage] = *resource.NewQuantity(vmi.Status.VirtualSize, resource.DecimalSI)
	return targetDVStorage, nil
}

func fetchImageSize(url string) (int64, error) {
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return 0, err
	}

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer rsp.Body.Close()

	contentLengthStr := rsp.Header.Get("Content-Length")
	if contentLengthStr == "" {
		return 0, ErrHeaderContentLengthNotFound
	}

	contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64)
	if err != nil {
		return 0, err
	}
	return contentLength, nil
}

func fetchImageVirtualSize(url string) (int64, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", "bytes=0-128")

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer rsp.Body.Close()

	rawContent, err := io.ReadAll(rsp.Body)
	if err != nil {
		return 0, err
	}

	if len(rawContent) < 128 {
		return 0, fmt.Errorf("content length is less than 128 bytes")
	}

	// REF: https://gitlab.com/qemu-project/qemu/-/blob/master/docs/interop/qcow2.txt
	// 0-3 bytes are the magic number, should be "QFI\xfb" for qcow format
	magicNumber := rawContent[0:4]
	if string(magicNumber) != "QFI\xfb" {
		logrus.Infof("Magic number is not correct: %v, this image is not qcow format", magicNumber)
		return 0, nil
	}

	// 24-31 bytes are the virtual size
	virtualSizeRaw := rawContent[24:32]
	virtualSize := binary.BigEndian.Uint64(virtualSizeRaw)

	// ensure the virtual size is not too large, skip gosec G115
	return int64(virtualSize), nil //nolint:gosec

}
