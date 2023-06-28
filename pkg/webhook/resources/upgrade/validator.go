package upgrade

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/docker/go-units"
	longhornv1beta1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	mgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	kubeletv1 "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
	clusterv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade"
	ctlclusterv1 "github.com/harvester/harvester/pkg/generated/controllers/cluster.x-k8s.io/v1alpha4"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	versionWebhook "github.com/harvester/harvester/pkg/webhook/resources/version"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	upgradeStateLabel              = "harvesterhci.io/upgradeState"
	skipWebhookAnnotation          = "harvesterhci.io/skipWebhook"
	rkeInternalIPAnnotation        = "rke2.io/internal-ip"
	managedChartNamespace          = "fleet-local"
	defaultMinFreeDiskSpace uint64 = 30 * 1024 * 1024 * 1024 // 30GB
	freeSystemPartitionMsg         = "df -h '/usr/local/'"
)

func NewValidator(
	upgrades ctlharvesterv1.UpgradeCache,
	nodes v1.NodeCache,
	lhVolumes ctllonghornv1.VolumeCache,
	clusters ctlclusterv1.ClusterCache,
	machines ctlclusterv1.MachineCache,
	managedChartCache mgmtv3.ManagedChartCache,
	versionCache ctlharvesterv1.VersionCache,
	httpClient *http.Client,
	bearToken string,
) types.Validator {
	return &upgradeValidator{
		upgrades:          upgrades,
		nodes:             nodes,
		lhVolumes:         lhVolumes,
		clusters:          clusters,
		machines:          machines,
		managedChartCache: managedChartCache,
		versionCache:      versionCache,
		httpClient:        httpClient,
		bearToken:         bearToken,
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
	versionCache      ctlharvesterv1.VersionCache
	httpClient        *http.Client
	bearToken         string
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

	var version *v1beta1.Version
	var err error
	if newUpgrade.Spec.Version != "" && newUpgrade.Spec.Image == "" {
		version, err = v.versionCache.Get(newUpgrade.Namespace, newUpgrade.Spec.Version)
		if err != nil {
			return werror.NewBadRequest(fmt.Sprintf("version %s is not found", newUpgrade.Spec.Version))
		}
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

	return v.checkResources(version)
}

func (v *upgradeValidator) checkResources(version *v1beta1.Version) error {
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

	if err := v.checkNodes(version); err != nil {
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

func (v *upgradeValidator) checkNodes(version *v1beta1.Version) error {
	nodes, err := v.nodes.List(labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("can't list nodes, err: %+v", err))
	}

	minFreeDiskSpace := defaultMinFreeDiskSpace
	if version != nil {
		if value, ok := version.Annotations[versionWebhook.MinFreeDiskSpaceGBAnnotation]; ok {
			v, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return werror.NewBadRequest(fmt.Sprintf("invalid value %s for %s annotation in version %s/%s", value, versionWebhook.MinFreeDiskSpaceGBAnnotation, version.Namespace, version.Name))
			}
			minFreeDiskSpace = v * 1024 * 1024 * 1024
		}
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

		if err := v.checkDiskSpace(node, minFreeDiskSpace); err != nil {
			return err
		}
	}

	return nil
}

func (v *upgradeValidator) checkDiskSpace(node *corev1.Node, minFreeDiskSpace uint64) error {
	internalIP, ok := node.Annotations[rkeInternalIPAnnotation]
	if !ok {
		return werror.NewInternalError(fmt.Sprintf("node %s doesn't have %s annotation", node.Name, rkeInternalIPAnnotation))
	}

	kubeletPort := node.Status.DaemonEndpoints.KubeletEndpoint.Port
	kubeletURL := fmt.Sprintf("https://%s:%d/stats/summary", internalIP, kubeletPort)
	req, err := http.NewRequest("GET", kubeletURL, nil)
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("node %s, can't make http.NewRequest to get %s, err: %+v", node.Name, kubeletURL, err))
	}
	req.Header.Set("Authorization", "Bearer "+v.bearToken)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("node %s, can't make http request to get %s, err: %+v", node.Name, kubeletURL, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return werror.NewInternalError(fmt.Sprintf("node %s, http response from %s is %d", node.Name, kubeletURL, resp.StatusCode))
	}

	var summary kubeletv1.Summary
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("node %s, can't read response from %s, err: %+v", node.Name, kubeletURL, err))
	}
	if err = json.Unmarshal(body, &summary); err != nil {
		return werror.NewInternalError(fmt.Sprintf("node %s, can't parse json response %s, err: %+v", node.Name, string(body), err))
	}
	if summary.Node.Fs.AvailableBytes == nil {
		return werror.NewInternalError(fmt.Sprintf("can't get node %s available bytes from %s, err: %+v", node.Name, kubeletURL, err))
	}
	if *summary.Node.Fs.AvailableBytes < minFreeDiskSpace {
		min := units.BytesSize(float64(minFreeDiskSpace))
		avail := units.BytesSize(float64(*summary.Node.Fs.AvailableBytes))
		return werror.NewBadRequest(fmt.Sprintf("Node %q has insufficient free system partition space %s (%s). The upgrade requires at least %s of free system partition space on each node.",
			node.Name, avail, freeSystemPartitionMsg, min))
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
