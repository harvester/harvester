package upgradelog

import (
	"testing"

	loggingv1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	mgmtv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/name"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
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
	testManagedChartName  = "test-upgrade-upgradelog-operator"
	testPvcName           = "test-upgrade-upgradelog-log-archive"
	testStatefulSetName   = "test-upgrade-upgradelog-fluentd"
	testArchiveName       = "test-archive"
	testImageVersion      = "dev"
)

var testImages = map[string]Image{
	"config_reloader": {
		Repository: "rancher/config-reload",
		Tag:        "default",
	},
	"fluentbit": {
		Repository: "rancher/fluentbit",
		Tag:        "dev",
	},
	"fluentd": {
		Repository: "test/fluentd",
		Tag:        "dev",
	},
}

func newTestApp() *catalogv1.App {
	return &catalogv1.App{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.RancherLoggingName,
			Namespace: util.CattleLoggingSystemNamespaceName,
		},
		Spec: catalogv1.ReleaseSpec{
			Chart: &catalogv1.Chart{
				Values: mgmtv3.MapStringInterface{
					"images": map[string]interface{}{
						"config_reloader": map[string]interface{}{
							"repository": "rancher/config-reload",
							"tag":        "default",
						},
						"fluentbit": map[string]interface{}{
							"repository": "rancher/fluentbit",
							"tag":        "default",
						},
						"fluentd": map[string]interface{}{
							"repository": "rancher/fluentd",
							"tag":        "dev",
						},
					},
				},
			},
			Values: mgmtv3.MapStringInterface{
				"images": map[string]interface{}{
					"fluentbit": map[string]interface{}{
						"repository": "rancher/fluentbit",
						"tag":        "dev",
					},
					"fluentd": map[string]interface{}{
						"repository": "test/fluentd",
						"tag":        "dev",
					},
				},
			},
		},
	}
}

func newTestClusterFlowBuilder() *clusterFlowBuilder {
	return newClusterFlowBuilder(testClusterFlowName).
		WithLabel(util.LabelUpgradeLog, testUpgradeLogName)
}

func newTestClusterOutputBuilder() *clusterOutputBuilder {
	return newClusterOutputBuilder(testClusterOutputName).
		WithLabel(util.LabelUpgradeLog, testUpgradeLogName)
}

func newTestDaemonSetBuilder() *daemonSetBuilder {
	return newDaemonSetBuilder(testDaemonSetName).
		WithLabel(util.LabelUpgradeLog, testUpgradeLogName)
}

func newTestJobBuilder() *jobBuilder {
	return newJobBuilder(testJobName)
}

func newTestLoggingBuilder() *loggingBuilder {
	return newLoggingBuilder(testLoggingName).
		WithLabel(util.LabelUpgradeLog, testUpgradeLogName)
}

func newTestManagedChartBuilder() *managedChartBuilder {
	return newManagedChartBuilder(testManagedChartName)
}

func newTestPvcBuilder() *pvcBuilder {
	return newPvcBuilder(testPvcName).
		WithLabel(util.LabelUpgradeLog, testUpgradeLogName)
}

