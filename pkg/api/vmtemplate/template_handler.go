package vmtemplate

import (
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/handlers"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/harvester/pkg/controller/master/template"
	ctlvmv1alpha1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
)

type templateLinkHandler struct {
	templateVersionCache ctlvmv1alpha1.VirtualMachineTemplateVersionCache
}

func (h *templateLinkHandler) byIDHandler(request *types.APIRequest) (types.APIObject, error) {
	if request.Link == "versions" {
		versions, err := h.getVersions(request.Namespace, request.Name)
		if err != nil {
			return types.APIObject{}, err
		}

		request.ResponseWriter.WriteList(request, http.StatusOK, versions)
	}

	return handlers.ByIDHandler(request)
}

func (h *templateLinkHandler) getVersions(templateNs, templateName string) (types.APIObjectList, error) {
	sets := labels.Set{
		template.TemplateLabel: templateName,
	}
	versions, err := h.templateVersionCache.List(templateNs, sets.AsSelector())
	if err != nil {
		return types.APIObjectList{}, apierror.NewAPIError(validation.ServerError, err.Error())
	}

	var result []types.APIObject
	for _, vtr := range versions {
		id := fmt.Sprintf("%s/%s", vtr.Namespace, vtr.Name)
		result = append(result, types.APIObject{
			Type:   templateVersionSchemaID,
			ID:     id,
			Object: vtr,
		})
	}

	return types.APIObjectList{
		Objects: result,
	}, nil
}
