package image

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/minio/minio-go/v6"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	kapierror "k8s.io/apimachinery/pkg/api/errors"

	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/util"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = nil
}

func CollectionFormatter(request *types.APIRequest, collection *types.GenericCollection) {
	collection.AddAction(request, "upload")
}

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
	resource := req.FormValue("resource")
	if resource == "" {
		return apierror.NewAPIError(validation.MissingRequired, "resource is required")
	}
	image := &apisv1alpha1.VirtualMachineImage{}
	if err := json.Unmarshal([]byte(resource), image); err != nil {
		return err
	}
	if image.Spec.DisplayName == "" {
		return apierror.NewAPIError(validation.MissingRequired, "displayName is required")
	}
	if image.Namespace == "" {
		image.Namespace = h.Options.Namespace
	}
	file, fileHeader, err := req.FormFile("file")
	if err != nil {
		return errors.Wrap(err, "Failed reading image file from request")
	}
	defer file.Close()

	apisv1alpha1.ImageImported.Unknown(image)
	image, err = h.Images.Create(image)
	if err != nil {
		return err
	}
	imageName := image.Name

	fileName := fileHeader.Filename
	mc, err := util.NewMinioClient(h.Options)
	if err != nil {
		h.recordUploadError(image, err)
		return err
	}
	size, err := mc.PutObject(util.BucketName, imageName, file, fileHeader.Size, minio.PutObjectOptions{ContentType: fileHeader.Header.Get("Content-Type")})
	if err != nil {
		h.recordUploadError(image, err)
		return err
	}
	logrus.Debugf("Successfully uploaded %s of size %d\n", fileName, size)

	downloadURL := fmt.Sprintf("%s/%s/%s", h.Options.ImageStorageEndpoint, util.BucketName, imageName)
	image.Status.DownloadURL = downloadURL
	image.Status.Progress = 100
	image.Status.DownloadedBytes = size
	apisv1alpha1.ImageImported.True(image)
	return h.updateWithRetries(image)
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