func newTestStatefulSetBuilder() *statefulSetBuilder {
	return newStatefulSetBuilder(testStatefulSetName).
		WithLabel(util.LabelUpgradeLog, testUpgradeLogName)
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
			name: "The log-collecting rule ClusterFlow is inactive, should therefore keep the respective UpgradeLog resource untouched",
			given: input{
				key:         testClusterFlowName,
				clusterFlow: newTestClusterFlowBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).Build(),
				upgradeLog:  newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
		},
		{
			name: "The log-collecting rule ClusterFlow is active, should therefore set the respective annotation",
			given: input{
				key:         testClusterFlowName,
				clusterFlow: newTestClusterFlowBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).Active().Build(),
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
			namespace:        util.HarvesterSystemNamespaceName,
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.clusterFlow, actual.err = handler.OnClusterFlowChange(tc.given.key, tc.given.clusterFlow)

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, testUpgradeLogName)
			assert.Nil(t, err)
			emptyConditionsTime(tc.expected.upgradeLog.Status.Conditions)
			emptyConditionsTime(actual.upgradeLog.Status.Conditions)
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
			name: "The log-collecting rule ClusterOutput is inactive, should therefore keep the respective UpgradeLog resource untouched",
			given: input{
				key:           testClusterOutputName,
				clusterOutput: newTestClusterOutputBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).Build(),
				upgradeLog:    newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
		},
		{
			name: "The log-collecting rule ClusterOutput is active, should therefore set the respective annotation",
			given: input{
				key:           testClusterOutputName,
				clusterOutput: newTestClusterOutputBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).Active().Build(),
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
			namespace:        util.HarvesterSystemNamespaceName,
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.clusterOutput, actual.err = handler.OnClusterOutputChange(tc.given.key, tc.given.clusterOutput)

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, testUpgradeLogName)
			assert.Nil(t, err)
			emptyConditionsTime(tc.expected.upgradeLog.Status.Conditions)
			emptyConditionsTime(actual.upgradeLog.Status.Conditions)
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
			name: "The fluent-bit DaemonSet is not ready, should therefore keep the respective UpgradeLog resource untouched",
			given: input{
				key:        testDaemonSetName,
				daemonSet:  newTestDaemonSetBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).NotReady().Build(),
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
		},
		{
			name: "The fluent-bit DaemonSet is ready, should therefore set the respective annotation ",
			given: input{
				key:        testDaemonSetName,
				daemonSet:  newTestDaemonSetBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).Ready().Build(),
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
			namespace:        util.HarvesterSystemNamespaceName,
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.daemonSet, actual.err = handler.OnDaemonSetChange(tc.given.key, tc.given.daemonSet)

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, testUpgradeLogName)
			assert.Nil(t, err)
			emptyConditionsTime(tc.expected.upgradeLog.Status.Conditions)
			emptyConditionsTime(actual.upgradeLog.Status.Conditions)
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
			name: "The log-packager Job is still running, should therefore set DownloadReady to False",
			given: input{
				key:        testJobName,
				job:        newTestJobBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).WithAnnotation(util.AnnotationArchiveName, testArchiveName).Build(),
				upgradeLog: newTestUpgradeLogBuilder().Archive(testArchiveName, 0, "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Archive(testArchiveName, 0, "").Build(),
			},
		},
		{
			name: "The log-packager Job is done, should therefore set DownloadReady to True",
			given: input{
				key:        testJobName,
				job:        newTestJobBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).WithAnnotation(util.AnnotationArchiveName, testArchiveName).Done().Build(),
				upgradeLog: newTestUpgradeLogBuilder().Archive(testArchiveName, 0, "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Archive(testArchiveName, 0, "").ArchiveReady(testArchiveName, true, "").Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgradeLog)

		var handler = &handler{
			namespace:        util.HarvesterSystemNamespaceName,
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.job, actual.err = handler.OnJobChange(tc.given.key, tc.given.job)

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, testUpgradeLogName)
			assert.Nil(t, err)
			emptyConditionsTime(tc.expected.upgradeLog.Status.Conditions)
			emptyConditionsTime(actual.upgradeLog.Status.Conditions)
			assert.Equal(t, tc.expected.upgradeLog, actual.upgradeLog, "case %q", tc.name)
		}
	}
}

