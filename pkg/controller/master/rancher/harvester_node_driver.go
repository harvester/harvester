package rancher

import (
	rancherv3api "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	harvesterDriverName = "harvester"
	driverURL           = "https://github.com/harvester/docker-machine-driver-harvester/releases/download/v0.1.0/docker-machine-driver-harvester-amd64.tar.gz"
	driverUIURL         = "https://github.com/harvester/ui-driver-harvester/releases/download/v0.1.0/component.js"
)

func (h *Handler) AddHarvesterNodeDriver() (*rancherv3api.NodeDriver, error) {
	driver, err := h.NodeDrivers.Get(harvesterDriverName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		logrus.Infof("Add default harvester node driver: %s", harvesterDriverName)
		harvesterDriver := &rancherv3api.NodeDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name: harvesterDriverName,
			},
			Spec: rancherv3api.NodeDriverSpec{
				DisplayName:      harvesterDriverName,
				Description:      "built-in harvester node driver",
				URL:              driverURL,
				UIURL:            driverUIURL,
				Builtin:          false,
				Active:           true,
				WhitelistDomains: []string{"github.com"},
			},
		}

		return h.NodeDrivers.Create(harvesterDriver)
	}

	if err != nil {
		return nil, err
	}

	if driver.Spec.URL == driverURL && driver.Spec.UIURL == driverUIURL {
		return nil, nil
	}

	updateCpy := driver.DeepCopy()
	updateCpy.Spec.URL = driverURL
	updateCpy.Spec.UIURL = driverUIURL
	return h.NodeDrivers.Update(updateCpy)
}
