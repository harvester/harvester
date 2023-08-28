package vmnetworkpolicy

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_reconcileNS(t *testing.T) {

	assert := require.New(t)
	var testCases = []struct {
		name      string
		ns        *corev1.Namespace
		roleFound bool
	}{{
		name: "system namespace",
		ns: &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "fleet-local",
			},
		},
		roleFound: false,
	}, {
		name: "default namespace",
		ns: &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
		},
		roleFound: true,
	}}

	fakeClient := fake.NewSimpleClientset()
	rbacCache := fakeclients.RoleBindingCache(fakeClient.RbacV1().RoleBindings)
	rbacClient := fakeclients.RoleBindingClient(fakeClient.RbacV1().RoleBindings)
	h := &handler{
		rbClient: rbacClient,
		rbCache:  rbacCache,
	}

	for _, v := range testCases {
		_, err := h.reconcileNS("", v.ns)
		assert.NoError(err, "expect no error ns reconcile")
		_, err = rbacCache.Get(v.ns.Name, roleBindingName)
		if v.roleFound {
			assert.Nil(err, "expected no error during rolebinding lookup", v.name)
		} else {
			assert.True(apierrors.IsNotFound(err), "expected to not find rolebinding", v.name)
		}
	}
}
