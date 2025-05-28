package setting

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
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
				pssEnforcementKey:        pssEnforcementValue,
				pssEnforcementVersionKey: pssEnforcementVersionValue,
			},
		},
	}
)

func TestPSSReconcile(t *testing.T) {
	assert := require.New(t)
	clientSet := fake.NewClientset(ns1, kubeSystem, defaultNS)
	h := &Handler{
		namespaces:      fakeclients.NamespaceClient(clientSet.CoreV1().Namespaces),
		namespacesCache: fakeclients.NamespaceCache(clientSet.CoreV1().Namespaces),
	}

	pssSetting := &harvesterv1.Setting{
		Value: `{"enabled":true,"whitelistedNamespacesList":"default"}`,
	}

	err := h.syncPodSecuritySetting(pssSetting)
	assert.NoError(err, "error while reconciling pss setting")

	// check if labels and keys get added
	nsObj, err := h.namespaces.Get(ns1.Name, metav1.GetOptions{})
	assert.NoError(err)
	assert.Equal(pssEnforcementValue, nsObj.Labels[pssEnforcementKey], "expected to find pss enforcement key")

	// check default gets whitelisted
	nsObj, err = h.namespaces.Get(defaultNS.Name, metav1.GetOptions{})
	assert.NoError(err)
	_, ok := nsObj.Labels[pssEnforcementKey]
	assert.False(ok, "expected to not find enforcement key")

	// check no changes are done to kube-system
	nsObj, err = h.namespaces.Get(kubeSystem.Name, metav1.GetOptions{})
	assert.NoError(err)
	_, ok = nsObj.Labels[pssEnforcementKey]
	assert.False(ok, "expected to not find enforcement key")
}
