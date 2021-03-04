package image

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	kapierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	imagectl "github.com/rancher/harvester/pkg/controller/master/image"
	"github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
)

const (
	actionUpload   = "upload"
	fileFormName   = "file"
	fileSizeHeader = "File-Size"
)

type UploadActionHandler struct {
	Options    config.Options
	Images     v1alpha1.VirtualMachineImageClient
	ImageCache v1alpha1.VirtualMachineImageCache
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
		return h.uploadImage(req)
	default:
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
}

func (h UploadActionHandler) uploadImage(req *http.Request) error {
	vars := mux.Vars(req)
	namespace := vars["namespace"]
	name := vars["name"]
	image, err := h.Images.Get(namespace, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if image.Spec.URL != "" {
		return apierror.NewAPIError(validation.InvalidAction, "Cannot upload image with URL set")
	}

	apisv1alpha1.ImageImported.Unknown(image)
	if err := h.updateWithRetries(image); err != nil {
		return err
	}

	mr, err := req.MultipartReader()
	if err != nil {
		h.recordUploadError(image, err)
		return errors.Wrap(err, "Failed reading file from request")
	}

	part, err := mr.NextPart()
	if err != nil {
		h.recordUploadError(image, err)
		return errors.Wrap(err, "Failed reading file from request")
	}
	defer part.Close()

	if part.FormName() != fileFormName {
		return errors.New("Expected file form in request")
	}

	size := int64(-1)
	sizeHeader := req.Header.Get(fileSizeHeader)
	if sizeHeader != "" {
		size, err = strconv.ParseInt(sizeHeader, 10, 64)
		if err != nil {
			h.recordUploadError(image, err)
			return err
		}
	}

	importer := imagectl.MinioImporter{
		Images:     h.Images,
		ImageCache: h.ImageCache,
		Options:    h.Options,
	}

	ctx, cancel := context.WithCancel(req.Context())
	if err := importer.ImportImageToMinio(ctx, cancel, part, image, size); err != nil {
		h.recordUploadError(image, err)
		return err
	}
	return nil
}

func (h UploadActionHandler) recordUploadError(image *apisv1alpha1.VirtualMachineImage, err error) {
	apisv1alpha1.ImageImported.False(image)
	apisv1alpha1.ImageImported.Message(image, err.Error())
	if err := h.updateWithRetries(image); err != nil {
		logrus.Errorf("Failed to update image: %v", err)
	}
}

func (h UploadActionHandler) updateWithRetries(image *apisv1alpha1.VirtualMachineImage) error {
	const retries = 3
	for i := 0; i < retries; {
		currentImage, err := h.ImageCache.Get(image.Namespace, image.Name)
		if err != nil {
			return err
		}
		toUpdate := currentImage.DeepCopy()
		toUpdate.Spec = image.Spec
		toUpdate.Status = image.Status
		if _, err := h.Images.Update(toUpdate); err == nil {
			return nil
		} else if err != nil && !kapierror.IsConflict(err) {
			return err
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("exceeded maximum retries for image update")
}
