package indexeres

import (
	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"

	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	UserNameIndex           = "auth.harvester.cattle.io/user-username-index"
	RbByRoleAndSubjectIndex = "auth.harvester.cattle.io/crb-by-role-and-subject"
)

func RegisterScaledIndexers(scaled *config.Scaled) {
	informer := scaled.Management.HarvesterFactory.Harvester().V1alpha1().User().Cache()
	informer.AddIndexer(UserNameIndex, indexUserByUsername)
}

func RegisterManagementIndexers(management *config.Management) {
	informer := management.RbacFactory.Rbac().V1().ClusterRoleBinding().Cache()
	informer.AddIndexer(RbByRoleAndSubjectIndex, rbByRoleAndSubject)
}

func indexUserByUsername(obj *v1alpha1.User) ([]string, error) {
	return []string{obj.Username}, nil
}

func rbByRoleAndSubject(obj *rbacv1.ClusterRoleBinding) ([]string, error) {
	var keys []string
	for _, s := range obj.Subjects {
		keys = append(keys, RbRoleSubjectKey(obj.RoleRef.Name, s))
	}
	return keys, nil
}

func RbRoleSubjectKey(roleName string, subject rbacv1.Subject) string {
	return roleName + "." + subject.Kind + "." + subject.Name

}
