package cdi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	uploadcdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/upload/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlcdiuploadv1 "github.com/harvester/harvester/pkg/generated/controllers/upload.cdi.kubevirt.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
)

const (
	CDIUploadURLRaw = "cdi-uploadproxy.harvester-system"
	UploadURI       = "/v1beta1/upload"
	UploadFormURI   = "/v1beta1/upload-form"
)

type Uploader struct {
	dataVolumeClient ctlcdiv1.DataVolumeClient
	scClient         ctlstoragev1.StorageClassClient
	cdiUploadClient  ctlcdiuploadv1.UploadTokenRequestClient
	httpClient       http.Client
	vmio             common.VMIOperator
}

func GetUploader(dataVolumeClient ctlcdiv1.DataVolumeClient,
	scClient ctlstoragev1.StorageClassClient,
	cdiUploadClient ctlcdiuploadv1.UploadTokenRequestClient,
	httpClient http.Client,
	vmio common.VMIOperator) backend.Uploader {

	// set insecure as default
	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &Uploader{
		dataVolumeClient: dataVolumeClient,
		scClient:         scClient,
		cdiUploadClient:  cdiUploadClient,
		httpClient:       httpClient,
		vmio:             vmio,
	}
}

func (cu *Uploader) DoUpload(vmImg *harvesterv1.VirtualMachineImage, req *http.Request) error {
	var err, uploadErr error
	defer func() {
		if err != nil {
			if updateErr := cu.vmio.FailUpload(vmImg, err.Error()); updateErr != nil {
				logrus.Error(err)
			}
		}
	}()

	updaterLocker := &sync.Mutex{}
	updaterCond := sync.NewCond(updaterLocker)

	urlParams := req.URL.Query()

	// check file size
	fileSizeStr := urlParams.Get("size")
	fileSize, err := strconv.ParseInt(fileSizeStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse file size: %v", err)
	}
	virtualSize := int64(fileSize)

	// check multipart
	contentType := req.Header.Get("Content-Type")
	isMultipartFormat := false
	if strings.HasPrefix(contentType, "multipart/form-data") {
		isMultipartFormat = true
	}

	// no matter multipart or not, the first 4k would be enough
	// to find the magic number and virtual size
	tmpBuff := make([]byte, 1024)
	readLen, err := req.Body.Read(tmpBuff)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read the first 1k request body (for checking format): %v", err)
	}
	logrus.Debugf("Read %d bytes from the request body", readLen)

	rawContent := tmpBuff[:readLen]
	// try to find the magic number of first 4096 bytes
	// the multipart body will contain the boundary string and the headers.
	// We should still find the magic number in the first 4096 bytes
	qcowMagic := []byte("QFI\xfb")
	qcowHeaderIndex := bytes.Index(rawContent, qcowMagic)
	logrus.Debugf("first 1024 bytes: %v", string(rawContent))
	if qcowHeaderIndex == -1 {
		logrus.Infof("Magic number is not correct: %v, this image is not qcow format", rawContent)
	} else {
		// The virtual size is at 24-31 bytes (from the qcow image header)
		virtualSizeRaw := rawContent[qcowHeaderIndex+24 : qcowHeaderIndex+32]
		// ensure the virtual size is not too large, skip gosec G115
		virtualSize = int64(binary.BigEndian.Uint64(virtualSizeRaw)) //nolint:gosec
	}
	logrus.Debugf("Qcow header index: %v, virtual size: %v", qcowHeaderIndex, virtualSize)

	if err = cu.updateVirtualSizeAndSize(vmImg, virtualSize, fileSize); err != nil {
		return err
	}

	// check VMImage status again (for size/virtual size)
	if err := wait.PollUntilContextTimeout(context.Background(), tickPolling, tickTimeout, true, func(context.Context) (bool, error) {
		return cu.waitVMImageStatus(vmImg, fileSize, virtualSize)
	}); err != nil {
		return fmt.Errorf("failed to wait for VMImage status: %v", err)
	}

	// get the latest VMImage
	vmImg, err = cu.vmio.GetVMImageObj(vmImg.Namespace, vmImg.Name)
	if err != nil {
		return fmt.Errorf("failed to get VMImage: %v", err)
	}

	// create DataVolume
	dvName := cu.vmio.GetName(vmImg)
	dvNamespace := cu.vmio.GetNamespace(vmImg)

	logrus.Infof("The VM Image Status updated, start to create DataVolume %s/%s", dvNamespace, dvName)

	targetSC, err := cu.scClient.Get(vmImg.Spec.TargetStorageClassName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get StorageClass %s: %v", vmImg.Spec.TargetStorageClassName, err)
	}

	// generate DV source
	dvSource, err := generateDVSource(vmImg, cu.vmio.GetSourceType(vmImg))
	if err != nil {
		return fmt.Errorf("failed to generate DV source: %v", err)
	}

	// generate DV target storage
	dvTargetStorage, err := generateDVTargetStorage(vmImg)
	if err != nil {
		return fmt.Errorf("failed to generate DV target storage: %v", err)
	}
	var boolTrue = true
	dataVolumeTemplate := &cdiv1.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: GenerateDVAnnotations(targetSC),
			Name:        dvName,
			Namespace:   dvNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         common.HarvesterAPIV1Beta1,
					Kind:               common.VMImageKind,
					Name:               cu.vmio.GetName(vmImg),
					UID:                cu.vmio.GetUID(vmImg),
					BlockOwnerDeletion: &boolTrue,
				},
			},
		},
		Spec: cdiv1.DataVolumeSpec{
			Source:  dvSource,
			Storage: dvTargetStorage,
		},
	}
	if _, err := cu.dataVolumeClient.Create(dataVolumeTemplate); err != nil {
		return fmt.Errorf("failed to create DataVolume %s/%s: %v", dvNamespace, dvName, err)
	}
	logrus.Infof("DataVolume %s/%s created", dvNamespace, dvName)

	// wait data volume UploadReady
	if err := wait.PollUntilContextTimeout(context.Background(), tickPolling, tickTimeout, true, func(context.Context) (bool, error) {
		return cu.waitDataVolumeStatus(dvNamespace, dvName, cdiv1.UploadReady)
	}); err != nil {
		return fmt.Errorf("failed to wait for VMImage status: %v", err)
	}

	uploadTokenRequest := &uploadcdiv1.UploadTokenRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "upload-token-",
			Namespace:    dvNamespace,
		},
		Spec: uploadcdiv1.UploadTokenRequestSpec{
			PvcName: dvName,
		},
	}
	retUploadTokenRequest, err := cu.cdiUploadClient.Create(uploadTokenRequest)
	if err != nil {
		return fmt.Errorf("failed to create UploadTokenRequest %s/%s: %v", dvNamespace, dvName, err)
	}

	newBody := io.MultiReader(bytes.NewReader(rawContent), req.Body)

	progress := &ProgressUpdater{
		targetBytes:       fileSize,
		lastTime:          time.Now(),
		imageNS:           vmImg.Namespace,
		imageName:         vmImg.Name,
		vmImgUpdateLocker: updaterLocker,
		vmImgCond:         updaterCond,
	}
	hookedReader := io.TeeReader(newBody, progress)

	token := retUploadTokenRequest.Status.Token
	uploadReq, err := http.NewRequestWithContext(req.Context(), http.MethodPost, getUploadFormURL(isMultipartFormat), io.NopCloser(hookedReader))
	if err != nil {
		return fmt.Errorf("failed to wrap the upload request: %w", err)
	}
	uploadReq.Header = req.Header
	uploadReq.Header.Add("Authorization", "Bearer "+token)

	// create VMI progress updater
	go cu.updateVMImageProgress(vmImg, updaterCond, progress, fileSize, &uploadErr)
	defer func() {
		updaterLocker.Lock()
		logrus.Debugf("Calling wake up with defer function (err: %v), need to update the VMImage status", err)
		uploadErr = err
		updaterCond.Signal()
		updaterLocker.Unlock()
	}()

	var urlErr *url.Error
	uploadResp, err := cu.httpClient.Do(uploadReq)
	if errors.As(err, &urlErr) {
		// Trim the "POST http://xxx" implementation detail for the error
		// set the err var and it will be recorded in image condition in the defer function
		err = errors.Unwrap(urlErr)
		return err
	} else if err != nil {
		return fmt.Errorf("failed to send the upload request: %w", err)
	}
	defer uploadResp.Body.Close()

	body, err := io.ReadAll(uploadResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	if uploadResp.StatusCode >= http.StatusBadRequest {
		// err will be recorded in image condition in the defer function
		err = fmt.Errorf("upload failed: %s", string(body))
		return err
	}

	// we don't wait the DataVolume to be Succeeded here.
	// the controller will help with that.
	// for the progress, we set it to 99% here on progress updater

	return nil
}

