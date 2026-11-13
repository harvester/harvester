package backup

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/util"
)

func TestJobAccess(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "backup-ns", UID: "backup-uid"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "restore-ns", UID: "restore-uid"}},
		// A valid namespace can also be a YAML boolean; templates must quote it.
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "true", UID: "boolean-uid"}},
	)
	for _, namespace := range []string{"backup-ns", "restore-ns", "true"} {
		require.NoError(t, EnsureJobAccess(ctx, client, namespace))
		client.ClearActions()
		require.NoError(t, EnsureJobAccess(ctx, client, namespace))
		for _, action := range client.Actions() {
			require.NotEqual(t, "update", action.GetVerb(), "repeated access setup must not update resources")
			require.NotEqual(t, "secrets", action.GetResource().Resource)
		}
		account, err := client.CoreV1().ServiceAccounts(namespace).Get(ctx, JobServiceAccount, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, namespace, account.Namespace)
		require.Equal(t, namespace, metav1.GetControllerOf(account).Name)
		require.False(t, ptr.Deref(account.AutomountServiceAccountToken, true))
		require.Empty(t, account.Secrets)
	}
	bindings := client.RbacV1().RoleBindings(util.LonghornSystemNamespaceName)
	list, err := bindings.List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list.Items, 3)
	for _, binding := range list.Items {
		owner := metav1.GetControllerOf(&binding)
		require.NotNil(t, owner)
		require.Equal(t, "Namespace", owner.Kind)
		require.Equal(t, "v1", owner.APIVersion)
		require.Equal(t, JobServiceAccount+"-"+string(owner.UID), binding.Name)
		require.Equal(t, rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName, Kind: "Role", Name: JobServiceAccount,
		}, binding.RoleRef)
		require.Equal(t, []rbacv1.Subject{{
			Kind: "ServiceAccount", Name: JobServiceAccount, Namespace: owner.Name,
		}}, binding.Subjects)
	}

	// Recover a deleted binding and repair changed subjects before a new Job.
	name := JobServiceAccount + "-backup-uid"
	require.NoError(t, bindings.Delete(ctx, name, metav1.DeleteOptions{}))
	require.NoError(t, EnsureJobAccess(ctx, client, "backup-ns"))
	binding, err := bindings.Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, err)
	binding.Subjects = []rbacv1.Subject{{Kind: "Group", Name: "system:authenticated"}}
	_, err = bindings.Update(ctx, binding, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.NoError(t, EnsureJobAccess(ctx, client, "backup-ns"))
	binding, err = bindings.Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, []rbacv1.Subject{{
		Kind: "ServiceAccount", Name: JobServiceAccount, Namespace: "backup-ns",
	}}, binding.Subjects)
}

func TestJobAccessRejectsConflicts(t *testing.T) {
	ctx := context.Background()
	for _, conflict := range []string{"account", "binding-owner", "binding-role", "terminating"} {
		t.Run(conflict, func(t *testing.T) {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default", UID: "namespace-uid"}}
			client := fake.NewSimpleClientset(ns)
			if conflict == "account" {
				_, err := client.CoreV1().ServiceAccounts(ns.Name).Create(ctx,
					&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: JobServiceAccount}}, metav1.CreateOptions{})
				require.NoError(t, err)
				require.ErrorContains(t, EnsureJobAccess(ctx, client, ns.Name), "different owner")
				return
			}
			if conflict == "terminating" {
				ns.DeletionTimestamp = ptr.To(metav1.Now())
				_, err := client.CoreV1().Namespaces().Update(ctx, ns, metav1.UpdateOptions{})
				require.NoError(t, err)
				require.ErrorContains(t, EnsureJobAccess(ctx, client, ns.Name), "terminating")
				return
			}
			require.NoError(t, EnsureJobAccess(ctx, client, ns.Name))
			bindings := client.RbacV1().RoleBindings(util.LonghornSystemNamespaceName)
			binding, err := bindings.Get(ctx, JobServiceAccount+"-"+string(ns.UID), metav1.GetOptions{})
			require.NoError(t, err)
			if conflict == "binding-owner" {
				binding.OwnerReferences[0].UID = types.UID("other")
			} else {
				binding.RoleRef.Name = "other"
			}
			_, err = bindings.Update(ctx, binding, metav1.UpdateOptions{})
			require.NoError(t, err)
			require.ErrorContains(t, EnsureJobAccess(ctx, client, ns.Name), "unexpected owner or role")
		})
	}
}
