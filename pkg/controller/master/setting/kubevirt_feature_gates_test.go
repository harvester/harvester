package setting

import (
	"context"
	"testing"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	mgmtv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func newHarvesterManagedChart(paused bool, kubeVirtValues map[string]interface{}) *mgmtv3.ManagedChart {
	data := map[string]interface{}{
		"harvester-node-disk-manager": map[string]interface{}{"enabled": true},
	}
	if kubeVirtValues != nil {
		data[kubeVirtValuesKey] = kubeVirtValues
	}
	return &mgmtv3.ManagedChart{
		ObjectMeta: metav1.ObjectMeta{Namespace: ManagedChartNamespace, Name: HarvesterManagedChartName},
		Spec: mgmtv3.ManagedChartSpec{
			Paused: paused,
			Values: &fleet.GenericMap{Data: data},
		},
	}
}

func TestSyncKubeVirtFeatureGates(t *testing.T) {
	monitorSpec := map[string]interface{}{"monitorAccount": "rancher-monitoring-operator"}

	tests := []struct {
		name          string
		value         string
		chart         *mgmtv3.ManagedChart
		expectErr     string
		expectGates   []interface{}
		expectNoKey   bool
		expectUpdated bool
	}{
		{
			name:          "add gates and keep existing kubevirt values",
			value:         "VSOCK,Snapshot",
			chart:         newHarvesterManagedChart(false, map[string]interface{}{"spec": monitorSpec}),
			expectGates:   []interface{}{"VSOCK", "Snapshot"},
			expectUpdated: true,
		},
		{
			name:          "add gates when kubevirt values are missing",
			value:         "VSOCK",
			chart:         newHarvesterManagedChart(false, nil),
			expectGates:   []interface{}{"VSOCK"},
			expectUpdated: true,
		},
		{
			name:  "same gates do not update the ManagedChart",
			value: "VSOCK",
			chart: newHarvesterManagedChart(false, map[string]interface{}{
				"spec": monitorSpec, kubeVirtAdditionalFeatureGatesKey: []interface{}{"VSOCK"},
			}),
			expectGates: []interface{}{"VSOCK"},
		},
		{
			name:  "empty value removes the key",
			value: "",
			chart: newHarvesterManagedChart(false, map[string]interface{}{
				"spec": monitorSpec, kubeVirtAdditionalFeatureGatesKey: []interface{}{"VSOCK"},
			}),
			expectNoKey:   true,
			expectUpdated: true,
		},
		{
			name:        "empty value without the key is a no-op",
			value:       "",
			chart:       newHarvesterManagedChart(false, map[string]interface{}{"spec": monitorSpec}),
			expectNoKey: true,
		},
		{
			name:      "paused ManagedChart is retried",
			value:     "VSOCK",
			chart:     newHarvesterManagedChart(true, map[string]interface{}{"spec": monitorSpec}),
			expectErr: "is paused",
		},
		{
			name:          "gate that went GA after an upgrade is skipped",
			value:         "ExpandDisks,VSOCK",
			chart:         newHarvesterManagedChart(false, map[string]interface{}{"spec": monitorSpec}),
			expectGates:   []interface{}{"VSOCK"},
			expectUpdated: true,
		},
		{
			name:  "stale GA gate already in the ManagedChart is removed",
			value: "Snapshot,ExpandDisks",
			chart: newHarvesterManagedChart(false, map[string]interface{}{
				"spec": monitorSpec, kubeVirtAdditionalFeatureGatesKey: []interface{}{"Snapshot", "ExpandDisks"},
			}),
			expectGates:   []interface{}{"Snapshot"},
			expectUpdated: true,
		},
		{
			name:  "value with only skipped gates removes the key",
			value: "ExpandDisks",
			chart: newHarvesterManagedChart(false, map[string]interface{}{
				"spec": monitorSpec, kubeVirtAdditionalFeatureGatesKey: []interface{}{"ExpandDisks"},
			}),
			expectNoKey:   true,
			expectUpdated: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.chart)
			h := &Handler{
				managedCharts:     fakeclients.ManagedChartClient(clientset.ManagementV3().ManagedCharts),
				managedChartCache: fakeclients.ManagedChartCache(clientset.ManagementV3().ManagedCharts),
			}
			setting := &harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.KubeVirtFeatureGatesSettingName},
				Value:      tc.value,
			}

			err := h.syncKubeVirtFeatureGates(setting)
			if tc.expectErr != "" {
				assert.ErrorContains(t, err, tc.expectErr)
				return
			}
			assert.NoError(t, err)

			updated, err := clientset.ManagementV3().ManagedCharts(ManagedChartNamespace).Get(context.TODO(), HarvesterManagedChartName, metav1.GetOptions{})
			assert.NoError(t, err)
			updates := 0
			for _, action := range clientset.Actions() {
				if action.GetVerb() == "update" && action.GetResource().Resource == "managedcharts" {
					updates++
				}
			}
			if tc.expectUpdated {
				assert.Equal(t, 1, updates)
			} else {
				assert.Equal(t, 0, updates)
			}

			// Values outside kubevirt are never touched.
			assert.Equal(t, map[string]interface{}{"enabled": true}, updated.Spec.Values.Data["harvester-node-disk-manager"])

			kubeVirtValues, _ := updated.Spec.Values.Data[kubeVirtValuesKey].(map[string]interface{})
			gates, exists := kubeVirtValues[kubeVirtAdditionalFeatureGatesKey]
			if tc.expectNoKey {
				assert.False(t, exists)
			} else {
				assert.Equal(t, tc.expectGates, gates)
			}
			if spec, ok := tc.chart.Spec.Values.Data[kubeVirtValuesKey].(map[string]interface{}); ok {
				assert.Equal(t, spec["spec"], kubeVirtValues["spec"])
			}
		})
	}
}
