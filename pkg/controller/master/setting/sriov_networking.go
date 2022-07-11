package setting

import (
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

// The sriovNetworkingChartManager function handles installation and removal of the
// sriov-network-operator Helm chart.
func (h *Handler) sriovNetworkingChartManager(setting *harvesterv1.Setting) error {
	if setting.Value == "true" {
		logrus.Infof("sriov-networking-enabled is set to true")
		//return installChart(h)
		return nil
	} else {
		logrus.Infof("sriov-networking-enabled is set to false")
		//return uninstallChart(h)
		return nil
	}
}

func installChart(h *Handler) error {
	ns := "harvester-system"
	name := "sriov-crd"
	opts := v1.GetOptions{}
	chart := &v3.ManagedChart{
		ObjectMeta: v1.ObjectMeta{
			Name:      "sriov-crd",
			Namespace: ns,
		},
		Spec: v3.ManagedChartSpec{
			Chart:           "sriov-crd",
			RepoName:        "rancher-charts",
			TargetNamespace: "cattle-sriov-system",
		},
	}
	_, err := h.managedCharts.Get(ns, name, opts)
	if err == nil {
		_, err = h.managedCharts.Create(chart)
		if err != nil {
			logrus.Infof("sriov managed chart already created")
			return nil
		}
		return nil
	} else {
		return err
	}
}

func uninstallChart(h *Handler) error {
	ns := "harvester-system"
	name := "sriov-crd"
	opts := v1.GetOptions{}
	_, err := h.managedCharts.Get(ns, name, opts)
	if err == nil {
		err = h.managedCharts.Delete(ns, name, &v1.DeleteOptions{})
		if err != nil {
			logrus.Infof("sriov managed chart already deleted")
			return nil
		}
		return nil
	} else {
		return err
	}
}
