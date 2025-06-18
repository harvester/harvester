package cdi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
)

type Downloader struct {
	vmImageDownloaderClient v1beta1.VirtualMachineImageDownloaderClient
	httpClient              http.Client
	vmio                    common.VMIOperator
}

func GetDownloader(vmImageDownloaderClient v1beta1.VirtualMachineImageDownloaderClient, httpClient http.Client, vmio common.VMIOperator) backend.Downloader {
	return &Downloader{
		vmImageDownloaderClient: vmImageDownloaderClient,
		httpClient:              httpClient,
		vmio:                    vmio,
	}
}

func (cd *Downloader) DoDownload(vmImg *harvesterv1.VirtualMachineImage, rw http.ResponseWriter, req *http.Request) error {
	if !cd.vmio.IsImported(vmImg) {
		return fmt.Errorf("please wait until the image has been imported")
	}

	vmImgName := cd.vmio.GetName(vmImg)
	vmImgNamespace := cd.vmio.GetNamespace(vmImg)
	imageDownloader, err := cd.vmImageDownloaderClient.Get(vmImgNamespace, vmImgName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get vm image downloader %s: %w", vmImgName, err)
	}
	if imageDownloader.Status.Status != harvesterv1.ImageDownloaderStatusReady {
		return fmt.Errorf("vm image downloader %s is not ready", vmImgName)
	}
	targetFileName := fmt.Sprintf("%s.qcow2", vmImgName)
	downloadURL := imageDownloader.Status.DownloadURL

	// ensure the target file is ready
	if err := wait.PollUntilContextTimeout(context.Background(), tickPolling, tickTimeout, true, func(context.Context) (bool, error) {
		return cd.endPointReady(downloadURL)
	}); err != nil {
		return fmt.Errorf("failed to wait for service ready")
	}

	// once the download server is ready, we need defer function to delete the downloader with any error
	defer func() {
		// Delete downloader after download
		if err := cd.vmImageDownloaderClient.Delete(vmImgNamespace, vmImgName, &metav1.DeleteOptions{}); err != nil {
			// just log the error, we cannot do anything at this moment
			logrus.Errorf("failed to delete ImageDownloader %s/%s: %v", vmImgNamespace, vmImgName, err)
		}
	}()

	downloadReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create the download request with VM Image(%s): %w", targetFileName, err)
	}

	downloadResp, err := cd.httpClient.Do(downloadReq)
	if err != nil {
		return fmt.Errorf("failed to send the download request with VM Image(%s): %w", targetFileName, err)
	}
	defer downloadResp.Body.Close()

	rw.Header().Set("Content-Disposition", "attachment; filename="+targetFileName)
	contentType := downloadResp.Header.Get("Content-Type")
	if contentType != "" {
		rw.Header().Set("Content-Type", contentType)
	}

	if _, err := io.Copy(rw, downloadResp.Body); err != nil {
		return fmt.Errorf("failed to copy download content to target(%s), err: %w", targetFileName, err)
	}

	return nil
}

func (cd *Downloader) DoCancel(vmImg *harvesterv1.VirtualMachineImage) error {
	imgName := cd.vmio.GetName(vmImg)
	imgNamespace := cd.vmio.GetNamespace(vmImg)

	if _, err := cd.vmImageDownloaderClient.Get(imgNamespace, imgName, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("ImageDownloader %s/%s is already gone, do no-op", imgNamespace, imgName)
			return nil
		}
		return fmt.Errorf("failed to get ImageDownloader %s/%s: %w", imgNamespace, imgName, err)
	}

	// Delete the Image downloader once the download is canceled
	if err := cd.vmImageDownloaderClient.Delete(imgNamespace, imgName, &metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("failed to delete ImageDownloader %s/%s: %w", imgNamespace, imgName, err)
	}

	return nil
}

func (cd *Downloader) endPointReady(targetURL string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel() // Make sure to cancel the context once the function is done

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
	if err != nil {
		logrus.Errorf("error creating HTTP request: %v", err)
		return false, nil
	}

	resp, err := cd.httpClient.Do(req)
	if err != nil {
		logrus.Errorf("error sending HTTP request: %v, try again.", err)
		return false, nil
	}
	defer resp.Body.Close()

	// Check if the status code is 200 OK
	logrus.Debugf("Current status code: %v, wanted: %v", resp.StatusCode, http.StatusOK)
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	return false, nil
}