func (cu *Uploader) waitVMImageStatus(vmImg *harvesterv1.VirtualMachineImage, size, virtualSize int64) (bool, error) {
	vmImg, err := cu.vmio.GetVMImageObj(vmImg.Namespace, vmImg.Name)
	if err != nil {
		return false, err
	}
	logrus.Debugf("Current VMImage status: size(%d), virtual size(%d), target size(%d), virtual size(%d)", vmImg.Status.Size, vmImg.Status.VirtualSize, size, virtualSize)
	if vmImg.Status.Size == size && vmImg.Status.VirtualSize == virtualSize {
		return true, nil
	}
	return false, nil
}

func (cu *Uploader) waitDataVolumeStatus(namespace, name string, targetState cdiv1.DataVolumePhase) (bool, error) {
	dv, err := cu.dataVolumeClient.Get(namespace, name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	if dv.Status.Phase == targetState {
		return true, nil
	}
	logrus.Debugf("Current DataVolume %s/%s status: %v, target status: %v", namespace, name, dv.Status.Phase, targetState)
	return false, nil
}

func (cu *Uploader) updateVMImageProgress(vmImg *harvesterv1.VirtualMachineImage, cond *sync.Cond, updater *ProgressUpdater, targetSize int64, anyErr *error) {
	uploadCompleted := false

	// get the NS/Name for logging purpose
	// the vmImg might be update in the loop (which means it might be a nil object if deleted)
	vmImgNS := vmImg.Namespace
	vmImgName := vmImg.Name
	for {
		cond.L.Lock()
		cond.Wait()

		if *anyErr != nil {
			logrus.Errorf("Found error (%v) during upload image (%s/%s), stop the progress updater", *anyErr, vmImgNS, vmImgName)
			cond.L.Unlock()
			// we could ignore failure update here, another defer function will handle it
			return
		}

		var err error
		// ensure the vmImage is the latest
		vmImg, err = cu.vmio.GetVMImageObj(vmImg.Namespace, vmImg.Name)
		if err != nil {
			logrus.Errorf("[updateVMImageProgress] failed to get VMImage %s/%s: %v, ignore the latest", vmImgNS, vmImgName, err)
			cond.L.Unlock()
			continue
		}

		// check if the upload is finished
		targetDVNs := cu.vmio.GetNamespace(vmImg)
		targetDVName := cu.vmio.GetName(vmImg)
		targetDV, err := cu.dataVolumeClient.Get(targetDVNs, targetDVName, metav1.GetOptions{})
		if err != nil {
			logrus.Errorf("[updateVMImageProgress] failed to get DataVolume %s/%s: %v", targetDVNs, targetDVName, err)
			cond.L.Unlock()
			continue
		}
		if targetDV.Status.Phase == cdiv1.Succeeded {
			logrus.Infof("DataVolume %s/%s upload finished", targetDVNs, targetDVName)
			_, err := cu.vmio.Imported(vmImg, "", 100, vmImg.Status.Size, vmImg.Status.VirtualSize)
			if err != nil {
				// signal again to retry the update
				logrus.Errorf("failed to update VMImage status: %v", err)
				cond.Signal()
				cond.L.Unlock()
				continue
			}
			cond.L.Unlock()
			return
		}

		currentBytes := updater.GetCurrentBytesNoLock()
		progress := (float64(currentBytes) / float64(targetSize)) * 100
		if progress >= 100 {
			// keep almost done progress, we need to wait the DataVolume status to be succeeded
			// on the controller side
			progress = 99
			uploadCompleted = true
		}
		logrus.Debugf("DataVolume %s/%s upload progress: %d/%d (%v)", targetDVNs, targetDVName, currentBytes, targetSize, progress)
		_, err = cu.vmio.Importing(vmImg, "Image Importing", int(progress))
		if err != nil {
			logrus.Errorf("failed to update VMImage status: %v", err)
		}
		cond.L.Unlock()
		if uploadCompleted {
			logrus.Infof("Upload image (%s/%s) completed, stop the progress updater", vmImgNS, vmImgName)
			return
		}
	}
}

func getUploadFormURL(isMultipartFormat bool) string {
	if isMultipartFormat {
		return fmt.Sprintf("https://%s%s", CDIUploadURLRaw, UploadFormURI)
	}
	return fmt.Sprintf("https://%s%s", CDIUploadURLRaw, UploadURI)
}

func (cu *Uploader) updateVirtualSizeAndSize(vmImg *harvesterv1.VirtualMachineImage, virtualSize, fileSize int64) error {
	// retry is needed here as the vm image controller could update the VMImage at the same time
	if _, err := cu.updateVMIWithRetryOnConflict(vmImg, func(img *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
		return cu.vmio.UpdateVirtualSizeAndSize(img, virtualSize, fileSize)
	}); err != nil {
		return fmt.Errorf("failed to update VM Image size and virtual size: %v", err)
	}
	return nil
}

func (cu *Uploader) updateVMIWithRetryOnConflict(
	vmImg *harvesterv1.VirtualMachineImage,
	updateFunc func(*harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error),
) (*harvesterv1.VirtualMachineImage, error) {
	namespace := vmImg.Namespace
	name := vmImg.Name
	var updatedVMI *harvesterv1.VirtualMachineImage

	err := retry.OnError(retry.DefaultRetry, func(err error) bool {
		// Retry on conflict errors or transient API errors
		return apierrors.IsConflict(err) || apierrors.IsServerTimeout(err) || apierrors.IsTimeout(err) || apierrors.IsServiceUnavailable(err)
	}, func() error {
		// Fetch the latest VMImage before each retry attempt to get the current resource version
		latestVMI, err := cu.vmio.GetVMImageObj(namespace, name)
		if err != nil {
			logrus.Warnf("Failed to get latest VMImage %s/%s: %v", namespace, name, err)
			return err
		}

		newVMI, err := updateFunc(latestVMI.DeepCopy())
		if err != nil {
			return err
		}

		updatedVMI, err = cu.vmio.UpdateVMI(latestVMI, newVMI)
		if err != nil {
			logrus.Warnf("Failed to update VM Image: %v, will retry if retriable.", err)
		}
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("failed to update VM Image %s/%s after retries: %v", namespace, name, err)
	}

	return updatedVMI, nil
}
