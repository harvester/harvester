package addon

import (
	"testing"

	loggingv1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvesterFake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_validateUpdatedAddon(t *testing.T) {
	var testCases = []struct {
		name          string
		oldAddon      *harvesterv1.Addon
		newAddon      *harvesterv1.Addon
		expectedError bool
	}{
		{
			name: "user can enable addon",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			expectedError: false,
		},
		{
			name: "user can disable addon",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			expectedError: false,
		},
		{
			name: "user can't change chart field",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1-changed",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			expectedError: true,
		},
		{
			name: "user can't change disabling addon",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
				Status: harvesterv1.AddonStatus{
					Status: harvesterv1.AddonDisabling,
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1-changed",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
				Status: harvesterv1.AddonStatus{
					Status: harvesterv1.AddonDisabling,
				},
			},
			expectedError: true,
		},
		{
			name: "user can disable deployed addon",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
				Status: harvesterv1.AddonStatus{
					Status: harvesterv1.AddonDeployed,
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
				Status: harvesterv1.AddonStatus{
					Status: harvesterv1.AddonDeployed,
				},
			},
			expectedError: false,
		},
		{
			name: "user can't disable enabling addon",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "disable-enabling-addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
				Status: harvesterv1.AddonStatus{
					Status: harvesterv1.AddonEnabling,
					Conditions: []harvesterv1.Condition{
						{
							Type:   harvesterv1.AddonOperationInProgress,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "disable-enabling-addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
				Status: harvesterv1.AddonStatus{
					Status: harvesterv1.AddonEnabling,
					Conditions: []harvesterv1.Condition{
						{
							Type:   harvesterv1.AddonOperationInProgress,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedError: true,
		},
		{
			name: "user can change addon annotations when addon is being enabled",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "change-enabling-addon1-annotation",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
				Status: harvesterv1.AddonStatus{
					Status: harvesterv1.AddonDeployed,
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "change-enabling-addon1-annotation",
					Annotations: map[string]string{
						"harvesterhci.io/addon-operation-timeout": "2",
					},
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
				Status: harvesterv1.AddonStatus{
					Status: harvesterv1.AddonDeployed,
				},
			},
			expectedError: false,
		},
		{
			name: "user can disable deployfailed addon",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "disable-deployfailed-addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
				Status: harvesterv1.AddonStatus{
					Status: harvesterv1.AddonEnabling,
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "disable-deployfailed-addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
				Status: harvesterv1.AddonStatus{
					Status: harvesterv1.AddonEnabling,
					Conditions: []harvesterv1.Condition{
						{
							Type:   harvesterv1.AddonOperationFailed,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "virtual cluster addon with valid dns",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vClusterAddonName,
					Namespace: vClusterAddonNamespace,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "vcluster",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vClusterAddonName,
					Namespace: vClusterAddonNamespace,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "vcluster",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "hostname: rancher.172.19.108.3.sslip.io\nrancherVersion: v2.7.4\nbootstrapPassword: harvesterAdmin\n",
				},
			},
			expectedError: false,
		},
		{
			name: "virtual cluster addon with ingress-expose address",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vClusterAddonName,
					Namespace: vClusterAddonNamespace,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "vcluster",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vClusterAddonName,
					Namespace: vClusterAddonNamespace,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "vcluster",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "hostname: 172.19.108.3\nrancherVersion: v2.7.4\nbootstrapPassword: harvesterAdmin\n",
				},
			},
			expectedError: true,
		},
		{
			name: "virtual cluster addon with invalid fqdn",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vClusterAddonName,
					Namespace: vClusterAddonNamespace,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "vcluster",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vClusterAddonName,
					Namespace: vClusterAddonNamespace,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "vcluster",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "hostname: FakeAddress.com\nrancherVersion: v2.7.4\nbootstrapPassword: harvesterAdmin\n",
				},
			},
			expectedError: true,
		},
		{
			name: "virtual cluster addon empty hostname",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vClusterAddonName,
					Namespace: vClusterAddonNamespace,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "vcluster",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vClusterAddonName,
					Namespace: vClusterAddonNamespace,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "vcluster",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "hostname: \nrancherVersion: v2.7.4\nbootstrapPassword: harvesterAdmin\n",
				},
			},
			expectedError: true,
		},
	}

	harvesterClientSet := harvesterFake.NewSimpleClientset()

	fakeAddonCache := fakeclients.AddonCache(harvesterClientSet.HarvesterhciV1beta1().Addons)
	fakeFlowCache := fakeclients.FlowCache(harvesterClientSet.LoggingV1beta1().Flows)
	fakeOutputCache := fakeclients.OutputCache(harvesterClientSet.LoggingV1beta1().Outputs)
	fakeClusterFlowCache := fakeclients.ClusterFlowCache(harvesterClientSet.LoggingV1beta1().ClusterFlows)
	fakeClusterOutputCache := fakeclients.ClusterOutputCache(harvesterClientSet.LoggingV1beta1().ClusterOutputs)
	validator := NewValidator(fakeAddonCache, fakeFlowCache, fakeOutputCache, fakeClusterFlowCache, fakeClusterOutputCache).(*addonValidator)

	for _, tc := range testCases {
		err := validator.validateUpdatedAddon(tc.newAddon, tc.oldAddon)
		if tc.expectedError {
			assert.NotNil(t, err, tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}

func Test_validateNewAddon(t *testing.T) {
	var testCases = []struct {
		name          string
		newAddon      *harvesterv1.Addon
		addonList     []*harvesterv1.Addon
		expectedError bool
	}{
		{
			name: "user can add new addon",
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			addonList:     []*harvesterv1.Addon{},
			expectedError: false,
		},
		{
			name: "user cannot add same addon, no matter differences in version and repo fields",
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			addonList: []*harvesterv1.Addon{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon1",
					},
					Spec: harvesterv1.AddonSpec{
						Repo:          "repo1",
						Chart:         "chart1",
						Version:       "version1",
						Enabled:       true,
						ValuesContent: "sample",
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		harvesterClientSet := harvesterFake.NewSimpleClientset()
		fakeAddonCache := fakeclients.AddonCache(harvesterClientSet.HarvesterhciV1beta1().Addons)
		fakeFlowCache := fakeclients.FlowCache(harvesterClientSet.LoggingV1beta1().Flows)
		fakeOutputCache := fakeclients.OutputCache(harvesterClientSet.LoggingV1beta1().Outputs)
		fakeClusterFlowCache := fakeclients.ClusterFlowCache(harvesterClientSet.LoggingV1beta1().ClusterFlows)
		fakeClusterOutputCache := fakeclients.ClusterOutputCache(harvesterClientSet.LoggingV1beta1().ClusterOutputs)
		validator := NewValidator(fakeAddonCache, fakeFlowCache, fakeOutputCache, fakeClusterFlowCache, fakeClusterOutputCache).(*addonValidator)
		for _, addon := range tc.addonList {
			err := harvesterClientSet.Tracker().Add(addon)
			assert.Nil(t, err)
		}

		err := validator.validateNewAddon(tc.newAddon)
		if tc.expectedError {
			assert.NotNil(t, err, tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}

func Test_validateRancherLoggingAddonWithClusterFlow(t *testing.T) {
	var testCases = []struct {
		name           string
		oldAddon       *harvesterv1.Addon
		newAddon       *harvesterv1.Addon
		clusterFlows   []*loggingv1.ClusterFlow
		clusterOutputs []*loggingv1.ClusterOutput
		expectedError  bool
	}{
		{
			name: "user can enable rancher-logging addon with empty clusterflows",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			clusterFlows:   []*loggingv1.ClusterFlow{},
			clusterOutputs: []*loggingv1.ClusterOutput{},
			expectedError:  false,
		},
		{
			name: "user can enable rancher-logging addon with dangling clusterflows when webhook is skipped",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
					Annotations: map[string]string{
						util.AnnotationSkipRancherLoggingAddonWebhookCheck: "true",
					},
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			clusterFlows: []*loggingv1.ClusterFlow{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cf1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.ClusterFlowSpec{
						LoggingRef:       "normal",
						GlobalOutputRefs: []string{"co1"},
					},
				},
			},
			clusterOutputs: []*loggingv1.ClusterOutput{},
			expectedError:  false,
		},
		{
			name: "user can't enable rancher-logging addon with dangling clusterflows: no output",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			clusterFlows: []*loggingv1.ClusterFlow{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cf1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.ClusterFlowSpec{
						LoggingRef:       "normal",
						GlobalOutputRefs: []string{"co1"},
					},
				},
			},
			clusterOutputs: []*loggingv1.ClusterOutput{},
			expectedError:  true,
		},
		{
			name: "user can't enable rancher-logging addon with dangling clusterflows: namespace mismtach",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			clusterFlows: []*loggingv1.ClusterFlow{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cf1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.ClusterFlowSpec{
						LoggingRef:       "test",
						GlobalOutputRefs: []string{"co1"},
					},
				},
			},
			clusterOutputs: []*loggingv1.ClusterOutput{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "co1",
						Namespace: "random",
					},
					Spec: loggingv1.ClusterOutputSpec{
						OutputSpec: loggingv1.OutputSpec{
							LoggingRef: "test",
						},
					},
				},
			},
			expectedError: true,
		},
		{
			name: "user can't enable rancher-logging addon with dangling clusterflows: LoggingRef mismtach",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			clusterFlows: []*loggingv1.ClusterFlow{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cf1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.ClusterFlowSpec{
						LoggingRef:       "test",
						GlobalOutputRefs: []string{"co1"},
					},
				},
			},
			clusterOutputs: []*loggingv1.ClusterOutput{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "co1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.ClusterOutputSpec{
						OutputSpec: loggingv1.OutputSpec{
							LoggingRef: "test2",
						},
					},
				},
			},
			expectedError: true,
		},
		{
			name: "user can enable rancher-logging addon with valid clusterflow & clusteroutput",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			clusterFlows: []*loggingv1.ClusterFlow{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cf1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.ClusterFlowSpec{
						LoggingRef:       "test",
						GlobalOutputRefs: []string{"co1"},
					},
				},
			},
			clusterOutputs: []*loggingv1.ClusterOutput{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "co1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.ClusterOutputSpec{
						OutputSpec: loggingv1.OutputSpec{
							LoggingRef: "test",
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "user can enable rancher-logging addon with clusterflows which refers no clusteroutput",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			clusterFlows: []*loggingv1.ClusterFlow{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cf1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.ClusterFlowSpec{
						LoggingRef: "test",
					},
				},
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		harvesterClientSet := harvesterFake.NewSimpleClientset()
		fakeAddonCache := fakeclients.AddonCache(harvesterClientSet.HarvesterhciV1beta1().Addons)
		fakeFlowCache := fakeclients.FlowCache(harvesterClientSet.LoggingV1beta1().Flows)
		fakeOutputCache := fakeclients.OutputCache(harvesterClientSet.LoggingV1beta1().Outputs)
		fakeClusterFlowCache := fakeclients.ClusterFlowCache(harvesterClientSet.LoggingV1beta1().ClusterFlows)
		fakeClusterOutputCache := fakeclients.ClusterOutputCache(harvesterClientSet.LoggingV1beta1().ClusterOutputs)
		validator := NewValidator(fakeAddonCache, fakeFlowCache, fakeOutputCache, fakeClusterFlowCache, fakeClusterOutputCache).(*addonValidator)
		for _, cf := range tc.clusterFlows {
			err := harvesterClientSet.Tracker().Add(cf)
			assert.Nil(t, err)
		}
		for _, co := range tc.clusterOutputs {
			err := harvesterClientSet.Tracker().Add(co)
			assert.Nil(t, err)
		}

		err := validator.validateUpdatedAddon(tc.newAddon, tc.oldAddon)
		if tc.expectedError {
			assert.NotNil(t, err, tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}

func Test_validateRancherLoggingAddonWithFlow(t *testing.T) {
	var testCases = []struct {
		name          string
		oldAddon      *harvesterv1.Addon
		newAddon      *harvesterv1.Addon
		flows         []*loggingv1.Flow
		outputs       []*loggingv1.Output
		expectedError bool
	}{
		{
			name: "user can't enable rancher-logging addon with dangling flows: no output",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			flows: []*loggingv1.Flow{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "flow1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.FlowSpec{
						LoggingRef:      "test",
						LocalOutputRefs: []string{"output1"},
					},
				},
			},
			outputs:       []*loggingv1.Output{},
			expectedError: true,
		},
		{
			name: "user can't enable rancher-logging addon with dangling flows: namespace mismtach",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			flows: []*loggingv1.Flow{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "flow1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.FlowSpec{
						LoggingRef:      "test",
						LocalOutputRefs: []string{"output1"},
					},
				},
			},
			outputs: []*loggingv1.Output{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "output1",
						Namespace: "random",
					},
					Spec: loggingv1.OutputSpec{
						LoggingRef: "test",
					},
				},
			},
			expectedError: true,
		},
		{
			name: "user can't enable rancher-logging addon with dangling flows: LoggingRef mismtach",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			flows: []*loggingv1.Flow{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "flow1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.FlowSpec{
						LoggingRef:      "test",
						LocalOutputRefs: []string{"output1"},
					},
				},
			},
			outputs: []*loggingv1.Output{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "output1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.OutputSpec{
						LoggingRef: "invalid",
					},
				},
			},
			expectedError: true,
		},
		{
			name: "user can enable rancher-logging addon with valid flow & output",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			flows: []*loggingv1.Flow{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "flow1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.FlowSpec{
						LoggingRef:      "test",
						LocalOutputRefs: []string{"output1"},
					},
				},
			},
			outputs: []*loggingv1.Output{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "output1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.OutputSpec{
						LoggingRef: "test",
					},
				},
			},
			expectedError: false,
		},
		{
			name: "user can enable rancher-logging addon with flows which refers no output",
			oldAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			newAddon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RancherLoggingName,
					Namespace: util.CattleLoggingSystemNamespaceName,
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			flows: []*loggingv1.Flow{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "flow1",
						Namespace: util.CattleLoggingSystemNamespaceName,
					},
					Spec: loggingv1.FlowSpec{
						LoggingRef: "test",
					},
				},
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		harvesterClientSet := harvesterFake.NewSimpleClientset()
		fakeAddonCache := fakeclients.AddonCache(harvesterClientSet.HarvesterhciV1beta1().Addons)
		fakeFlowCache := fakeclients.FlowCache(harvesterClientSet.LoggingV1beta1().Flows)
		fakeOutputCache := fakeclients.OutputCache(harvesterClientSet.LoggingV1beta1().Outputs)
		fakeClusterFlowCache := fakeclients.ClusterFlowCache(harvesterClientSet.LoggingV1beta1().ClusterFlows)
		fakeClusterOutputCache := fakeclients.ClusterOutputCache(harvesterClientSet.LoggingV1beta1().ClusterOutputs)
		validator := NewValidator(fakeAddonCache, fakeFlowCache, fakeOutputCache, fakeClusterFlowCache, fakeClusterOutputCache).(*addonValidator)
		for _, cf := range tc.flows {
			err := harvesterClientSet.Tracker().Add(cf)
			assert.Nil(t, err)
		}
		for _, co := range tc.outputs {
			err := harvesterClientSet.Tracker().Add(co)
			assert.Nil(t, err)
		}

		err := validator.validateUpdatedAddon(tc.newAddon, tc.oldAddon)
		if tc.expectedError {
			assert.NotNil(t, err, tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}
