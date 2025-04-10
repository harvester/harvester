package backingimage

import (
	"fmt"
	"io"
	"net/http"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
)

type Downloader struct {
	biCache    ctllhv1.BackingImageCache
	httpClient http.Client
	vmio       common.VMIOperator
}

func GetDownloader(biCache ctllhv1.BackingImageCache, httpClient http.Client, vmio common.VMIOperator) backend.Downloader {
	return &Downloader{
		biCache:    biCache,
		httpClient: httpClient,
		vmio:       vmio,
	}
}

func (bid *Downloader) DoDownload(vmi *harvesterv1.VirtualMachineImage, rw http.ResponseWriter, req *http.Request) error {
	if !bid.vmio.IsImported(vmi) {
		return fmt.Errorf("please wait until the image has been imported")
	}
	if bid.vmio.IsEncryptOperation(vmi) {
		return fmt.Errorf("encrypted image is not supported for download")
	}

	biName, err := util.GetBackingImageName(bid.biCache, vmi)
	if err != nil {
		return fmt.Errorf("failed to get backing image name for VMImage %s/%s, error: %w", vmi.Namespace, vmi.Name, err)
	}

	targetFileName := fmt.Sprintf("%s.gz", bid.vmio.GetDisplayName(vmi))
	downloadURL := fmt.Sprintf("%s/backingimages/%s/download", util.LonghornDefaultManagerURL, biName)
	downloadReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create the download request with backing Image(%s): %w", biName, err)
	}

	downloadResp, err := bid.httpClient.Do(downloadReq)
	if err != nil {
		return fmt.Errorf("failed to send the download request with backing Image(%s): %w", biName, err)
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

func (bid *Downloader) DoCancel(_ *harvesterv1.VirtualMachineImage) error {
	// Cancel download is do no-op for backing image
	return nil
}
