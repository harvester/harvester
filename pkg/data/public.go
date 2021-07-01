package data

import (
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/config"
)

const (
	publicNamespace = "harvester-public"
)

func createPublicNamespace(mgmtCtx *config.Management) error {
	namespaces := mgmtCtx.CoreFactory.Core().V1().Namespace()
	roleBindings := mgmtCtx.RbacFactory.Rbac().V1().RoleBinding()

	// Create harvester-public namespace
	if _, err := namespaces.Create(&v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: publicNamespace},
	}); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	// All authenticated users are readable in the public namespace
	if _, err := roleBindings.Create(&rbacv1.RoleBinding{
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
	}); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}
