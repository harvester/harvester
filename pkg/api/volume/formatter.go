package volume

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/api/vm"
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

const (
	actionExport = "export"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string, 1)
	resource.AddAction(request, actionExport)
}

type ExportActionHandler struct {
	images v1beta1.VirtualMachineImageClient
}

func (h ExportActionHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
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
	_, _ = rw.Write([]byte("Successfully export the volume."))
}

func (h ExportActionHandler) do(rw http.ResponseWriter, r *http.Request) error {
	vars := mux.Vars(r)
	action := vars["action"]
	pvcName := vars["name"]
	imageNamespace := vars["namespace"]

	switch action {
	case actionExport:
		var input vm.ExportVolumeInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.DisplayName == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `displayName` is required")
		}
		if input.Namespace == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `namespace` is required")
		}
		return h.exportVolume(r.Context(), input.Namespace, pvcName, input.DisplayName, imageNamespace)
	default:
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
}

func (h ExportActionHandler) exportVolume(ctx context.Context, pvcNamespace, pvcName, imageDisplayName, imageNamespace string) error {
	vmImage := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "image-",
			Namespace:    imageNamespace,
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			DisplayName:  imageDisplayName,
			SourceType:   harvesterv1.VirtualMachineImageSourceTypeExportVolume,
			PVCName:      pvcName,
			PVCNamespace: pvcNamespace,
		},
	}
	_, err := h.images.Create(vmImage)
	if err != nil {
		logrus.Errorf("Unable to create image from volume %s", pvcName)
		return err
	}
	return nil
}
