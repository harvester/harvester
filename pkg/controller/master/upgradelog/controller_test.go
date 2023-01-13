package upgradelog

import (
	"fmt"
	"testing"

	loggingv1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	harvesterUpgradeLabel = "harvesterhci.io/upgrade"

	testUpgradeName       = "test-upgrade"
	testUpgradeLogName    = "test-upgrade-upgradelog"
	testClusterFlowName   = "test-upgrade-upgradelog-clusterflow"
	testClusterOutputName = "test-upgrade-upgradelog-clusteroutput"
	testDaemonSetName     = "test-upgrade-upgradelog-fluentbit"
	testDeploymentName    = "test-upgrade-upgradelog-log-downloader"
	testJobName           = "test-upgrade-upgradelog-log-packager"
	testLoggingName       = "test-upgrade-upgradelog-infra"
	testPvcName           = "test-upgrade-upgradelog-log-archive"
	testStatefulSetName   = "test-upgrade-upgradelog-fluentd"
	testArchiveName       = "test-archive"
	testImageVersion      = "dev"
)

func newTestClusterFlowBuilder() *clusterFlowBuilder {
	return newClusterFlowBuilder(testClusterFlowName).
		WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName)
}

func newTestClusterOutputBuilder() *clusterOutputBuilder {
	return newClusterOutputBuilder(testClusterOutputName).
		WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName)
}

func newTestDaemonSetBuilder() *daemonSetBuilder {
	return newDaemonSetBuilder(testDaemonSetName).
		WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName)
}

func newTestJobBuilder() *jobBuilder {
	return newJobBuilder(testJobName)
}

func newTestLoggingBuilder() *loggingBuilder {
	return newLoggingBuilder(testLoggingName).
		WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName)
}

func newTestPvcBuilder() *pvcBuilder {
	return newPvcBuilder(testPvcName).
		WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName)
}

func newTestStatefulSetBuilder() *statefulSetBuilder {
	return newStatefulSetBuilder(testStatefulSetName).
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

func TestHandler_OnClusterFlowChange(t *testing.T) {
	type input struct {
		key         string
		clusterFlow *loggingv1.ClusterFlow
		upgradeLog  *harvesterv1.UpgradeLog
	}
	type output struct {
		clusterFlow *loggingv1.ClusterFlow
		upgradeLog  *harvesterv1.UpgradeLog
		err         error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "The log collecting rule clusterFlow is inactive, should therefore keep the respective upgradeLog resource untouched",
			given: input{
				key:         testClusterFlowName,
				clusterFlow: newTestClusterFlowBuilder().WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName).Build(),
				upgradeLog:  newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
		},
		{
			name: "The log collecting rule clusterFlow is active, should therefore set the respective annotation",
			given: input{
				key:         testClusterFlowName,
				clusterFlow: newTestClusterFlowBuilder().WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName).Active().Build(),
				upgradeLog:  newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().WithAnnotation(upgradeLogClusterFlowAnnotation, upgradeLogClusterFlowReady).Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgradeLog)

		var handler = &handler{
			namespace:        upgradeLogNamespace,
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.clusterFlow, actual.err = handler.OnClusterFlowChange(tc.given.key, tc.given.clusterFlow)

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(upgradeLogNamespace, testUpgradeLogName)
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.upgradeLog, actual.upgradeLog, "case %q", tc.name)
		}
	}
}

