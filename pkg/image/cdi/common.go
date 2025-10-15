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
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

var (
	ErrHeaderContentLengthNotFound = errors.New("Content-Length is not found in the header")
)

const (
	tickTimeout = 60 * time.Second
	tickPolling = 1 * time.Second
)

type ProgressUpdater struct {
	totalBytes  int64
	targetBytes int64
	imageNS     string
	imageName   string
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
		logrus.Infof("Downloaded %d bytes for image %s/%s", pu.totalBytes, pu.imageNS, pu.imageName)
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

func generateDVSource(vmImg *harvesterv1.VirtualMachineImage, sourceType harvesterv1.VirtualMachineImageSourceType) (*cdiv1.DataVolumeSource, error) {
	dvSource := &cdiv1.DataVolumeSource{}
	switch sourceType {
	case harvesterv1.VirtualMachineImageSourceTypeDownload:
		dvSourceHTTP := generateDVSourceHTTP(vmImg)
		dvSource.HTTP = dvSourceHTTP
		return dvSource, nil
	case harvesterv1.VirtualMachineImageSourceTypeUpload:
		dvSourceUpload := &cdiv1.DataVolumeSourceUpload{}
		dvSource.Upload = dvSourceUpload
		return dvSource, nil
	case harvesterv1.VirtualMachineImageSourceTypeExportVolume:
		dvSourceExport := &cdiv1.DataVolumeSourcePVC{}
		dvSourceExport.Name = vmImg.Spec.PVCName
		dvSourceExport.Namespace = vmImg.Namespace
		dvSource.PVC = dvSourceExport
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

func GenerateDVAnnotations(sc *storagev1.StorageClass) map[string]string {
	annotations := map[string]string{}
	if sc.VolumeBindingMode == nil {
		// if volumeBindingMode is not set, means `Immediate` mode
		// do not need to set annotation
		return annotations
	}
	volBindingMode := *sc.VolumeBindingMode
	// need annotation `cdi.kubevirt.io/storage.bind.immediate.requested: "true"`
	if volBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
		annotations[cdicommon.AnnImmediateBinding] = "true"
	}
	return annotations
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
	req.Header.Set("Range", "bytes=0-127")

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer rsp.Body.Close()

	// Use LimitReader to prevent OOM, some http servers may ignore the Range header and return full content
	// which may lead to OOM if the image is too large
	limitedReader := io.LimitReader(rsp.Body, 128)
	rawContent, err := io.ReadAll(limitedReader)
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