func TestHandler_OnManagedChartChange(t *testing.T) {
	type input struct {
		key          string
		managedChart *mgmtv3.ManagedChart
		upgradeLog   *harvesterv1.UpgradeLog
	}
	type output struct {
		managedChart *mgmtv3.ManagedChart
		upgradeLog   *harvesterv1.UpgradeLog
		err          error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "The logging-operator ManagedChart is not ready, should therefore keep the respective UpgradeLog resource untouched",
			given: input{
				key:          testManagedChartName,
				managedChart: newTestManagedChartBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).Build(),
				upgradeLog:   newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
		},
		{
			name: "The logging-operator ManagedChart is ready, should therefore reflect on the UpgradeLog resource",
			given: input{
				key:          testManagedChartName,
				managedChart: newTestManagedChartBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).Ready().Build(),
				upgradeLog:   newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgradeLog)

		var handler = &handler{
			namespace:        util.HarvesterSystemNamespaceName,
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.managedChart, actual.err = handler.OnManagedChartChange(tc.given.key, tc.given.managedChart)

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, testUpgradeLogName)
			assert.Nil(t, err)
			emptyConditionsTime(tc.expected.upgradeLog.Status.Conditions)
			emptyConditionsTime(actual.upgradeLog.Status.Conditions)
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
			name: "The fluentd StatefulSet is not ready, should therefore keep the respective UpgradeLog resource untouched",
			given: input{
				key:         testStatefulSetName,
				statefulSet: newTestStatefulSetBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).Replicas(1).Build(),
				upgradeLog:  newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
		},
		{
			name: "The fluetd StatefulSet is ready, should therefore set the respective annotation ",
			given: input{
				key:         testStatefulSetName,
				statefulSet: newTestStatefulSetBuilder().WithLabel(util.LabelUpgradeLog, testUpgradeLogName).Replicas(1).ReadyReplicas(1).Build(),
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
			namespace:        util.HarvesterSystemNamespaceName,
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.statefulSet, actual.err = handler.OnStatefulSetChange(tc.given.key, tc.given.statefulSet)

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, testUpgradeLogName)
			assert.Nil(t, err)
			emptyConditionsTime(tc.expected.upgradeLog.Status.Conditions)
			emptyConditionsTime(actual.upgradeLog.Status.Conditions)
			assert.Equal(t, tc.expected.upgradeLog, actual.upgradeLog, "case %q", tc.name)
		}
	}
}

func TestHandler_OnUpgradeChange(t *testing.T) {
	type input struct {
		key        string
		upgrade    *harvesterv1.Upgrade
		upgradeLog *harvesterv1.UpgradeLog
	}
	type output struct {
		upgrade    *harvesterv1.Upgrade
		upgradeLog *harvesterv1.UpgradeLog
		err        error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "The upgrade is labeled with read-message, should therefore purge the relevant UpgradeLog and its sub-components",
			given: input{
				key: testUpgradeName,
				upgrade: newTestUpgradeBuilder().
					WithLabel(util.LabelUpgradeReadMessage, "true").
					LogEnable(true).
					UpgradeLogStatus(testUpgradeLogName).Build(),
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					WithLabel(util.LabelUpgradeReadMessage, "true").
					LogEnable(true).Build(),
			},
		},
		{
			name: "The upgrade is labeled with other labels, should therefore leave the relevant UpgradeLog untouched",
			given: input{
				key:        testUpgradeName,
				upgrade:    newTestUpgradeBuilder().WithLabel(util.LabelUpgradeReadMessage, "fake").Build(),
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
			expected: output{
				upgrade:    newTestUpgradeBuilder().WithLabel(util.LabelUpgradeReadMessage, "fake").Build(),
				upgradeLog: newTestUpgradeLogBuilder().Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgrade, tc.given.upgradeLog)

		var handler = &handler{
			namespace:        util.HarvesterSystemNamespaceName,
			upgradeClient:    fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeCache:     fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			upgradeLogCache:  fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs),
		}

		var actual output
		actual.upgrade, actual.err = handler.OnUpgradeChange(tc.given.key, tc.given.upgrade)

		if tc.expected.upgrade != nil {
			emptyConditionsTime(tc.expected.upgrade.Status.Conditions)
			emptyConditionsTime(actual.upgrade.Status.Conditions)
			assert.Equal(t, tc.expected.upgrade, actual.upgrade, "case %q", tc.name)
		}

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, testUpgradeLogName)
			assert.Nil(t, err)
			emptyConditionsTime(tc.expected.upgradeLog.Status.Conditions)
			emptyConditionsTime(actual.upgradeLog.Status.Conditions)
			assert.Equal(t, tc.expected.upgradeLog, actual.upgradeLog, "case %q", tc.name)
		} else {
			var err error
			actual.upgradeLog, err = handler.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, testUpgradeLogName)
			assert.True(t, apierrors.IsNotFound(err), "case %q", tc.name)
		}
	}
}