func TestHandler_OnClusterOutputChange(t *testing.T) {
	type input struct {
		key           string
		clusterOutput *loggingv1.ClusterOutput
		upgradeLog    *harvesterv1.UpgradeLog
	}
	type output struct {
		clusterOutput *loggingv1.ClusterOutput
		upgradeLog    *harvesterv1.UpgradeLog
		err           error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "The log collecting rule clusterOutput is inactive, should therefore keep the respective upgradeLog resource untouched",
			given: input{
				key:           testClusterOutputName,
				clusterOutput: newTestClusterOutputBuilder().WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName).Build(),
				upgradeLog:    newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
		},
		{
			name: "The log collecting rule clusterOutput is active, should therefore set the respective annotation",
			given: input{
				key:           testClusterOutputName,
				clusterOutput: newTestClusterOutputBuilder().WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName).Active().Build(),
				upgradeLog:    newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().WithAnnotation(upgradeLogClusterOutputAnnotation, upgradeLogClusterOutputReady).Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgradeLog)

		var handler = &handler{
			namespace:        upgradeLogNamespace,
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.clusterOutput, actual.err = handler.OnClusterOutputChange(tc.given.key, tc.given.clusterOutput)

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(upgradeLogNamespace, testUpgradeLogName)
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.upgradeLog, actual.upgradeLog, "case %q", tc.name)
		}
	}
}

func TestHandler_OnDaemonSetChange(t *testing.T) {
	type input struct {
		key        string
		daemonSet  *appsv1.DaemonSet
		upgradeLog *harvesterv1.UpgradeLog
	}
	type output struct {
		daemonSet  *appsv1.DaemonSet
		upgradeLog *harvesterv1.UpgradeLog
		err        error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "The fluent-bit daemonSet is not ready, should therefore keep the respective upgradeLog resource untouched",
			given: input{
				key:        testDaemonSetName,
				daemonSet:  newTestDaemonSetBuilder().WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName).NotReady().Build(),
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
		},
		{
			name: "The fluent-bit daemonSet is ready, should therefore set the respective annotation ",
			given: input{
				key:        testDaemonSetName,
				daemonSet:  newTestDaemonSetBuilder().WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName).Ready().Build(),
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().WithAnnotation(upgradeLogFluentBitAnnotation, upgradeLogFluentBitReady).Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgradeLog)

		var handler = &handler{
			namespace:        upgradeLogNamespace,
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.daemonSet, actual.err = handler.OnDaemonSetChange(tc.given.key, tc.given.daemonSet)

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(upgradeLogNamespace, testUpgradeLogName)
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.upgradeLog, actual.upgradeLog, "case %q", tc.name)
		}
	}
}

func TestHandler_OnJobChange(t *testing.T) {
	type input struct {
		key        string
		job        *batchv1.Job
		upgradeLog *harvesterv1.UpgradeLog
	}
	type output struct {
		job        *batchv1.Job
		upgradeLog *harvesterv1.UpgradeLog
		err        error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "The log packager job is still running, should therefore set DownloadReady to False",
			given: input{
				key:        testJobName,
				job:        newTestJobBuilder().WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName).WithAnnotation(archiveNameAnnotation, testArchiveName).Build(),
				upgradeLog: newTestUpgradeLogBuilder().Archive(testArchiveName, 0, "", false).Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Archive(testArchiveName, 0, "", false).Build(),
			},
		},
		{
			name: "The log packager job is done, should therefore set DownloadReady to True",
			given: input{
				key:        testJobName,
				job:        newTestJobBuilder().WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName).WithAnnotation(archiveNameAnnotation, testArchiveName).Done().Build(),
				upgradeLog: newTestUpgradeLogBuilder().Archive(testArchiveName, 0, "", false).Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Archive(testArchiveName, 0, "", true).Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgradeLog)

		var handler = &handler{
			namespace:        upgradeLogNamespace,
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.job, actual.err = handler.OnJobChange(tc.given.key, tc.given.job)

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(upgradeLogNamespace, testUpgradeLogName)
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.upgradeLog, actual.upgradeLog, "case %q", tc.name)
		}
	}
}

