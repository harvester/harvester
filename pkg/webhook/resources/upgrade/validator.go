package upgrade

import (
	"fmt"
	"strings"

	longhornv1beta1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	mgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	clusterv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade"
	ctlclusterv1 "github.com/harvester/harvester/pkg/generated/controllers/cluster.x-k8s.io/v1alpha4"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	upgradeStateLabel     = "harvesterhci.io/upgradeState"
	skipWebhookAnnotation = "harvesterhci.io/skipWebhook"
	managedChartNamespace = "fleet-local"
)

func NewValidator(
	upgrades ctlharvesterv1.UpgradeCache,
	nodes v1.NodeCache,
	lhVolumes ctllonghornv1.VolumeCache,
	clusters ctlclusterv1.ClusterCache,
	machines ctlclusterv1.MachineCache,
	managedChartCache mgmtv3.ManagedChartCache,
) types.Validator {
	return &upgradeValidator{
		upgrades:          upgrades,
		nodes:             nodes,
		lhVolumes:         lhVolumes,
		clusters:          clusters,
		machines:          machines,
		managedChartCache: managedChartCache,
	}
}

type upgradeValidator struct {
	types.DefaultValidator

	upgrades          ctlharvesterv1.UpgradeCache
	nodes             v1.NodeCache
	lhVolumes         ctllonghornv1.VolumeCache
	clusters          ctlclusterv1.ClusterCache
	machines          ctlclusterv1.MachineCache
	managedChartCache mgmtv3.ManagedChartCache
}

func (v *upgradeValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.UpgradeResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.Upgrade{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Delete,
		},
	}
}

func (v *upgradeValidator) Create(request *types.Request, newObj runtime.Object) error {
	newUpgrade := newObj.(*v1beta1.Upgrade)

	if newUpgrade.Spec.Version == "" && newUpgrade.Spec.Image == "" {
		return werror.NewBadRequest("version or image field are not specified.")
	}

	req, err := labels.NewRequirement(upgradeStateLabel, selection.NotIn, []string{upgrade.StateSucceeded, upgrade.StateFailed})
	if err != nil {
		return werror.NewBadRequest(fmt.Sprintf("%s label is already set as %s or %s", upgradeStateLabel, upgrade.StateSucceeded, upgrade.StateFailed))
	}

	upgrades, err := v.upgrades.List(newUpgrade.Namespace, labels.NewSelector().Add(*req))
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("can't list upgrades, err: %+v", err))
	}
	if len(upgrades) > 0 {
		msg := fmt.Sprintf("cannot proceed until previous upgrade %q completes", upgrades[0].Name)
		return werror.NewConflict(msg)
	}

	if newUpgrade.Annotations != nil {
		if skipWebhook, ok := newUpgrade.Annotations[skipWebhookAnnotation]; ok && strings.ToLower(skipWebhook) == "true" {
			return nil
		}
	}

	return v.checkResources()
}

func (v *upgradeValidator) checkResources() error {
	hasDegradedVolume, err := v.hasDegradedVolume()
	if err != nil {
		return werror.NewInternalError(err.Error())
	}

	if hasDegradedVolume {
		return werror.NewBadRequest("there are degraded volumes, please check all volumes are healthy")
	}

	cluster, err := v.clusters.Get(util.FleetLocalNamespaceName, util.LocalClusterName)
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("can't find %s/%s cluster, err: %+v", util.FleetLocalNamespaceName, util.LocalClusterName, err))
	}

	if cluster.Status.Phase != string(clusterv1alpha4.ClusterPhaseProvisioned) {
		return werror.NewBadRequest(fmt.Sprintf("cluster %s/%s status is %s, please wait for it to be provisioned", util.FleetLocalNamespaceName, util.LocalClusterName, cluster.Status.Phase))
	}

	if err := v.checkManagedCharts(); err != nil {
		return err
	}

	if err := v.checkNodes(); err != nil {
		return err
	}

	return v.checkMachines()
}

func (v *upgradeValidator) hasDegradedVolume() (bool, error) {
	nodes, err := v.nodes.List(labels.Everything())
	if err != nil {
		return false, err
	}

	if len(nodes) < 3 {
		return false, nil
	}

	volumes, err := v.lhVolumes.List(util.LonghornSystemNamespaceName, labels.Everything())
	if err != nil {
		return false, err
	}

	for _, volume := range volumes {
		if volume.Status.Robustness == longhornv1beta1.VolumeRobustnessDegraded {
			return true, nil
		}
	}

	return false, nil
}

func (v *upgradeValidator) checkManagedCharts() error {
	managedCharts, err := v.managedChartCache.List(managedChartNamespace, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("can't list managed charts, err: %+v", err))
	}

	for _, managedChart := range managedCharts {
		for _, condition := range managedChart.Status.Conditions {
			if condition.Type == fleetv1alpha1.BundleConditionReady {
				if condition.Status != corev1.ConditionTrue {
					return werror.NewBadRequest(fmt.Sprintf("managed chart %s is not ready, please wait for it to be ready", managedChart.Name))
				}
				break
			}
		}
	}

	return nil
}

func (v *upgradeValidator) checkNodes() error {
	nodes, err := v.nodes.List(labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("can't list nodes, err: %+v", err))
	}

	for _, node := range nodes {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady {
				if condition.Status != corev1.ConditionTrue {
					return werror.NewBadRequest(fmt.Sprintf("node %s is not ready, please wait for it to be ready", node.Name))
				}
				break
			}
		}

		if node.Spec.Unschedulable {
			return werror.NewBadRequest(fmt.Sprintf("node %s is unschedulable, please wait for it to be schedulable", node.Name))
		}
	}

	return nil
}

func (v *upgradeValidator) checkMachines() error {
	machines, err := v.machines.List(util.FleetLocalNamespaceName, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("can't list machines, err: %+v", err))
	}

	for _, machine := range machines {
		if machine.Status.GetTypedPhase() != clusterv1alpha4.MachinePhaseRunning {
			return werror.NewInternalError(fmt.Sprintf("machine %s/%s is not running", machine.Namespace, machine.Name))
		}
	}

	return nil
}

func (v *upgradeValidator) Delete(_ *types.Request, oldObj runtime.Object) error {
	oldUpgrade := oldObj.(*v1beta1.Upgrade)
	if oldUpgrade.Annotations != nil {
		if skipWebhook, ok := oldUpgrade.Annotations[skipWebhookAnnotation]; ok && strings.ToLower(skipWebhook) == "true" {
			return nil
		}
	}

	cluster, err := v.clusters.Get(util.FleetLocalNamespaceName, util.LocalClusterName)
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("can't find %s/%s cluster, err: %+v", util.FleetLocalNamespaceName, util.LocalClusterName, err))
	}

	if cluster.Status.Phase == string(clusterv1alpha4.ClusterPhaseProvisioning) {
		return werror.NewBadRequest(fmt.Sprintf("cluster %s/%s status is provisioning, please wait for it to be provisioned", util.FleetLocalNamespaceName, util.LocalClusterName))
	}

	return nil
}
