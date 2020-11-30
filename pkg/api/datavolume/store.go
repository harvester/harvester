package datavolume

import (
	"fmt"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	kv1alpha3 "kubevirt.io/client-go/api/v1alpha3"

	cdiv1beta1 "github.com/rancher/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/rancher/harvester/pkg/util"
)

type dvStore struct {
	types.Store
	dvCache cdiv1beta1.DataVolumeCache
}

func (s *dvStore) Create(request *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	util.SetHTTPSourceDataVolume(data.Data())
	return s.Store.Create(request, request.Schema, data)
}

func (s *dvStore) Update(request *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (types.APIObject, error) {
	util.SetHTTPSourceDataVolume(data.Data())
	return s.Store.Update(request, request.Schema, data, id)
}

func (s *dvStore) Delete(request *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	if err := s.canDelete(request.Namespace, request.Name); err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, err.Error())
	}
	return s.Store.Delete(request, request.Schema, id)
}

func (s *dvStore) canDelete(namespace, name string) error {
	dv, err := s.dvCache.Get(namespace, name)
	if err != nil {
		return fmt.Errorf("failed to get dv %s, %v", name, err)
	}

	if len(dv.OwnerReferences) != 0 {
		for _, owner := range dv.OwnerReferences {
			if owner.Kind == kv1alpha3.VirtualMachineGroupVersionKind.Kind {
				return fmt.Errorf("can not delete the volume %s which is currently owned by the VM %s", name, owner.Name)
			}
		}
	}

	return nil
}