func TestHandler_OnStatefulSetChange(t *testing.T) {
	type input struct {
		key         string
		statefulSet *appsv1.StatefulSet
		upgradeLog  *harvesterv1.UpgradeLog
	}
	type output struct {
		statefulSet *appsv1.StatefulSet
		upgradeLog  *harvesterv1.UpgradeLog
		err         error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "The fluentd statefulSet is not ready, should therefore keep the respective upgradeLog resource untouched",
			given: input{
				key:         testStatefulSetName,
				statefulSet: newTestStatefulSetBuilder().WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName).Replicas(1).Build(),
				upgradeLog:  newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
		},
		{
			name: "The fluetd statefulSet is ready, should therefore set the respective annotation ",
			given: input{
				key:         testStatefulSetName,
				statefulSet: newTestStatefulSetBuilder().WithLabel(harvesterUpgradeLogLabel, testUpgradeLogName).Replicas(1).ReadyReplicas(1).Build(),
				upgradeLog:  newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().WithAnnotation(upgradeLogFluentdAnnotation, upgradeLogFluentdReady).Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgradeLog)

		var handler = &handler{
			namespace:        upgradeLogNamespace,
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.statefulSet, actual.err = handler.OnStatefulSetChange(tc.given.key, tc.given.statefulSet)

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(upgradeLogNamespace, testUpgradeLogName)
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.upgradeLog, actual.upgradeLog, "case %q", tc.name)
		}
	}
}

