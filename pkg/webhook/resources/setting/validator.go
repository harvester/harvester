package setting

import (
	"encoding/json"
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	settingctl "github.com/harvester/harvester/pkg/controller/master/setting"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	httpProxySettingName        = "http-proxy"
	overcommitConfigSettingName = "overcommit-config"
	vipPoolsConfigSettingName   = "vip-pools"
)

type validateSettingFunc func(setting *v1beta1.Setting) error

var validateSettingFuncs = map[string]validateSettingFunc{
	httpProxySettingName:                      validateHTTPProxy,
	settings.VMForceDeletionPolicySettingName: validateVMForceDeletionPolicy,
	overcommitConfigSettingName:               validateOvercommitConfig,
	vipPoolsConfigSettingName:                 validateVipPoolsConfig,
}

func NewValidator() types.Validator {
	return &settingValidator{}
}

type settingValidator struct {
	types.DefaultValidator
}

func (v *settingValidator) Resource() types.Resource {
	return types.Resource{
		Name:       v1beta1.SettingResourceName,
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.Setting{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *settingValidator) Create(request *types.Request, newObj runtime.Object) error {
	return validateSetting(newObj)
}

func (v *settingValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	return validateSetting(newObj)
}

func validateSetting(newObj runtime.Object) error {
	setting := newObj.(*v1beta1.Setting)

	if validateFunc, ok := validateSettingFuncs[setting.Name]; ok {
		return validateFunc(setting)
	}

	return nil
}

func validateHTTPProxy(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(setting.Value), &util.HTTPProxyConfig{}); err != nil {
		message := fmt.Sprintf("failed to unmarshal the setting value, %v", err)
		return werror.NewInvalidError(message, "value")
	}
	return nil
}

func validateOvercommitConfig(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}
	overcommit := &settings.Overcommit{}
	if err := json.Unmarshal([]byte(setting.Value), overcommit); err != nil {
		return werror.NewInvalidError(fmt.Sprintf("Invalid JSON: %s", setting.Value), "Value")
	}
	emit := func(percentage int, field string) error {
		msg := fmt.Sprintf("Cannot undercommit. Should be greater than or equal to 100 but got %d", percentage)
		return werror.NewInvalidError(msg, field)
	}
	if overcommit.Cpu < 100 {
		return emit(overcommit.Cpu, "cpu")
	}
	if overcommit.Memory < 100 {
		return emit(overcommit.Memory, "memory")
	}
	if overcommit.Storage < 100 {
		return emit(overcommit.Storage, "storage")
	}
	return nil
}

func validateVMForceDeletionPolicy(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	if _, err := settings.DecodeVMForceDeletionPolicy(setting.Value); err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	return nil
}

func validateVipPoolsConfig(setting *v1beta1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	pools := map[string]string{}
	err := json.Unmarshal([]byte(setting.Value), &pools)
	if err != nil {
		return err
	}

	if err := settingctl.ValidateCIDRs(pools); err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	return nil
}
