package image

import (
	"fmt"
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
	actionUpload = "upload"
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

type UploadActionHandler struct {
	httpClient                  http.Client
	Images                      v1beta1.VirtualMachineImageClient
	ImageCache                  v1beta1.VirtualMachineImageCache
	BackingImageDataSources     ctllhv1beta1.BackingImageDataSourceClient
	BackingImageDataSourceCache ctllhv1beta1.BackingImageDataSourceCache
}

func (h UploadActionHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
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

func (h UploadActionHandler) do(rw http.ResponseWriter, req *http.Request) error {
	vars := mux.Vars(req)
	action := vars["action"]
	switch action {
	case actionUpload:
		return h.uploadImage(rw, req)
	default:
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
}

func (h UploadActionHandler) uploadImage(rw http.ResponseWriter, req *http.Request) error {
	vars := mux.Vars(req)
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

	uploadUrl := fmt.Sprintf("http://longhorn-backend.longhorn-system:9500/v1/backingimages/%s-%s", namespace, name)
	uploadReq, err := http.NewRequestWithContext(req.Context(), http.MethodPost, uploadUrl, req.Body)
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

func (h UploadActionHandler) waitForBackingImageDataSourceReady(name string) error {
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

func (h UploadActionHandler) updateImportedConditionOnConflict(image *apisv1beta1.VirtualMachineImage,
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
