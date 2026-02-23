package networkattachmentdefinition

import (
	"fmt"

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

	for _, settingName := range []string{settings.StorageNetworkName, settings.RWXStorageNetworkSettingName} {
		usedBySetting, err := v.checkNadUsedBySetting(nad, settingName)
		if err != nil {
			return werror.NewInternalError(err.Error())
		}
		if usedBySetting {
			return werror.NewBadRequest(fmt.Sprintf("cannot delete NetworkAttachmentDefinition %s/%s which is used by Harvester setting '%s'", nad.Namespace, nad.Name, settingName))
		}
	}

	return nil
}

func (v *nadValidator) checkNadUsedBySetting(nad *cniv1.NetworkAttachmentDefinition, settingName string) (bool, error) {
	setting, err := v.settingCache.Get(settingName)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}
	if setting == nil {
		return false, nil
	}

	nadAnno := util.NadStorageNetworkAnnotation
	if settingName == settings.RWXStorageNetworkSettingName {
		nadAnno = util.RWXNadStorageNetworkAnnotation
	}
	nadNamespacedName := fmt.Sprintf("%s/%s", nad.Namespace, nad.Name)
	current := setting.Annotations[nadAnno]
	return current == nadNamespacedName, nil
}