func TestHandler_OnUpgradeLogChange(t *testing.T) {
	type input struct {
		key           string
		addon         *harvesterv1.Addon
		app           *catalogv1.App
		clusterFlow   *loggingv1.ClusterFlow
		clusterOutput *loggingv1.ClusterOutput
		logging       *loggingv1.Logging
		managedChart  *mgmtv3.ManagedChart
		pvc           *corev1.PersistentVolumeClaim
		upgrade       *harvesterv1.Upgrade
		upgradeLog    *harvesterv1.UpgradeLog
	}
	type output struct {
		clusterFlow   *loggingv1.ClusterFlow
		clusterOutput *loggingv1.ClusterOutput
		deployment    *appsv1.Deployment
		logging       *loggingv1.Logging
		managedChart  *mgmtv3.ManagedChart
		pvc           *corev1.PersistentVolumeClaim
		service       *corev1.Service
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
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "Both Addon and ManagedChart do not exist, therefore install the ManagedChart",
			given: input{
				key: testUpgradeLogName,
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				managedChart: prepareOperator(newTestUpgradeLogBuilder().Build()),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "There exists an enabled rancher-logging Addon, therefore skip the ManagedChart installation",
			given: input{
				key:   testUpgradeLogName,
				addon: newAddonBuilder(util.RancherLoggingName).Enable(true).Build(),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "Skipped", "rancher-logging Addon is enabled").Build(),
			},
		},
		{
			name: "There exists a ready rancher-logging ManagedChart, therefore skip the ManagedChart installation",
			given: input{
				key:          testUpgradeLogName,
				managedChart: newManagedChartBuilder(util.RancherLoggingName).Ready().Build(),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "Skipped", "rancher-logging ManagedChart is ready").Build(),
			},
		},
		{
			name: "The logging-operator is deployed, should therefore create Logging resource",
			given: input{
				key: testUpgradeLogName,
				app: newTestApp(),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				logging: prepareLogging(newTestUpgradeLogBuilder().Build(), testImages),
				pvc:     preparePvc(newTestUpgradeLogBuilder().Build()),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "The underlying logging infrastructure is partly ready (fluent-bit), should therefore keep the respective UpgradeLog resource untouched",
			given: input{
				key: testUpgradeLogName,
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentBitAnnotation, upgradeLogFluentBitReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentBitAnnotation, upgradeLogFluentBitReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "The underlying logging infrastructure is partly ready (fluentd), should therefore keep the respective UpgradeLog resource untouched",
			given: input{
				key: testUpgradeLogName,
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentdAnnotation, upgradeLogFluentdReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentdAnnotation, upgradeLogFluentdReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "The underlying logging infrastructure is ready, should therefore mark the InfraReady condition as ready",
			given: input{
				key: testUpgradeLogName,
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentBitAnnotation, upgradeLogFluentBitReady).
					WithAnnotation(upgradeLogFluentdAnnotation, upgradeLogFluentdReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogFluentBitAnnotation, upgradeLogFluentBitReady).
					WithAnnotation(upgradeLogFluentdAnnotation, upgradeLogFluentdReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
		{
			name: "The InfraReady condition is marked as ready, should therefore installed the log-collecting rules",
			given: input{
				key: testUpgradeLogName,
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				clusterFlow:   prepareClusterFlow(newTestUpgradeLogBuilder().Build()),
				clusterOutput: prepareClusterOutput(newTestUpgradeLogBuilder().Build()),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
		{
			name: "The log-collecting rules are installed, should therefore mark the UpgradeLogReady condition as ready",
			given: input{
				key: testUpgradeLogName,
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogClusterFlowAnnotation, upgradeLogClusterFlowReady).
					WithAnnotation(upgradeLogClusterOutputAnnotation, upgradeLogClusterOutputReady).
					UpgradeLogReadyCondition(corev1.ConditionUnknown, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogClusterFlowAnnotation, upgradeLogClusterFlowReady).
					WithAnnotation(upgradeLogClusterOutputAnnotation, upgradeLogClusterOutputReady).
					WithAnnotation(upgradeLogStateAnnotation, upgradeLogStateCollecting).
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
		{
			name: "The UpgradeLogReady condition is ready, should therefore mark the LogReady condition of the Upgrade resource as ready",
			given: input{
				key:     testUpgradeLogName,
				upgrade: newTestUpgradeBuilder().Build(),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					WithLabel(util.LabelUpgradeState, util.UpgradeStateLoggingInfraPrepared).
					LogReadyCondition(corev1.ConditionTrue, "", "").Build(),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").
					UpgradeEndedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "The UpgradeLogReady condition is ready but the Upgrade resource is missing, should therefore set the UpgradeEnded condition as True",
			given: input{
				key: testUpgradeLogName,
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").
					UpgradeEndedCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
		{
			name: "The logging infra is ready and the upgrade is resumed, should therefore create the log-downloader Deployment",
			given: input{
				key:     testUpgradeLogName,
				upgrade: newTestUpgradeBuilder().Build(),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").
					UpgradeEndedCondition(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				deployment: prepareLogDownloader(newTestUpgradeLogBuilder().Build(), testImageVersion),
				service:    prepareLogDownloaderSvc(newTestUpgradeLogBuilder().Build()),
				upgradeLog: newTestUpgradeLogBuilder().
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").
					UpgradeEndedCondition(corev1.ConditionUnknown, "", "").
					DownloadReadyCondition(corev1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "The UpgradeEnded condition is set as True, should therefore tear down the logging infrastructure (log-archive volume should retain)",
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
					InfraReadyCondition(corev1.ConditionTrue, "", "").
					UpgradeEndedCondition(corev1.ConditionTrue, "", "").
					DownloadReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
			expected: output{
				pvc: preparePvc(newTestUpgradeLogBuilder().Build()),
				upgradeLog: newTestUpgradeLogBuilder().
					WithAnnotation(upgradeLogStateAnnotation, upgradeLogStateStopped).
					UpgradeLogReadyCondition(corev1.ConditionTrue, "", "").
					OperatorDeployedCondition(corev1.ConditionTrue, "", "").
					InfraReadyCondition(corev1.ConditionTrue, "", "").
					UpgradeEndedCondition(corev1.ConditionTrue, "", "").
					DownloadReadyCondition(corev1.ConditionTrue, "", "").Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgradeLog)
		if tc.given.addon != nil {
			var err = clientset.Tracker().Add(tc.given.addon)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.app != nil {
			var err = clientset.Tracker().Add(tc.given.app)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
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
		if tc.given.managedChart != nil {
			var err = clientset.Tracker().Add(tc.given.managedChart)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.upgrade != nil {
			var err = clientset.Tracker().Add(tc.given.upgrade)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var k8sclientset = k8sfake.NewSimpleClientset()
		if tc.given.pvc != nil {
			var err = k8sclientset.Tracker().Add(tc.given.pvc)
			assert.Nil(t, err, "mock resource should add into k8s fake controller tracker")
		}

		var handler = &handler{
			namespace:           util.HarvesterSystemNamespaceName,
			addonCache:          fakeclients.AddonCache(clientset.HarvesterhciV1beta1().Addons),
			appCache:            fakeclients.AppCache(clientset.CatalogV1().Apps),
			clusterFlowClient:   fakeclients.ClusterFlowClient(clientset.LoggingV1beta1().ClusterFlows),
			clusterOutputClient: fakeclients.ClusterOutputClient(clientset.LoggingV1beta1().ClusterOutputs),
			deploymentClient:    fakeclients.DeploymentClient(k8sclientset.AppsV1().Deployments),
			loggingClient:       fakeclients.LoggingClient(clientset.LoggingV1beta1().Loggings),
			managedChartClient:  fakeclients.ManagedChartClient(clientset.ManagementV3().ManagedCharts),
			managedChartCache:   fakeclients.ManagedChartCache(clientset.ManagementV3().ManagedCharts),
			pvcClient:           fakeclients.PersistentVolumeClaimClient(k8sclientset.CoreV1().PersistentVolumeClaims),
			serviceClient:       fakeclients.ServiceClient(k8sclientset.CoreV1().Services),
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
			actual.clusterFlow, err = handler.clusterFlowClient.Get(util.HarvesterSystemNamespaceName, name.SafeConcatName(testUpgradeLogName, util.UpgradeLogFlowComponent), metav1.GetOptions{})
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.clusterFlow, actual.clusterFlow, "case %q", tc.name)
		} else {
			var err error
			actual.clusterFlow, err = handler.clusterFlowClient.Get(util.HarvesterSystemNamespaceName, name.SafeConcatName(testUpgradeLogName, util.UpgradeLogFlowComponent), metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err), "case %q", tc.name)
			assert.Nil(t, actual.clusterFlow, "case %q", tc.name)
		}

		if tc.expected.clusterOutput != nil {
			// HACK: cannot create ClusterOutput with namespace specified using fake client so we skip the field here
			tc.expected.clusterOutput.Namespace = ""
			var err error
			actual.clusterOutput, err = handler.clusterOutputClient.Get(util.HarvesterSystemNamespaceName, name.SafeConcatName(testUpgradeLogName, util.UpgradeLogOutputComponent), metav1.GetOptions{})
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.clusterOutput, actual.clusterOutput, "case %q", tc.name)
		} else {
			var err error
			actual.clusterOutput, err = handler.clusterOutputClient.Get(util.HarvesterSystemNamespaceName, name.SafeConcatName(testUpgradeLogName, util.UpgradeLogOutputComponent), metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err), "case %q", tc.name)
			assert.Nil(t, actual.clusterOutput, "case %q", tc.name)
		}

		if tc.expected.deployment != nil {
			var err error
			actual.deployment, err = handler.deploymentClient.Get(util.HarvesterSystemNamespaceName, name.SafeConcatName(testUpgradeLogName, util.UpgradeLogDownloaderComponent), metav1.GetOptions{})
			assert.Nil(t, err)
		}

		if tc.expected.logging != nil {
			var err error
			actual.logging, err = handler.loggingClient.Get(name.SafeConcatName(testUpgradeLogName, util.UpgradeLogInfraComponent), metav1.GetOptions{})
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.logging, actual.logging, "case %q", tc.name)
		} else {
			var err error
			actual.logging, err = handler.loggingClient.Get(name.SafeConcatName(testUpgradeLogName, util.UpgradeLogInfraComponent), metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err), "case %q", tc.name)
			assert.Nil(t, actual.logging, "case %q", tc.name)
		}

		if tc.expected.managedChart != nil {
			var err error
			actual.managedChart, err = handler.managedChartClient.Get(util.FleetLocalNamespaceName, name.SafeConcatName(testUpgradeLogName, util.UpgradeLogOperatorComponent), metav1.GetOptions{})
			assert.Nil(t, err)
		}

		if tc.expected.pvc != nil {
			var err error
			actual.pvc, err = handler.pvcClient.Get(util.HarvesterSystemNamespaceName, name.SafeConcatName(testUpgradeLogName, util.UpgradeLogArchiveComponent), metav1.GetOptions{})
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.pvc, actual.pvc, "case %q", tc.name)
		} else {
			var err error
			actual.pvc, err = handler.pvcClient.Get(util.HarvesterSystemNamespaceName, name.SafeConcatName(testUpgradeLogName, util.UpgradeLogArchiveComponent), metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err), "case %q", tc.name)
			assert.Nil(t, actual.pvc, "case %q", tc.name)
		}

		if tc.expected.service != nil {
			var err error
			actual.service, err = handler.serviceClient.Get(util.HarvesterSystemNamespaceName, testUpgradeLogName, metav1.GetOptions{})
			assert.Nil(t, err)
			assert.Equal(t, tc.expected.service, actual.service, "case %q", tc.name)
		}

		if tc.expected.upgrade != nil {
			var err error
			actual.upgrade, err = handler.upgradeCache.Get(util.HarvesterSystemNamespaceName, testUpgradeName)
			assert.Nil(t, err)
			emptyConditionsTime(tc.expected.upgrade.Status.Conditions)
			emptyConditionsTime(actual.upgrade.Status.Conditions)
			assert.Equal(t, tc.expected.upgrade, actual.upgrade, "case %q", tc.name)
		}

		if tc.expected.upgradeLog != nil {
			emptyConditionsTime(tc.expected.upgradeLog.Status.Conditions)
			emptyConditionsTime(actual.upgradeLog.Status.Conditions)
			assert.Equal(t, tc.expected.upgradeLog, actual.upgradeLog, "case %q", tc.name)
		}
	}
}

func emptyConditionsTime(conditions []harvesterv1.Condition) {
	for k := range conditions {
		conditions[k].LastTransitionTime = ""
		conditions[k].LastUpdateTime = ""
	}
}
