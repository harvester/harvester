package managedchart

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"

	"github.com/harvester/harvester/pkg/util"
)

func Test_validateDeleteManagedChart(t *testing.T) {

	var testCases = []struct {
		name          string
		mc            *managementv3.ManagedChart
		expectedError bool
	}{
		{
			name: "user cannot delete managedchart harvester ",
			mc: &managementv3.ManagedChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.HarvesterManagedChart,
					Namespace: util.FleetLocalNamespaceName,
				},
			},
			expectedError: true,
		},
		{
			name: "user cannot delete managedchart harvester-crd",
			mc: &managementv3.ManagedChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.HarvesterCRDManagedChart,
					Namespace: util.FleetLocalNamespaceName,
				},
			},
			expectedError: true,
		},
		{
			name: "user cannot delete managedchart rancher-monitoring-crd",
			mc: &managementv3.ManagedChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherMonitoringCRDManagedChart,
					Namespace: util.FleetLocalNamespaceName,
				},
			},
			expectedError: true,
		},
		{
			name: "user cannot delete managedchart rancher-logging-crd",
			mc: &managementv3.ManagedChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingCRDManagedChart,
					Namespace: util.FleetLocalNamespaceName,
				},
			},
			expectedError: true,
		},
		{
			name: "user can delete managedchart fleet-local/test",
			mc: &managementv3.ManagedChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: util.FleetLocalNamespaceName,
				},
			},
			expectedError: false,
		},
		{
			name: "user can delete managedchart test/test",
			mc: &managementv3.ManagedChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {

		validator := NewValidator().(*managedChartValidator)

		err := validator.Delete(nil, tc.mc)
		if tc.expectedError {
			assert.NotNil(t, err, tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}
