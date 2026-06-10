package upgradelog

import (
	"fmt"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(
	upgrades ctlharvesterv1.UpgradeCache,
	upgradeLogs ctlharvesterv1.UpgradeLogCache,
) types.Validator {
	return &upgradeLogValidator{
		upgrades:    upgrades,
		upgradeLogs: upgradeLogs,
	}
}

type upgradeLogValidator struct {
	types.DefaultValidator
	upgrades    ctlharvesterv1.UpgradeCache
	upgradeLogs ctlharvesterv1.UpgradeLogCache
}

func (v *upgradeLogValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.UpgradeLogResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.UpgradeLog{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *upgradeLogValidator) Create(req *types.Request, newObj runtime.Object) error {
	newUpgradeLog := newObj.(*v1beta1.UpgradeLog)
	if newUpgradeLog == nil {
		return nil
	}

	if req == nil || !req.IsFromController() {
		return werror.NewBadRequest("upgradelog can only be created by the harvester controller")
	}

	upgradeName := newUpgradeLog.Spec.UpgradeName
	logrus.Debugf("Validating UpgradeLog creation for upgrade: %s", upgradeName)

	if upgradeName == "" {
		return werror.NewBadRequest("upgradeName field is not specified")
	}

	if newUpgradeLog.Namespace != util.HarvesterSystemNamespaceName {
		return werror.NewBadRequest(fmt.Sprintf(
			"UpgradeLog must be created in namespace %s",
			util.HarvesterSystemNamespaceName))
	}

	upgrade, err := v.upgrades.Get(util.HarvesterSystemNamespaceName, upgradeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return werror.NewBadRequest(fmt.Sprintf("referenced upgrade %s not found in namespace %s",
				upgradeName, util.HarvesterSystemNamespaceName))
		}
		return werror.NewInternalError(fmt.Sprintf("failed to get upgrade %s: %v", upgradeName, err))
	}

	if !upgrade.Spec.LogEnabled {
		return fmt.Errorf("LogEnabled is false for upgrade %s", upgradeName)
	}

	if v1beta1.UpgradeCompleted.GetStatus(upgrade) != "" && !v1beta1.LogReady.IsUnknown(upgrade) {
		return fmt.Errorf("cannot create upgradeLog: upgrade %s is not initializing or waiting for logging infrastructure", upgradeName)
	}

	existingUpgradeLogs, err := v.upgradeLogs.List(util.HarvesterSystemNamespaceName, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to list existing upgradelogs: %v", err))
	}
	for _, existingUpgradeLog := range existingUpgradeLogs {
		if existingUpgradeLog.Name == newUpgradeLog.Name {
			continue
		}
		if existingUpgradeLog.Spec.UpgradeName == upgradeName {
			return werror.NewBadRequest(fmt.Sprintf(
				"upgradelog %s already exists for upgrade %s",
				existingUpgradeLog.Name, upgradeName))
		}
	}

	logrus.Debugf("UpgradeLog validation passed for upgrade %s", upgradeName)
	return nil
}

func (v *upgradeLogValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldUpgradeLog := oldObj.(*v1beta1.UpgradeLog)
	newUpgradeLog := newObj.(*v1beta1.UpgradeLog)

	if oldUpgradeLog.Spec.UpgradeName != newUpgradeLog.Spec.UpgradeName {
		return werror.NewBadRequest("spec.upgradeName is immutable and cannot be changed")
	}

	return nil
}
