package setting

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	psaApi "k8s.io/pod-security-admission/api"

	"k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

var (
	ns1 = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ns1",
		},
	}
	kubeSystem = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}
	defaultNS = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
			Labels: map[string]string{
				psaApi.EnforceLevelLabel:   string(psaApi.LevelBaseline),
				psaApi.EnforceVersionLabel: psaApi.VersionLatest,
			},
		},
	}

	demoNS = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "demo",
			Labels: map[string]string{
				util.HarvesterManagedPSSKey: util.HarvesterManagedPSSValue,
				psaApi.EnforceLevelLabel:    string(psaApi.LevelBaseline),
				psaApi.EnforceVersionLabel:  psaApi.VersionLatest,
			},
		},
	}

	restrictedNS = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "restricted-ns",
		},
	}

	privilegedNS = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "privileged-ns",
		},
	}

	existingManagedPSSNS = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing-managed-ns",
			Labels: map[string]string{
				psaApi.EnforceLevelLabel:    string(psaApi.LevelBaseline),
				psaApi.EnforceVersionLabel:  psaApi.VersionLatest,
				util.HarvesterManagedPSSKey: util.HarvesterManagedPSSValue,
			},
		},
	}
)

func TestEnablePSSReconcile(t *testing.T) {
	assert := require.New(t)
	clientSet := fake.NewClientset(ns1, kubeSystem, defaultNS, restrictedNS, privilegedNS, demoNS)
	h := &Handler{
		namespaces:      fakeclients.NamespaceClient(clientSet.CoreV1().Namespaces),
		namespacesCache: fakeclients.NamespaceCache(clientSet.CoreV1().Namespaces),
	}

	pssSetting := &harvesterv1.Setting{
		Value: `{"enabled":true,"whitelistedNamespacesList":"default,demo", "restrictedNamespacesList": "demo,restricted-ns","privilegedNamespacesList":"demo2,privileged-ns"}`,
	}

	err := h.syncPodSecuritySetting(pssSetting)
	assert.NoError(err, "error while reconciling pss setting")

	// check if labels and keys get added
	nsObj, err := h.namespaces.Get(ns1.Name, metav1.GetOptions{})
	assert.NoError(err)
	assert.Equal(string(psaApi.LevelBaseline), nsObj.Labels[psaApi.EnforceLevelLabel], "expected to find pss enforcement key")

	// check default namespace does not change
	nsObj, err = h.namespaces.Get(defaultNS.Name, metav1.GetOptions{})
	assert.NoError(err)
	_, ok := nsObj.Labels[psaApi.EnforceLevelLabel]
	assert.True(ok, "expected to find enforcement key as this is manually managed")

	// check no changes are done to kube-system
	nsObj, err = h.namespaces.Get(kubeSystem.Name, metav1.GetOptions{})
	assert.NoError(err)
	_, ok = nsObj.Labels[psaApi.EnforceLevelLabel]
	assert.False(ok, "expected to not find enforcement key")

	// check restricted pss is applied
	restrictedNSObj, err := h.namespaces.Get(restrictedNS.Name, metav1.GetOptions{})
	assert.NoError(err)
	assert.Equal(string(psaApi.LevelRestricted), restrictedNSObj.Labels[psaApi.EnforceLevelLabel], "expected to find restricted enforcement key")

	// check privileged pss is applied
	privilegedNSObj, err := h.namespaces.Get(privilegedNS.Name, metav1.GetOptions{})
	assert.NoError(err)
	assert.Equal(string(psaApi.LevelPrivileged), privilegedNSObj.Labels[psaApi.EnforceLevelLabel], "expected to find privileged enforcement key")

	// check demo namespace is whitelisted
	nsObj, err = h.namespaces.Get(demoNS.Name, metav1.GetOptions{})
	assert.NoError(err)
	_, ok = nsObj.Labels[psaApi.EnforceLevelLabel]
	assert.False(ok, "expected to not find enforcement key")
}

func TestDisablePSSReconcile(t *testing.T) {
	assert := require.New(t)
	clientSet := fake.NewClientset(ns1, kubeSystem, defaultNS, restrictedNS, privilegedNS, existingManagedPSSNS)
	h := &Handler{
		namespaces:      fakeclients.NamespaceClient(clientSet.CoreV1().Namespaces),
		namespacesCache: fakeclients.NamespaceCache(clientSet.CoreV1().Namespaces),
	}

	pssSetting := &harvesterv1.Setting{
		Value: `{"enabled":false,"whitelistedNamespacesList":"default", "restrictedNamespacesList": "demo,restricted-ns","privilegedNamespacesList":"demo2,privileged-ns"}`,
	}

	err := h.syncPodSecuritySetting(pssSetting)
	assert.NoError(err, "error while reconciling pss setting")

	// check no baseline policy is added
	nsObj, err := h.namespaces.Get(ns1.Name, metav1.GetOptions{})
	assert.NoError(err)
	_, ok := nsObj.Labels[psaApi.EnforceLevelLabel]
	assert.False(ok, "expected to not find enforcement key")

	// check default remains untouched and existing PSS is not removed
	nsObj, err = h.namespaces.Get(defaultNS.Name, metav1.GetOptions{})
	assert.NoError(err)
	_, ok = nsObj.Labels[psaApi.EnforceLevelLabel]
	assert.True(ok, "expected to find enforcement key")

	// check no changes are done to kube-system
	nsObj, err = h.namespaces.Get(kubeSystem.Name, metav1.GetOptions{})
	assert.NoError(err)
	_, ok = nsObj.Labels[psaApi.EnforceLevelLabel]
	assert.False(ok, "expected to not find enforcement key")

	// check restricted pss is skipped
	restrictedNSObj, err := h.namespaces.Get(restrictedNS.Name, metav1.GetOptions{})
	assert.NoError(err)
	_, ok = restrictedNSObj.Labels[psaApi.EnforceLevelLabel]
	assert.False(ok, "expected to not find enforcement key")

	// check privileged pss is skipped
	privilegedNSObj, err := h.namespaces.Get(privilegedNS.Name, metav1.GetOptions{})
	assert.NoError(err)
	_, ok = privilegedNSObj.Labels[psaApi.EnforceLevelLabel]
	assert.False(ok, "expected to not find enforcement key")

	// check existing baseline policy is removed
	nsObj, err = h.namespaces.Get(existingManagedPSSNS.Name, metav1.GetOptions{})
	assert.NoError(err)
	_, ok = nsObj.Labels[psaApi.EnforceLevelLabel]
	assert.False(ok, "expected to not find enforcement key")
	_, ok = nsObj.Labels[psaApi.EnforceVersionLabel]
	assert.False(ok, "expected to not find enforcement key")
}
