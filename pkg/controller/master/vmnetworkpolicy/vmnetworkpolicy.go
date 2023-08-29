package vmnetworkpolicy

import (
	"context"
	"reflect"

	ctlrbacv1 "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/strings/slices"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
)

const (
	roleBindingName = "default-vmnetworkpolicy"
)

var (
	// list of ns to be skipping during rolebinding operation
	systemNamespaces = []string{"cattle-system", "fleet-local", "cattle-monitoring-system", "kube-system", "harvester-system", "rancher-vcluster", "cattle-dashboards", "cattle-fleet-clusters-system", "cattle-fleet-local-system", "cattle-impersonation-system", "cattle-logging-system", "kube-node-lease", "longhorn-system", "cattle-fleet-system"}
)

type handler struct {
	rbCache  ctlrbacv1.RoleBindingCache
	rbClient ctlrbacv1.RoleBindingClient
}

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	ns := management.CoreFactory.Core().V1().Namespace()
	rb := management.RbacFactory.Rbac().V1().RoleBinding()
	h := &handler{
		rbCache:  rb.Cache(),
		rbClient: rb,
	}

	ns.OnChange(ctx, "reconcile-default-rolebinding", h.reconcileNS)
	return nil
}

func (h *handler) reconcileNS(_ string, ns *corev1.Namespace) (*corev1.Namespace, error) {
	if ns == nil || ns.DeletionTimestamp != nil {
		return ns, nil
	}

	if slices.Contains(systemNamespaces, ns.Name) {
		logrus.Debugf("ns %s is a system namespace, ignoring", ns.Name)
		return ns, nil
	}

	requiredRole := generateRoleBinding(ns.Name)
	currentRoleBinding, err := h.rbCache.Get(ns.Name, roleBindingName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, err := h.rbClient.Create(requiredRole)
			return ns, err
		}
		return ns, err
	}

	if !reflect.DeepEqual(currentRoleBinding.Subjects, requiredRole.Subjects) || !reflect.DeepEqual(currentRoleBinding.RoleRef, requiredRole.RoleRef) {
		currentRoleBinding.RoleRef = requiredRole.RoleRef
		currentRoleBinding.Subjects = requiredRole.Subjects
		_, err := h.rbClient.Update(currentRoleBinding)
		return ns, err
	}

	return ns, nil
}

func generateRoleBinding(ns string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: ns,
		},
		RoleRef: rbacv1.RoleRef{
			Name:     harvesterv1beta1.SidecarClusterRoleName,
			Kind:     "ClusterRole",
			APIGroup: rbacv1.GroupName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      "default",
			Namespace: ns,
		}},
	}
}
