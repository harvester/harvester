package rancher

import (
	rancherv3api "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rancher/harvester/pkg/settings"
)

const (
	harvesterDriverName = "harvester"
	driverURL           = "https://harvester-node-driver.s3.amazonaws.com/driver/v0.1.4/docker-machine-driver-harvester-amd64.tar.gz"
	driverUIURL         = "https://harvester-node-driver.s3.amazonaws.com/ui/v0.1.4/component.js"
	serverVersionAnno   = "harvesterhci.io/serverVersion"
)

var whitelistDomains = []string{"harvester-node-driver.s3.amazonaws.com"}

func (h *Handler) AddHarvesterNodeDriver() (*rancherv3api.NodeDriver, error) {
	serverVersion := settings.ServerVersion.Get()
	driver, err := h.NodeDrivers.Get(harvesterDriverName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		logrus.Infof("Add default harvester node driver: %s", harvesterDriverName)
		harvesterDriver := &rancherv3api.NodeDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name: harvesterDriverName,
				Annotations: map[string]string{
					serverVersionAnno: serverVersion,
				},
			},
			Spec: rancherv3api.NodeDriverSpec{
				DisplayName:      harvesterDriverName,
				Description:      "built-in harvester node driver",
				URL:              driverURL,
				UIURL:            driverUIURL,
				Builtin:          false,
				Active:           true,
				WhitelistDomains: whitelistDomains,
			},
		}

		return h.NodeDrivers.Create(harvesterDriver)
	}

	if err != nil {
		return nil, err
	}

	if driver.Annotations[serverVersionAnno] == serverVersion || (driver.Spec.URL == driverURL && driver.Spec.UIURL == driverUIURL) {
		return nil, nil
	}

	updateCpy := driver.DeepCopy()
	updateCpy.Spec.URL = driverURL
	updateCpy.Spec.UIURL = driverUIURL
	return h.NodeDrivers.Update(updateCpy)
}
