package setting

import (
	"encoding/json"
	"errors"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

// The sriovNetworkingChartManager function handles installation and removal of the sriov-network-operator Helm chart.
func (h *Handler) sriovNetworkingChartManager(setting *harvesterv1.Setting) error {
	ns := "harvester-system"
	name := "sriov-crd"
	opts := v1.GetOptions{}
	chart := &v3.ManagedChart{
		ObjectMeta: v1.ObjectMeta{
			Name: "sriov-crd",
			Namespace: ns,
		},
		Spec: v3.ManagedChartSpec{
			Chart: "sriov-crd",
			RepoName: "rancher-charts",
			TargetNamespace: "cattle-sriov-system",
		},
	}
	managedChart, err := h.managedCharts.Get(ns, name, opts)
	if err != nil {
		managedChart, err = h.managedCharts.Create(chart)
		if err != nil {
			return err
		}
	}
	s, err := json.Marshal(managedChart)
	logrus.Infof("===== %s", string(s))
	if err != nil {
		return err
	}
	return errors.New(string(s))
}
