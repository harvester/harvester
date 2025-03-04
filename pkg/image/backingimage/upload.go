package backingimage

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
)

type Uploader struct {
	biCache    ctllhv1.BackingImageCache
	bidsClient ctllhv1.BackingImageDataSourceClient
	httpClient http.Client
	vmio       common.VMIOperator
}

func GetUploader(biCache ctllhv1.BackingImageCache,
	bidsClient ctllhv1.BackingImageDataSourceClient,
	httpClient http.Client,
	vmio common.VMIOperator) backend.Uploader {
	return &Uploader{
		biCache:    biCache,
		bidsClient: bidsClient,
		httpClient: httpClient,
		vmio:       vmio,
	}
}

func (biu *Uploader) waitForBackingImageDataSourceReady(name string) error {
	retry := 30
	for i := 0; i < retry; i++ {
		ds, err := biu.bidsClient.Get(util.LonghornSystemNamespaceName, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed waiting for backing image data source to be ready: %w", err)
		}
		if err == nil {
			if ds.Status.CurrentState == lhv1beta2.BackingImageStatePending {
				return nil
			}
			if ds.Status.CurrentState == lhv1beta2.BackingImageStateFailed {
				return errors.New(ds.Status.Message)
			}
		}
		time.Sleep(2 * time.Second)
	}
	return errors.New("timeout waiting for backing image data source to be ready")
}

func (biu *Uploader) DoUpload(vmi *harvesterv1.VirtualMachineImage, req *http.Request) error {
	var err error
	defer func() {
		if err != nil {
			if updateErr := biu.vmio.FailUpload(vmi, err.Error()); updateErr != nil {
				logrus.Error(err)
			}
		}
	}()

	// Wait for backing image data source to be ready. Otherwise the upload request will fail.
	dsName, err := util.GetBackingImageDataSourceName(biu.biCache, vmi)
	if err != nil {
		return fmt.Errorf("failed to get backing image name for VMImage %s/%s, error: %w", vmi.Namespace, vmi.Name, err)
	}

	if err := biu.waitForBackingImageDataSourceReady(dsName); err != nil {
		return err
	}

	uploadURL := fmt.Sprintf("%s/backingimages/%s", util.LonghornDefaultManagerURL, dsName)
	uploadReq, err := http.NewRequestWithContext(req.Context(), http.MethodPost, uploadURL, req.Body)
	if err != nil {
		return fmt.Errorf("failed to create the upload request: %w", err)
	}
	uploadReq.Header = req.Header
	uploadReq.URL.RawQuery = req.URL.RawQuery

	var urlErr *url.Error
	uploadResp, err := biu.httpClient.Do(uploadReq)
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

	return nil
}
