package user

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
)

const (
	userRbacControllerAgentName = "user-rbac-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	users := management.HarvesterFactory.Harvesterhci().V1beta1().User()

	userRBACController := &userRBACHandler{
		users:                   users,
		clusterRoleBindings:     management.RbacFactory.Rbac().V1().ClusterRoleBinding(),
		clusterRoleBindingCache: management.RbacFactory.Rbac().V1().ClusterRoleBinding().Cache(),
	}

	users.OnChange(ctx, userRbacControllerAgentName, userRBACController.OnChanged)
	return nil
}
