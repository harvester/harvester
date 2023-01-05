package upgradelog

import (
	"fmt"
	"testing"

	loggingv1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

const (
	harvesterUpgradeLabel = "harvesterhci.io/upgrade"

	testUpgradeName       = "test-upgrade"
	testUpgradeLogName    = "test-upgrade-upgradelog"
	testClusterFlowName   = "test-upgrade-upgradelog-clusterflow"
	testClusterOutputName = "test-upgrade-upgradelog-clusteroutput"
	testLoggingName       = "test-upgrade-upgradelog-infra"
	testPvcName           = "test-upgrade-upgradelog-log-archive"
)

func newTestClusterFlowBuilder() *clusterFlowBuilder {
	return newClusterFlowBuilder(testClusterFlowName).
		WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName)
}

func newTestClusterOutputBuilder() *clusterOutputBuilder {
	return newClusterOutputBuilder(testClusterOutputName).
		WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName)
}

func newTestLoggingBuilder() *loggingBuilder {
	return newLoggingBuilder(testLoggingName).
		WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName)
}

func newTestPvcBuilder() *pvcBuilder {
	return newPvcBuilder(testPvcName).
		WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName)
}

func newTestUpgradeBuilder() *upgradeBuilder {
	return newUpgradeBuilder(testUpgradeName)
}

func newTestUpgradeLogBuilder() *upgradeLogBuilder {
	return newUpgradeLogBuilder(testUpgradeLogName).
		WithLabel(harvesterUpgradeLabel, testUpgradeName).
		Upgrade(testUpgradeName)
}

func TestHandler_OnUpgradeLogChange(t *testing.T) {
	type input struct {
		key        string
		upgrade    *harvesterv1.Upgrade
		upgradeLog *harvesterv1.UpgradeLog
	}
	type output struct {
		clusterFlow   *loggingv1.ClusterFlow
		clusterOutput *loggingv1.ClusterOutput
		upgrade       *harvesterv1.Upgrade
		upgradeLog    *harvesterv1.UpgradeLog
		err           error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "Initialization",
			given: input{
				key:        testUpgradeLogName,
				upgrade:    newTestUpgradeBuilder().Build(),
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
		{
			name: "The logging-operator is deployed, should therefore create logging resource",
			given: input{
				key:     testUpgradeLogName,
				upgrade: newTestUpgradeBuilder().Build(),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "The underlying logging infrastructure is ready, should therefore mark the InfraScaffolded condition as ready",
			given: input{
				key: testUpgradeLogName,
				// logging: newTestLoggingBuilder().Build(),
				// pvc:     newTestPvcBuilder().Build(),
				upgrade: newTestUpgradeBuilder().Build(),
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentBitAnnotation, upgradeLogFluentBitReady).
					WithAnnotation(upgradeLogFluentdAnnotation, upgradeLogFluentdReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentBitAnnotation, upgradeLogFluentBitReady).
					WithAnnotation(upgradeLogFluentdAnnotation, upgradeLogFluentdReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
		{
			name: "The InfraScaffolded condition is marked as ready, should therefore create clusterflow and clusteroutput resources",
			given: input{
				key: testUpgradeLogName,
				// logging: newTestLoggingBuilder().Build(),
				// pvc:     newTestPvcBuilder().Build(),
				upgrade: newTestUpgradeBuilder().Build(),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				// clusterFlow:   newTestClusterFlowBuilder().Namespace("").Build(),
				// clusterOutput: newTestClusterOutputBuilder().Namespace("").Build(),
				clusterFlow:   prepareClusterFlow(newTestUpgradeLogBuilder().Build()),
				clusterOutput: prepareClusterOutput(newTestUpgradeLogBuilder().Build()),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
		{
			name: "The log collecting rules are installed, should therefore mark the UpgradeLogReady condition as ready",
			given: input{
				key:     testUpgradeLogName,
				upgrade: newTestUpgradeBuilder().Build(),
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogClusterFlowAnnotation, upgradeLogClusterFlowReady).
					WithAnnotation(upgradeLogClusterOutputAnnotation, upgradeLogClusterOutputReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogClusterFlowAnnotation, upgradeLogClusterFlowReady).
					WithAnnotation(upgradeLogClusterOutputAnnotation, upgradeLogClusterOutputReady).
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
		{
			name: "The UpgradeLogReady condition is ready, should therefore mark the LogReady condition of the upgrade resource as ready",
			given: input{
				key:     testUpgradeLogName,
				upgrade: newTestUpgradeBuilder().Build(),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					WithLabel(upgradeStateLabel, UpgradeStateLoggingInfraPrepared).
					LogReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgrade, tc.given.upgradeLog)
		var k8sclientset = k8sfake.NewSimpleClientset()

		var handler = &handler{
			namespace:           upgradeLogNamespace,
			clusterFlowClient:   fakeclients.ClusterFlowClient(clientset.LoggingV1beta1().ClusterFlows),
			clusterOutputClient: fakeclients.ClusterOutputClient(clientset.LoggingV1beta1().ClusterOutputs),
			loggingClient:       fakeclients.LoggingClient(clientset.LoggingV1beta1().Loggings),
			pvcClient:           fakeclients.PersistentVolumeClaimClient(k8sclientset.CoreV1().PersistentVolumeClaims),
			upgradeClient:       fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeCache:        fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeLogClient:    fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.upgradeLog, actual.err = handler.OnUpgradeLogChange(tc.given.key, tc.given.upgradeLog)

		if tc.expected.clusterFlow != nil {
			// HACK: cannot create ClusterFlow with namespace specified using fake client so we skip the field here
			tc.expected.clusterFlow.Namespace = ""
			var err error
			actual.clusterFlow, err = handler.clusterFlowClient.Get(upgradeLogNamespace, fmt.Sprintf("%s-clusterflow", testUpgradeLogName), metav1.GetOptions{})
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.clusterFlow, actual.clusterFlow, "case %q", tc.name)
		}

		if tc.expected.clusterOutput != nil {
			// HACK: cannot create ClusterOutput with namespace specified using fake client so we skip the field here
			tc.expected.clusterOutput.Namespace = ""
			var err error
			actual.clusterOutput, err = handler.clusterOutputClient.Get(upgradeLogNamespace, fmt.Sprintf("%s-clusteroutput", testUpgradeLogName), metav1.GetOptions{})
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.clusterOutput, actual.clusterOutput, "case %q", tc.name)
		}

		if tc.expected.upgrade != nil {
			var err error
			actual.upgrade, err = handler.upgradeCache.Get(upgradeLogNamespace, testUpgradeName)
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.upgrade, actual.upgrade, "case %q", tc.name)
		}

		if tc.expected.upgradeLog != nil {
			assert.Equal(t, tc.expected.upgradeLog, actual.upgradeLog, "case %q", tc.name)
		}
	}
}
