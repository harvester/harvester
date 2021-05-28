package templateversion

import (
	"fmt"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	fieldTemplateID = "spec.templateId"
	fieldKeyPairIds = "spec.keyPairIds"
)

func NewValidator(templateCache ctlharvesterv1.VirtualMachineTemplateCache, templateVersionCache ctlharvesterv1.VirtualMachineTemplateVersionCache, keypairs ctlharvesterv1.KeyPairCache) types.Validator {
	return &templateVersionValidator{
		templateCache:        templateCache,
		templateVersionCache: templateVersionCache,
		keypairs:             keypairs,
	}
}

type templateVersionValidator struct {
	types.DefaultValidator

	templateCache        ctlharvesterv1.VirtualMachineTemplateCache
	templateVersionCache ctlharvesterv1.VirtualMachineTemplateVersionCache
	keypairs             ctlharvesterv1.KeyPairCache
}

func (v *templateVersionValidator) Resource() types.Resource {
	return newResource([]admissionregv1.OperationType{
		admissionregv1.Create,
		admissionregv1.Update,
		admissionregv1.Delete,
	})
}

func (v *templateVersionValidator) Create(request *types.Request, newObj runtime.Object) error {
	vmTemplVersion := newObj.(*v1beta1.VirtualMachineTemplateVersion)

	templateID := vmTemplVersion.Spec.TemplateID
	if templateID == "" {
		return werror.NewInvalidError("TemplateId is empty", fieldTemplateID)
	}

	templateNs, templateName := ref.Parse(templateID)
	if vmTemplVersion.Namespace != templateNs {
		return werror.NewInvalidError("Template version and template should reside in the same namespace", "metadata.namespace")
	}

	if _, err := v.templateCache.Get(templateNs, templateName); err != nil {
		return werror.NewInvalidError(err.Error(), fieldTemplateID)
	}

	keyPairIDs := vmTemplVersion.Spec.KeyPairIDs
	if len(keyPairIDs) > 0 {
		for i, kp := range keyPairIDs {
			keyPairNs, keyPairName := ref.Parse(kp)
			_, err := v.keypairs.Get(keyPairNs, keyPairName)
			if err != nil {
				message := fmt.Sprintf("KeyPairID %s is invalid, %v", v, err)
				field := fmt.Sprintf("%s[%d]", fieldKeyPairIds, i)
				return werror.NewInvalidError(message, field)
			}
		}
	}

	return nil
}

func (v *templateVersionValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	if request.IsFromController() {
		return nil
	}
	logrus.Infof("not allow for user %s", request.UserInfo.Username)
	return werror.NewMethodNotAllowed("Update templateVersion is not supported")
}

func (v *templateVersionValidator) Delete(request *types.Request, oldObj runtime.Object) error {
	// If a template is deleted, its versions are garbage collected.
	// No need to check for template existence or if a version is the default version or not.
	if request.IsGarbageCollection() {
		return nil
	}
	vmTemplVersion := oldObj.(*v1beta1.VirtualMachineTemplateVersion)
	version, err := v.templateVersionCache.Get(vmTemplVersion.Namespace, vmTemplVersion.Name)
	if err != nil {
		return err
	}

	templNs, templName := ref.Parse(version.Spec.TemplateID)
	vt, err := v.templateCache.Get(templNs, templName)
	if err != nil {
		return err
	}

	vresionID := ref.Construct(vmTemplVersion.Namespace, vmTemplVersion.Name)
	if vt.Spec.DefaultVersionID == vresionID {
		return werror.NewBadRequest("Cannot delete the default templateVersion")
	}

	return nil
}
