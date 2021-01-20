package auth

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/settings"
	pkguser "github.com/rancher/harvester/pkg/user"
)

const (
	bootstrapAdminConfig   = "admincreated"
	defaultAdminLabelKey   = "auth.harvester.cattle.io/bootstrapping"
	defaultAdminLabelValue = "admin-user"
	usernameLabelKey       = "harvester.cattle.io/username"
	defaultAdminPassword   = "password"
)

var defaultAdminLabel = map[string]string{
	defaultAdminLabelKey: defaultAdminLabelValue,
}

// bootstrapAdmin checks if the bootstrapAdminConfig exists, if it does this indicates it has
// already created the admin user and should not attempt it again. Otherwise attempt to create the admin.
func BootstrapAdmin(mgmt *config.Management, namespace string) error {
	if settings.NoDefaultAdmin.Get() == "true" {
		return nil
	}

	set := labels.Set(defaultAdminLabel)
	admins, err := mgmt.HarvesterFactory.Harvester().V1alpha1().User().List(v1.ListOptions{LabelSelector: set.String()})
	if err != nil {
		return err
	}

	if len(admins.Items) > 0 {
		logrus.Info("Default admin already created, skip create admin step")
		return nil
	}

	if _, err := mgmt.CoreFactory.Core().V1().ConfigMap().Get(namespace, bootstrapAdminConfig, v1.GetOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			logrus.Warnf("Unable to determine if admin user already created: %v", err)
			return err
		}
	} else {
		// config map already exists, nothing to do
		return nil
	}

	users, err := mgmt.HarvesterFactory.Harvester().V1alpha1().User().List(v1.ListOptions{})
	if err != nil {
		return err
	}

	if len(users.Items) == 0 {
		// Options map does not exist and no users, attempt to create the default admin user
		hash, err := pkguser.HashPasswordString(defaultAdminPassword)
		if err != nil {
			return err
		}

		_, err = mgmt.HarvesterFactory.Harvester().V1alpha1().User().Create(&v1alpha1.User{
			ObjectMeta: v1.ObjectMeta{
				GenerateName: "user-",
				Labels:       defaultAdminLabel,
			},
			DisplayName: "Default Admin",
			Username:    "admin",
			Password:    string(hash),
			IsAdmin:     true,
		})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "can not ensure admin user exists")
		}

		users, err := mgmt.HarvesterFactory.Harvester().V1alpha1().User().List(v1.ListOptions{
			LabelSelector: set.String(),
		})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "failed list users")
		}

		bindings, err := mgmt.RbacFactory.Rbac().V1().ClusterRoleBinding().List(v1.ListOptions{LabelSelector: set.String()})
		if err != nil {
			return err
		}
		if len(bindings.Items) == 0 && len(users.Items) > 0 {
			_, err = mgmt.RbacFactory.Rbac().V1().ClusterRoleBinding().Create(
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: v1.ObjectMeta{
						GenerateName: "default-admin-",
						Labels: map[string]string{
							defaultAdminLabelKey: defaultAdminLabelValue,
							usernameLabelKey:     "admin",
						},
						OwnerReferences: []v1.OwnerReference{
							{
								APIVersion: v1alpha1.SchemeGroupVersion.String(),
								Kind:       "User",
								Name:       users.Items[0].Name,
								UID:        users.Items[0].UID,
							},
						},
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:     "User",
							APIGroup: rbacv1.GroupName,
							Name:     users.Items[0].Name,
						},
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: rbacv1.GroupName,
						Kind:     "ClusterRole",
						Name:     "cluster-admin",
					},
				})
			if err != nil {
				logrus.Warnf("Failed to create default admin global role binding: %v", err)
			} else {
				logrus.Info("Created default admin user and binding")
			}
		}
	}

	adminConfigMap := corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      bootstrapAdminConfig,
			Namespace: namespace,
		},
	}

	_, err = mgmt.CoreFactory.Core().V1().ConfigMap().Create(&adminConfigMap)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			logrus.Warnf("Error creating admin config map: %v", err)
		}

	}
	return nil
}
