package networkattachmentdefinition

import (
	"fmt"

	hncutils "github.com/harvester/harvester-network-controller/pkg/utils"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(settingCache ctlharvesterv1.SettingCache) types.Validator {
	return &nadValidator{
		settingCache: settingCache,
	}
}

type nadValidator struct {
	types.DefaultValidator
	settingCache ctlharvesterv1.SettingCache
}

func (v *nadValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"network-attachment-definitions"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   cniv1.SchemeGroupVersion.Group,
		APIVersion: cniv1.SchemeGroupVersion.Version,
		ObjectType: &cniv1.NetworkAttachmentDefinition{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Delete,
		},
	}
}

func (v *nadValidator) Delete(_ *types.Request, obj runtime.Object) error {
	nad := obj.(*cniv1.NetworkAttachmentDefinition)
	if nad == nil {
		return nil
	}

	// Skip NADs that are not used for storage network.
	// A resource is considered as a storage network NAD if it meets all the
	// following conditions:
	// 1. The namespace is `harvester-system`
	// 2. It has the annotation `storage-network.settings.harvesterhci.io: true`
	// 3. The name has the prefix `storagenetwork-`
	if !hncutils.IsStorageNetworkNad(nad) {
		return nil
	}

	// Get the `storage-network` setting to check if it uses the deleting NAD.
	setting, err := v.settingCache.Get(settings.StorageNetworkName)
	if err != nil && apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return werror.NewInternalError(err.Error())
	}
	if setting == nil {
		return nil
	}

	nadNamespacedName := fmt.Sprintf("%s/%s", nad.Namespace, nad.Name)
	current := setting.Annotations[util.NadStorageNetworkAnnotation]
	if current == nadNamespacedName {
		return werror.NewBadRequest(fmt.Sprintf("cannot delete NetworkAttachmentDefinition %s which is used by Harvester setting '%s'", nadNamespacedName, settings.StorageNetworkName))
	}

	return nil
}
