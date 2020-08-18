package vmtemplate

import (
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	ctlvmv1alpha1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/ref"
	"github.com/rancher/wrangler/pkg/schemas/validation"
)

type templateStore struct {
	types.Store
	templateVersionCache ctlvmv1alpha1.VirtualMachineTemplateVersionCache
}

func (s *templateStore) Update(request *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (types.APIObject, error) {
	defaultVersionID := data.Data().String("spec", "defaultVersionId")
	if defaultVersionID != "" {
		ns, name := ref.Parse(defaultVersionID)
		defaultVersion, err := s.templateVersionCache.Get(ns, name)
		if err != nil {
			return types.APIObject{}, apierror.NewAPIError(validation.ServerError, err.Error())
		}
		data.Data().SetNested(defaultVersion.Status.Version, "status", "defaultVersion")
	}
	return s.Store.Update(request, request.Schema, data, id)
}
