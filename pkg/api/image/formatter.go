package image

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apisv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
)

const (
	actionUpload         = "upload"
	actionDownload       = "download"
	actionDownloadCancel = "downloadcancel"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string, 1)
	if request.AccessControl.CanUpdate(request, resource.APIObject, resource.Schema) != nil {
		return
	}

	sourceTypeStr := resource.APIObject.Data().String("spec", "sourceType")
	sourceType := apisv1beta1.VirtualMachineImageSourceType(sourceTypeStr)

	if sourceType == apisv1beta1.VirtualMachineImageSourceTypeUpload {
		resource.AddAction(request, actionUpload)
	}
}

type Handler struct {
	vmiClient   v1beta1.VirtualMachineImageClient
	vmio        common.VMIOperator
	downloaders map[apisv1beta1.VMIBackend]backend.Downloader
	uploaders   map[apisv1beta1.VMIBackend]backend.Uploader
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
	case actionDownloadCancel:
		return h.cancelDownloadImage(req)
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
	vmi, err := h.vmiClient.Get(namespace, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get VMImage with name(%s), ns(%s), error: %w", name, namespace, err)
	}

	return h.downloaders[util.GetVMIBackend(vmi)].DoDownload(vmi, rw, req)
}

func (h Handler) uploadImage(_ http.ResponseWriter, req *http.Request) error {
	vars := util.EncodeVars(mux.Vars(req))
	namespace := vars["namespace"]
	name := vars["name"]
	vmi, err := h.vmiClient.Get(namespace, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	return h.uploaders[util.GetVMIBackend(vmi)].DoUpload(vmi, req)
}

func (h Handler) cancelDownloadImage(req *http.Request) error {
	vars := util.EncodeVars(mux.Vars(req))
	namespace := vars["namespace"]
	name := vars["name"]
	vmi, err := h.vmiClient.Get(namespace, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get VMImage with name(%s), ns(%s), error: %w", name, namespace, err)
	}

	return h.downloaders[util.GetVMIBackend(vmi)].DoCancel(vmi)
}
