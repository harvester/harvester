package image

import (
	"fmt"
	"net/http"

	"github.com/minio/minio-go/v6"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/util"
	"github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	common.Formatter(request, resource)
	resource.Actions = nil
}

func CollectionFormatter(request *types.APIRequest, collection *types.GenericCollection) {
	collection.AddAction(request, "upload")
}

type UploadActionHandler struct {
	Images     v1alpha1.VirtualMachineImageClient
	ImageCache v1alpha1.VirtualMachineImageCache
}

func (h UploadActionHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if err := h.do(rw, req); err != nil {
		if e, ok := err.(*apierror.APIError); ok {
			rw.WriteHeader(e.Code.Status)
		} else {
			rw.WriteHeader(http.StatusInternalServerError)
		}
		rw.Write([]byte(err.Error()))
	} else {
		rw.WriteHeader(http.StatusOK)
	}
}

func (h UploadActionHandler) do(rw http.ResponseWriter, req *http.Request) error {
	imageName := req.FormValue("displayName")
	if imageName == "" {
		return apierror.NewAPIError(validation.MissingRequired, "displayName is required")
	}
	namespace := req.FormValue("namespace")
	if namespace == "" {
		namespace = config.Namespace
	}
	file, fileHeader, err := req.FormFile("file")
	if err != nil {
		return errors.Wrap(err, "Failed reading image file from request")
	}
	defer file.Close()

	fileName := fileHeader.Filename
	generatedName := fmt.Sprintf("%s-%s", "image", rand.String(5))
	mc, err := util.NewMinioClient()
	if err != nil {
		return err
	}
	n, err := mc.PutObject(util.BucketName, generatedName, file, fileHeader.Size, minio.PutObjectOptions{ContentType: fileHeader.Header.Get("Content-Type")})
	if err != nil {
		return err
	}
	logrus.Debugf("Successfully uploaded %s of size %d\n", fileName, n)

	downloadURL := fmt.Sprintf("%s/%s/%s", config.ImageStorageEndpoint, util.BucketName, generatedName)
	image := &apisv1alpha1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{
			Name:      generatedName,
			Namespace: namespace,
		},
		Spec: apisv1alpha1.VirtualMachineImageSpec{
			DisplayName: imageName,
		},
		Status: apisv1alpha1.VirtualMachineImageStatus{
			DownloadURL: downloadURL,
			Progress:    100,
		},
	}
	apisv1alpha1.ImageImported.True(image)
	apisv1alpha1.ImageImported.Message(image, "uploaded by user")
	_, err = h.Images.Create(image)
	return err
}
