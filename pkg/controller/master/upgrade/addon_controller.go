package upgrade

import (
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

type addonHandler struct {
	namespace    string
	upgradeCache ctlharvesterv1.UpgradeCache
	addonClient  ctlharvesterv1.AddonClient
	addonCache   ctlharvesterv1.AddonCache
}

func (h *addonHandler) OnChanged(_ string, addon *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if addon == nil || addon.DeletionTimestamp != nil || addon.Name != util.DeschedulerName || addon.Labels == nil || !addon.Spec.Enabled {
		return addon, nil
	}

	upgradeControllerLock.Lock()
	defer upgradeControllerLock.Unlock()

	upgrade, err := h.upgradeCache.Get(upgradeNamespace, addon.Labels[harvesterUpgradeLabel])
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	if upgrade != nil && upgrade.Annotations != nil && upgrade.Annotations[reenableDeschedulerAddonAnnotation] == "true" {
		toUpdate := addon.DeepCopy()
		logrus.Infof("Disabling %s addon during upgrade", toUpdate.Name)
		toUpdate.Spec.Enabled = false
		if _, err := h.addonClient.Update(toUpdate); err != nil {
			return nil, err
		}
	}

	return addon, nil
}
