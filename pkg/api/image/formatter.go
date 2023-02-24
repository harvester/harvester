package image

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"github.com/gorilla/mux"
	lhv1beta1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apisv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllhv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	actionUpload   = "upload"
	actionDownload = "download"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string, 1)
	if request.AccessControl.CanUpdate(request, resource.APIObject, resource.Schema) != nil {
		return
	}

	if resource.APIObject.Data().String("spec", "sourceType") == apisv1beta1.VirtualMachineImageSourceTypeUpload {
		resource.AddAction(request, actionUpload)
	}
}

type Handler struct {
	httpClient                  http.Client
	Images                      v1beta1.VirtualMachineImageClient
	ImageCache                  v1beta1.VirtualMachineImageCache
	BackingImageDataSources     ctllhv1beta1.BackingImageDataSourceClient
	BackingImageDataSourceCache ctllhv1beta1.BackingImageDataSourceCache
}

func (h Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if err := h.do(rw, req); err != nil {
		status := http.StatusInternalServerError
		if e, ok := err.(*apierror.APIError); ok {
			status = e.Code.Status
		}
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}
	rw.WriteHeader(http.StatusOK)
}

func (h Handler) do(rw http.ResponseWriter, req *http.Request) error {
	vars := util.EncodeVars(mux.Vars(req))
	if req.Method == http.MethodGet {
		return h.doGet(vars["link"], rw, req)
	} else if req.Method == http.MethodPost {
		return h.doPost(vars["action"], rw, req)
	}

	return apierror.NewAPIError(validation.InvalidAction, fmt.Sprintf("Unsupported method %s", req.Method))
}

func (h Handler) doGet(link string, rw http.ResponseWriter, req *http.Request) error {
	switch link {
	case actionDownload:
		return h.downloadImage(rw, req)
	default:
		return apierror.NewAPIError(validation.InvalidAction, fmt.Sprintf("Unsupported GET action %s", link))
	}
}

func (h Handler) doPost(action string, rw http.ResponseWriter, req *http.Request) error {
	switch action {
	case actionUpload:
		return h.uploadImage(rw, req)
	default:
		return apierror.NewAPIError(validation.InvalidAction, fmt.Sprintf("Unsupported POST action %s", action))
	}
}

func (h Handler) downloadImage(rw http.ResponseWriter, req *http.Request) error {
	vars := util.EncodeVars(mux.Vars(req))
	namespace := vars["namespace"]
	name := vars["name"]
	vmImage, err := h.Images.Get(namespace, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get VMImage with name(%s), ns(%s), error: %w", name, namespace, err)
	}

	targetFileName := vmImage.Spec.DisplayName
	bkimgName := fmt.Sprintf("%s-%s", namespace, name)
	downloadURL := fmt.Sprintf("%s/backingimages/%s/download", util.LonghornDefaultManagerURL, bkimgName)
	downloadReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create the download request with backing Image(%s): %w", bkimgName, err)
	}

	downloadResp, err := h.httpClient.Do(downloadReq)
	if err != nil {
		return fmt.Errorf("failed to send the download request with backing Image(%s): %w", bkimgName, err)
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed with unexpected http Status code %d", downloadResp.StatusCode)
	}

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

func (h Handler) uploadImage(rw http.ResponseWriter, req *http.Request) error {
	vars := util.EncodeVars(mux.Vars(req))
	namespace := vars["namespace"]
	name := vars["name"]
	image, err := h.Images.Get(namespace, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			if updateErr := h.updateImportedConditionOnConflict(image, "False", "UploadFailed", err.Error()); updateErr != nil {
				logrus.Error(err)
			}
		}
	}()

	//Wait for backing image data source to be ready. Otherwise the upload request will fail.
	dsName := fmt.Sprintf("%s-%s", namespace, name)
	if err := h.waitForBackingImageDataSourceReady(dsName); err != nil {
		return err
	}

	uploadURL := fmt.Sprintf("%s/backingimages/%s-%s", util.LonghornDefaultManagerURL, namespace, name)
	uploadReq, err := http.NewRequestWithContext(req.Context(), http.MethodPost, uploadURL, req.Body)
	if err != nil {
		return fmt.Errorf("failed to create the upload request: %w", err)
	}
	uploadReq.Header = req.Header
	uploadReq.URL.RawQuery = req.URL.RawQuery

	var urlErr *url.Error
	uploadResp, err := h.httpClient.Do(uploadReq)
	if errors.As(err, &urlErr) {
		// Trim the "POST http://xxx" implementation detail for the error
		// set the err var and it will be recorded in image condition in the defer function
		err = errors.Unwrap(urlErr)
		return err
	} else if err != nil {
		return fmt.Errorf("failed to send the upload request: %w", err)
	}
	defer uploadResp.Body.Close()

	body, err := ioutil.ReadAll(uploadResp.Body)
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

func (h Handler) waitForBackingImageDataSourceReady(name string) error {
	retry := 30
	for i := 0; i < retry; i++ {
		ds, err := h.BackingImageDataSources.Get(util.LonghornSystemNamespaceName, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed waiting for backing image data source to be ready: %w", err)
		}
		if err == nil {
			if ds.Status.CurrentState == lhv1beta1.BackingImageStatePending {
				return nil
			}
			if ds.Status.CurrentState == lhv1beta1.BackingImageStateFailed {
				return errors.New(ds.Status.Message)
			}
		}
		time.Sleep(2 * time.Second)
	}
	return errors.New("timeout waiting for backing image data source to be ready")
}

func (h Handler) updateImportedConditionOnConflict(image *apisv1beta1.VirtualMachineImage,
	status, reason, message string) error {
	retry := 3
	for i := 0; i < retry; i++ {
		current, err := h.ImageCache.Get(image.Namespace, image.Name)
		if err != nil {
			return err
		}
		if current.DeletionTimestamp != nil {
			return nil
		}
		toUpdate := current.DeepCopy()
		apisv1beta1.ImageImported.SetStatus(toUpdate, status)
		apisv1beta1.ImageImported.Reason(toUpdate, reason)
		apisv1beta1.ImageImported.Message(toUpdate, message)
		if reflect.DeepEqual(current, toUpdate) {
			return nil
		}
		_, err = h.Images.Update(toUpdate)
		if err == nil || !apierrors.IsConflict(err) {
			return err
		}
		time.Sleep(2 * time.Second)
	}
	return errors.New("failed to update image uploaded condition, max retries exceeded")
}
