package restore

import (
	"fmt"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	ctlkubevirtv1 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

type vmRestoreStore struct {
	types.Store
	vmCache ctlkubevirtv1.VirtualMachineCache
}

func (s *vmRestoreStore) Create(request *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	objData := data.Data()
	targetVM := objData.Map("spec", "target").String("name")
	backupName := objData.Map("spec").String("virtualMachineBackupName")
	newVM := objData.Map("spec").Bool("newVM")

	if targetVM == "" || backupName == "" {
		return types.APIObject{}, apierror.NewAPIError(validation.InvalidBodyContent, "either VM name or backup name is empty")
	}

	vm, err := s.vmCache.Get(request.Namespace, targetVM)
	if err != nil {
		if newVM && apierrors.IsNotFound(err) {
			return s.Store.Create(request, schema, data)
		}
		return types.APIObject{}, apierror.NewAPIError(validation.InvalidBodyContent, err.Error())
	}

	// restore a new vm but the vm is already exist
	if newVM && vm != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.InvalidBodyContent, fmt.Sprintf("VM %s is already exists", vm.Name))
	}

	// restore an existing vm but the vm is still running
	if !newVM && vm.Status.Ready {
		return types.APIObject{}, apierror.NewAPIError(validation.InvalidBodyContent, "please stop the VM before doing a restore")
	}

	return s.Store.Create(request, schema, data)
}
