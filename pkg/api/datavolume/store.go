package datavolume

import (
	"fmt"
	"strings"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	kv1 "kubevirt.io/client-go/api/v1"

	cdiv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
)

type dvStore struct {
	types.Store
	dvCache cdiv1beta1.DataVolumeCache
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

	annotationSchemaOwners, err := ref.GetSchemaOwnersFromAnnotation(dv)
	if err != nil {
		return fmt.Errorf("failed to get schema owners from annotation: %w", err)
	}

	attachedList := annotationSchemaOwners.List(kv1.VirtualMachineGroupVersionKind.GroupKind())
	if len(attachedList) != 0 {
		return fmt.Errorf("can not delete the volume %s which is currently attached by these VMs: %s", name, strings.Join(attachedList, ","))
	}

	if len(dv.OwnerReferences) == 0 {
		return nil
	}

	var ownerList []string
	for _, owner := range dv.OwnerReferences {
		if owner.Kind == kv1.VirtualMachineGroupVersionKind.Kind {
			ownerList = append(ownerList, owner.Name)
		}
	}

	if len(ownerList) != 0 {
		return fmt.Errorf("can not delete the volume %s which is currently owned by these VMs: %s", name, strings.Join(ownerList, ","))
	}

	return nil
}
