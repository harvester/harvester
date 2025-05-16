package namespace

import (
	"testing"

	"github.com/rancher/wrangler/v3/pkg/webhook"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	psaApi "k8s.io/pod-security-admission/api"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvesterFake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/webhook/types"
)

var (
	pssEnabledSetting = &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.ClusterPodSecurityStandardSettingName,
		},
		Value: `{"enabled":true,"whitelistedNamespacesList":"default", "restrictedNamespacesList": "demo,restricted-ns","privilegedNamespacesList":"demo2,privileged-ns"}`,
	}

	pssDisabledSetting = &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.ClusterPodSecurityStandardSettingName,
		},
		Value: `{"enabled":false,"whitelistedNamespacesList":"default", "restrictedNamespacesList": "demo,restricted-ns","privilegedNamespacesList":"demo2,privileged-ns"}`,
	}

	pssEnabledAdditionalTestNamespaceSetting = &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.ClusterPodSecurityStandardSettingName,
		},
		Value: `{"enabled":true,"whitelistedNamespacesList":"default,test", "restrictedNamespacesList": "demo,restricted-ns,test","privilegedNamespacesList":"demo2,privileged-ns"}`,
	}
)

func Test_NamespaceUpdateValidation(t *testing.T) {

	assert := require.New(t)
	var testCases = []struct {
		Name         string
		Setting      *harvesterv1.Setting
		OldNamespace *corev1.Namespace
		NewNamespace *corev1.Namespace
		Request      *types.Request
		ExpectError  bool
	}{
		{
			Name:    "update pss labels while setting is disabled as normal user",
			Setting: pssDisabledSetting,
			OldNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			NewNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						psaApi.EnforceLevelLabel: string(psaApi.LevelBaseline),
					},
				},
			},
			Request: &types.Request{
				Request: &webhook.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						UserInfo: authenticationv1.UserInfo{
							Username: "demo",
						},
					},
				},
			},
			ExpectError: false,
		},
		{
			Name:    "update pss labels while setting is enabled as normal user when namespace is not whitelisted",
			Setting: pssEnabledSetting,
			OldNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			NewNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						psaApi.EnforceLevelLabel: string(psaApi.LevelBaseline),
					},
				},
			},
			Request: &types.Request{
				Request: &webhook.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						UserInfo: authenticationv1.UserInfo{
							Username: "demo",
						},
					},
				},
			},
			ExpectError: true,
		},
		{
			Name:    "update pss audit level while setting is enabled as normal user when namespace is not whitelisted",
			Setting: pssEnabledSetting,
			OldNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			NewNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						psaApi.AuditLevelLabel: string(psaApi.LevelBaseline),
					},
				},
			},
			Request: &types.Request{
				Request: &webhook.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						UserInfo: authenticationv1.UserInfo{
							Username: "demo",
						},
					},
				},
			},
			ExpectError: true,
		},
		{
			Name:    "attempt to remove pss labels while setting is enabled as normal user when namespace is not whitelisted",
			Setting: pssEnabledSetting,
			OldNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						psaApi.EnforceLevelLabel: string(psaApi.LevelBaseline),
					},
				},
			},
			NewNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			Request: &types.Request{
				Request: &webhook.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						UserInfo: authenticationv1.UserInfo{
							Username: "demo",
						},
					},
				},
			},
			ExpectError: true,
		},
		{
			Name:    "update pss labels while setting is enabled as harvester service account user",
			Setting: pssEnabledSetting,
			OldNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			NewNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						psaApi.EnforceLevelLabel: string(psaApi.LevelBaseline),
					},
				},
			},
			Request: &types.Request{
				Request: &webhook.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						UserInfo: authenticationv1.UserInfo{
							Username: harvesterSAName,
						},
					},
				},
			},
			ExpectError: false,
		},
		{
			Name:    "update pss labels while setting is enabled as normal user when test namespace is whitelisted",
			Setting: pssEnabledAdditionalTestNamespaceSetting,
			OldNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			NewNamespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						psaApi.EnforceLevelLabel: string(psaApi.LevelBaseline),
					},
				},
			},
			Request: &types.Request{
				Request: &webhook.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						UserInfo: authenticationv1.UserInfo{
							Username: "demo",
						},
					},
				},
			},
			ExpectError: false,
		},
	}

	// run update validation tests
	for _, tc := range testCases {
		harvesterClientSet := harvesterFake.NewSimpleClientset()
		settingsCache := fakeclients.HarvesterSettingCache(harvesterClientSet.HarvesterhciV1beta1().Settings)
		assert.NoError(harvesterClientSet.Tracker().Add(tc.Setting), "expected no error while adding setting definition to fake client")
		h := &namespaceValidator{
			settingsCache: settingsCache,
		}
		err := h.Update(tc.Request, tc.OldNamespace, tc.NewNamespace)
		if tc.ExpectError {
			assert.Error(err, "expected error to occur for test case", tc.Name)
		} else {
			assert.Nil(err, "expected no error to occur for test case", tc.Name)
		}
	}
}

func Test_NamespaceCreateValidation(t *testing.T) {
	assert := require.New(t)
	var testCases = []struct {
		Name        string
		Setting     *harvesterv1.Setting
		Namespace   *corev1.Namespace
		Request     *types.Request
		ExpectError bool
	}{
		{
			Name:    "update pss labels while setting is disabled as normal user",
			Setting: pssDisabledSetting,
			Namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						psaApi.EnforceLevelLabel: string(psaApi.LevelBaseline),
					},
				},
			},
			Request: &types.Request{
				Request: &webhook.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						UserInfo: authenticationv1.UserInfo{
							Username: "demo",
						},
					},
				},
			},
			ExpectError: false,
		},
		{
			Name:    "update pss labels while setting is enabled as normal user",
			Setting: pssEnabledSetting,
			Namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						psaApi.EnforceLevelLabel: string(psaApi.LevelBaseline),
					},
				},
			},
			Request: &types.Request{
				Request: &webhook.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						UserInfo: authenticationv1.UserInfo{
							Username: "demo",
						},
					},
				},
			},
			ExpectError: true,
		},
		{
			Name:    "update pss labels while setting is enabled as harvester service account user",
			Setting: pssEnabledSetting,
			Namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						psaApi.EnforceLevelLabel: string(psaApi.LevelBaseline),
					},
				},
			},
			Request: &types.Request{
				Request: &webhook.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						UserInfo: authenticationv1.UserInfo{
							Username: harvesterSAName,
						},
					},
				},
			},
			ExpectError: false,
		},
	}

	for _, tc := range testCases {
		harvesterClientSet := harvesterFake.NewSimpleClientset()
		settingsCache := fakeclients.HarvesterSettingCache(harvesterClientSet.HarvesterhciV1beta1().Settings)
		assert.NoError(harvesterClientSet.Tracker().Add(tc.Setting), "expected no error while adding setting definition to fake client")
		h := &namespaceValidator{
			settingsCache: settingsCache,
		}
		err := h.Create(tc.Request, tc.Namespace)
		if tc.ExpectError {
			assert.Error(err, "expected error to occur for test case", tc.Name)
		} else {
			assert.Nil(err, "expected no error to occur for test case", tc.Name)
		}
	}
}