func TestHandler_OnUpgradeLogChange(t *testing.T) {
	type input struct {
		key           string
		clusterFlow   *loggingv1.ClusterFlow
		clusterOutput *loggingv1.ClusterOutput
		logging       *loggingv1.Logging
		pvc           *corev1.PersistentVolumeClaim
		upgrade       *harvesterv1.Upgrade
		upgradeLog    *harvesterv1.UpgradeLog
	}
	type output struct {
		clusterFlow   *loggingv1.ClusterFlow
		clusterOutput *loggingv1.ClusterOutput
		deployment    *appsv1.Deployment
		logging       *loggingv1.Logging
		pvc           *corev1.PersistentVolumeClaim
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
				key: testUpgradeLogName,
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				logging: prepareLogging(newTestUpgradeLogBuilder().Build()),
				pvc:     preparePvc(newTestUpgradeLogBuilder().Build()),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "The underlying logging infrastructure is partly ready (fluent-bit), should therefore keep the respective upgradeLog resource untouched",
			given: input{
				key: testUpgradeLogName,
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentBitAnnotation, upgradeLogFluentBitReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentBitAnnotation, upgradeLogFluentBitReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "The underlying logging infrastructure is partly ready (fluentd), should therefore keep the respective upgradeLog resource untouched",
			given: input{
				key: testUpgradeLogName,
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentdAnnotation, upgradeLogFluentdReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentdAnnotation, upgradeLogFluentdReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "The underlying logging infrastructure is ready, should therefore mark the InfraScaffolded condition as ready",
			given: input{
				key: testUpgradeLogName,
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
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
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
				key: testUpgradeLogName,
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
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogStateAnnotation, upgradeLogStateCollecting).
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").
					UpgradeEndedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "The logging infra is ready and the upgrade is resumed, should therefore create the downloader deployment",
			given: input{
				key:     testUpgradeLogName,
				upgrade: newTestUpgradeBuilder().Build(),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").
					UpgradeEndedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				deployment: prepareLogDownloader(newTestUpgradeLogBuilder().Build(), testImageVersion),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").
					UpgradeEndedCondition(corev1.ConditionUnknown, "", "").
					DownloadReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "The UpgradeEnded condition is set as True, should therefore tear down the logging infrastructure (log archive volume should retain)",
			given: input{
				key:           testUpgradeLogName,
				clusterFlow:   newTestClusterFlowBuilder().Build(),
				clusterOutput: newTestClusterOutputBuilder().Build(),
				logging:       newTestLoggingBuilder().Build(),
				pvc:           preparePvc(newTestUpgradeLogBuilder().Build()),
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogStateAnnotation, upgradeLogStateCollecting).
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").
					UpgradeEndedCondition(corev1.ConditionTrue, "", "").
					DownloadReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				pvc: preparePvc(newTestUpgradeLogBuilder().Build()),
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogStateAnnotation, upgradeLogStateStopped).
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraScaffoldedCondition(corev1.ConditionTrue, "", "").
					UpgradeEndedCondition(corev1.ConditionTrue, "", "").
					DownloadReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgradeLog)
		if tc.given.clusterFlow != nil {
			var err = clientset.Tracker().Add(tc.given.clusterFlow)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.clusterOutput != nil {
			var err = clientset.Tracker().Add(tc.given.clusterOutput)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.logging != nil {
			var err = clientset.Tracker().Add(tc.given.logging)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.upgrade != nil {
			var err = clientset.Tracker().Add(tc.given.upgrade)
			assert.Nil(t, err, "moch resource should add into fake controller tracker")
		}

		var k8sclientset = k8sfake.NewSimpleClientset()
		if tc.given.pvc != nil {
			var err = k8sclientset.Tracker().Add(tc.given.pvc)
			assert.Nil(t, err, "mock resource should add into k8s fake controller tracker")
		}

		var handler = &handler{
			namespace:           upgradeLogNamespace,
			clusterFlowClient:   fakeclients.ClusterFlowClient(clientset.LoggingV1beta1().ClusterFlows),
			clusterOutputClient: fakeclients.ClusterOutputClient(clientset.LoggingV1beta1().ClusterOutputs),
			deploymentClient:    fakeclients.DeploymentClient(k8sclientset.AppsV1().Deployments),
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
		} else {
			var err error
			actual.clusterFlow, err = handler.clusterFlowClient.Get(upgradeLogNamespace, fmt.Sprintf("%s-clusterflow", testUpgradeLogName), metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err), "case %q", tc.name)
			assert.Nil(t, actual.clusterFlow, "case %q", tc.name)
		}

		if tc.expected.clusterOutput != nil {
			// HACK: cannot create ClusterOutput with namespace specified using fake client so we skip the field here
			tc.expected.clusterOutput.Namespace = ""
			var err error
			actual.clusterOutput, err = handler.clusterOutputClient.Get(upgradeLogNamespace, fmt.Sprintf("%s-clusteroutput", testUpgradeLogName), metav1.GetOptions{})
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.clusterOutput, actual.clusterOutput, "case %q", tc.name)
		} else {
			var err error
			actual.clusterOutput, err = handler.clusterOutputClient.Get(upgradeLogNamespace, fmt.Sprintf("%s-clusteroutput", testUpgradeLogName), metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err), "case %q", tc.name)
			assert.Nil(t, actual.clusterOutput, "case %q", tc.name)
		}

		if tc.expected.deployment != nil {
			var err error
			actual.deployment, err = handler.deploymentClient.Get(upgradeLogNamespace, fmt.Sprintf("%s-log-downloader", testUpgradeLogName), metav1.GetOptions{})
			assert.Nil(t, err)
		}

		if tc.expected.logging != nil {
			var err error
			actual.logging, err = handler.loggingClient.Get(fmt.Sprintf("%s-infra", testUpgradeLogName), metav1.GetOptions{})
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.logging, actual.logging, "case %q", tc.name)
		} else {
			var err error
			actual.logging, err = handler.loggingClient.Get(fmt.Sprintf("%s-infra", testUpgradeLogName), metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err), "case %q", tc.name)
			assert.Nil(t, actual.logging, "case %q", tc.name)
		}

		if tc.expected.pvc != nil {
			var err error
			actual.pvc, err = handler.pvcClient.Get(upgradeLogNamespace, fmt.Sprintf("%s-log-archive", testUpgradeLogName), metav1.GetOptions{})
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.pvc, actual.pvc, "case %q", tc.name)
		} else {
			var err error
			actual.pvc, err = handler.pvcClient.Get(upgradeLogNamespace, fmt.Sprintf("%s-log-archive", testUpgradeLogName), metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err), "case %q", tc.name)
			assert.Nil(t, actual.pvc, "case %q", tc.name)
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
