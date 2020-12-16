package user

import (
	ctlrbacv1 "github.com/rancher/wrangler-api/pkg/generated/controllers/rbac/v1"
	"github.com/sirupsen/logrus"
	k8srbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	ctlapisv1alpha1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/indexeres"
)

const (
	usernameLabelKey     = "harvester.cattle.io/username"
	adminRole            = "cluster-admin"
	publicInfoViewerRole = "system:public-info-viewer"
)

// userHandler reconcile clusterRole and clusterRoleBinding to k8s cluster
type userRBACHandler struct {
	users                   ctlapisv1alpha1.UserClient
	clusterRoleBindings     ctlrbacv1.ClusterRoleBindingClient
	clusterRoleBindingCache ctlrbacv1.ClusterRoleBindingCache
}

func (h *userRBACHandler) OnChanged(key string, user *apisv1alpha1.User) (*apisv1alpha1.User, error) {
	if user == nil || user.DeletionTimestamp != nil {
		return user, nil
	}

	roleName := publicInfoViewerRole
	if user.IsAdmin {
		roleName = adminRole
	}

	return user, h.ensureClusterBinding(roleName, user)
}

func buildSubjectFromUser(user *apisv1alpha1.User) k8srbacv1.Subject {
	return k8srbacv1.Subject{
		Kind: "User",
		Name: user.Name,
	}
}

func (h *userRBACHandler) ensureClusterBinding(roleName string, user *apisv1alpha1.User) error {
	subject := buildSubjectFromUser(user)
	key := indexeres.RbRoleSubjectKey(roleName, subject)
	crbs, err := h.clusterRoleBindingCache.GetByIndex(indexeres.RbByRoleAndSubjectIndex, key)
	if err != nil {
		return err
	}

	var existedCRB *k8srbacv1.ClusterRoleBinding
	var deleteCRB []*k8srbacv1.ClusterRoleBinding
	for _, crb := range crbs {
		var keepCurrent bool
		for _, sb := range crb.Subjects {
			iKey := indexeres.RbRoleSubjectKey(crb.RoleRef.Name, sb)
			if iKey == key {
				existedCRB = crb
				keepCurrent = true
				continue
			}
		}

		if _, ok := crb.Labels[usernameLabelKey]; ok && !keepCurrent {
			deleteCRB = append(deleteCRB, crb)
		}
	}

	for _, crb := range deleteCRB {
		if err := h.clusterRoleBindings.Delete(crb.Name, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	if existedCRB != nil {
		return nil
	}

	logrus.Infof("Creating clusterRoleBinding with role %v for subject %v", roleName, subject.Name)
	_, err = h.clusterRoleBindings.Create(&k8srbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "clusterrolebinding-",
			Labels: map[string]string{
				usernameLabelKey: user.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha1.SchemeGroupVersion.String(),
					Kind:       "User",
					Name:       user.Name,
					UID:        user.UID,
				},
			},
		},
		Subjects: []k8srbacv1.Subject{subject},
		RoleRef: k8srbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: roleName,
		},
	})

	return err
}
