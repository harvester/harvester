package guestcluster

import (
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

type GuestClusterController struct {
	guestClusterClient ctlharvesterv1.GuestClusterClient
	guestClusterCache  ctlharvesterv1.GuestClusterCache

	settingCache ctlharvesterv1.SettingCache
}

func (h *GuestClusterController) OnChange(_ string, gc *harvesterv1.GuestCluster) (*harvesterv1.GuestCluster, error) {
	if gc == nil || gc.DeletionTimestamp != nil {
		return nil, nil
	}

	logrus.Infof("detect guest cluster: %s/%s", gc.Namespace, gc.Name)
	// detect the current status, update self

	return gc, nil
}
