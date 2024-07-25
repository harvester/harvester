package data

import (
	"github.com/rancher/wrangler/v3/pkg/apply"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/settings"
)

func addAuthenticatedRoles(apply apply.Apply) error {
	return apply.
		WithDynamicLookup().
		WithSetID("harvester-authenticated").
		ApplyObjects(
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "harvester-public",
					Namespace: publicNamespace,
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: rbacv1.GroupName,
					Kind:     "ClusterRole",
					Name:     "view",
				},
				Subjects: []rbacv1.Subject{
					{
						APIGroup: rbacv1.GroupName,
						Kind:     rbacv1.GroupKind,
						Name:     "system:authenticated",
					},
				},
			},
			&rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "harvester-authenticated",
				},
				Rules: []rbacv1.PolicyRule{
					{
						Verbs:         []string{"get", "watch"},
						APIGroups:     []string{"harvesterhci.io"},
						Resources:     []string{"settings"},
						ResourceNames: settings.WhiteListedSettings,
					},
					{
						Verbs:     []string{"get", "list", "watch"},
						APIGroups: []string{"network.harvesterhci.io"},
						Resources: []string{"clusternetworks"},
					},
					{
						Verbs:         []string{"get", "watch"},
						APIGroups:     []string{""},
						Resources:     []string{"namespaces"},
						ResourceNames: []string{publicNamespace},
					},
				},
			},
			&rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "harvester-authenticated",
				},
				Subjects: []rbacv1.Subject{{
					Kind:     "Group",
					APIGroup: rbacv1.GroupName,
					Name:     "system:authenticated",
				}},
				RoleRef: rbacv1.RoleRef{
					APIGroup: rbacv1.GroupName,
					Kind:     "ClusterRole",
					Name:     "harvester-authenticated",
				},
			},
		)
}
