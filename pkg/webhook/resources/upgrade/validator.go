package upgrade

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/docker/go-units"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	mgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	kubeletconfigv1 "k8s.io/kubelet/config/v1beta1"
	kubeletstatsv1 "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade"
	ctlclusterv1 "github.com/harvester/harvester/pkg/generated/controllers/cluster.x-k8s.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
	versionWebhook "github.com/harvester/harvester/pkg/webhook/resources/version"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	upgradeStateLabel                         = "harvesterhci.io/upgradeState"
	skipWebhookAnnotation                     = "harvesterhci.io/skipWebhook"
	rkeInternalIPAnnotation                   = "rke2.io/internal-ip"
	managedChartNamespace                     = "fleet-local"
	defaultMinFreeDiskSpace            uint64 = 30 * 1024 * 1024 * 1024 // 30GB
	defaultNewImageSize                uint64 = 13 * 1024 * 1024 * 1024 // 13GB, this value aggregates all tarball image sizes. It may change in the future.
	defaultImageGCHighThresholdPercent        = 85.0                    // default value in kubelet config
	freeSystemPartitionMsg                    = "df -h '/usr/local/'"
)

func NewValidator(
	upgrades ctlharvesterv1.UpgradeCache,
	nodes v1.NodeCache,
	lhVolumes ctllhv1.VolumeCache,
	clusters ctlclusterv1.ClusterCache,
	machines ctlclusterv1.MachineCache,
	managedChartCache mgmtv3.ManagedChartCache,
	versionCache ctlharvesterv1.VersionCache,
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache,
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
		vmiCache:          vmiCache,
		httpClient:        httpClient,
		bearToken:         bearToken,
	}
}

type upgradeValidator struct {
	types.DefaultValidator

	upgrades          ctlharvesterv1.UpgradeCache
	nodes             v1.NodeCache
	lhVolumes         ctllhv1.VolumeCache
	clusters          ctlclusterv1.ClusterCache
	machines          ctlclusterv1.MachineCache
	managedChartCache mgmtv3.ManagedChartCache
	versionCache      ctlharvesterv1.VersionCache
	vmiCache          ctlkubevirtv1.VirtualMachineInstanceCache
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

func (v *upgradeValidator) Create(_ *types.Request, newObj runtime.Object) error {
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

	if cluster.Status.Phase != string(clusterv1.ClusterPhaseProvisioned) {
		return werror.NewBadRequest(fmt.Sprintf("cluster %s/%s status is %s, please wait for it to be provisioned", util.FleetLocalNamespaceName, util.LocalClusterName, cluster.Status.Phase))
	}

	if err := v.checkManagedCharts(); err != nil {
		return err
	}

	if err := v.checkNodes(version); err != nil {
		return err
	}

	if err := v.checkMachines(); err != nil {
		return err
	}

	if err := v.checkSingleReplicaVolumes(); err != nil {
		return err
	}

	return v.checkNonLiveMigratableVMs()
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
		if volume.Status.Robustness == lhv1beta2.VolumeRobustnessDegraded {
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
	kubeletURL := fmt.Sprintf("https://%s:%d", internalIP, kubeletPort)
	summary, err := v.getKubeletStatsSummary(node.Name, kubeletURL)
	if err != nil {
		return err
	}

	if summary.Node.Fs == nil || summary.Node.Fs.AvailableBytes == nil || summary.Node.Fs.CapacityBytes == nil || summary.Node.Fs.UsedBytes == nil {
		return werror.NewInternalError(fmt.Sprintf("can't get node %s filesystem stats from %s, err: %+v", node.Name, kubeletURL, err))
	}

	kubeletConfiguration, err := v.getKubeletConfigz(node.Name, kubeletURL)
	if err != nil {
		return err
	}

	imageGCHighThresholdPercent := defaultImageGCHighThresholdPercent
	if kubeletConfiguration.ImageGCHighThresholdPercent != nil {
		imageGCHighThresholdPercent = float64(*kubeletConfiguration.ImageGCHighThresholdPercent)
	}
	usedPercent := (float64(*summary.Node.Fs.UsedBytes+defaultNewImageSize) / float64(*summary.Node.Fs.CapacityBytes)) * 100.0
	logrus.Debugf("node %s uses %.3f%% storage space, kubelet image garbage collection threshold is %.3f%%", node.Name, usedPercent, imageGCHighThresholdPercent)

	if usedPercent > imageGCHighThresholdPercent {
		// Using strconv.FormatFloat to show imageGCHighThresholdPercent to trim zeros, because the default value is 0.85.
		return werror.NewBadRequest(fmt.Sprintf("Node %q will reach %.2f%% storage space after loading new images. It's higher than kubelet image garbage collection threshold %s%%.",
			node.Name, usedPercent, strconv.FormatFloat(imageGCHighThresholdPercent, 'f', -1, 64)))
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
		if machine.Status.GetTypedPhase() != clusterv1.MachinePhaseRunning {
			return werror.NewInternalError(fmt.Sprintf("machine %s/%s is not running", machine.Namespace, machine.Name))
		}
	}

	return nil
}

func (v *upgradeValidator) checkSingleReplicaVolumes() error {
	// Upgrade should be rejected if any single-replica volume exists
	nodes, err := v.nodes.List(labels.Everything())
	if err != nil {
		return err
	}

	// Skip for single-node cluster
	if len(nodes) == 1 {
		return nil
	}

	// Find all single-replica volumes
	singleReplicaVolumes, err := v.lhVolumes.GetByIndex(indexeres.VolumeByReplicaCountIndex, "1")
	if err != nil {
		return err
	}

	volumeNames := make([]string, 0, len(singleReplicaVolumes))
	for _, volume := range singleReplicaVolumes {
		switch volume.Status.State {
		case lhv1beta2.VolumeStateCreating, lhv1beta2.VolumeStateAttached, lhv1beta2.VolumeStateAttaching:
			pvcNamespace := volume.Status.KubernetesStatus.Namespace
			pvcName := volume.Status.KubernetesStatus.PVCName
			volumeNames = append(volumeNames, pvcNamespace+"/"+pvcName)
		}
	}

	if len(volumeNames) > 0 {
		return werror.NewInvalidError(
			fmt.Sprintf("The backing volumes of the following PVCs have single replica configured, please consider shutdown the corresponding workload before upgrade: %s", strings.Join(volumeNames, ", ")), "",
		)
	}

	return nil
}

func (v *upgradeValidator) checkNonLiveMigratableVMs() error {
	allVMIs, err := v.vmiCache.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		return err
	}

	allNodes, err := v.nodes.List(labels.Everything())
	if err != nil {
		return err
	}

	nonLiveMigratableVMNames, err := virtualmachineinstance.GetAllNonLiveMigratableVMINames(allVMIs, allNodes)
	if err != nil {
		return err
	}

	if len(nonLiveMigratableVMNames) > 0 {
		return werror.NewInvalidError(
			fmt.Sprintf("There are non-live migratable VMs that need to be shut off before initiating the upgrade: %s", strings.Join(nonLiveMigratableVMNames, ", ")), "",
		)
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

	if cluster.Status.Phase == string(clusterv1.ClusterPhaseProvisioning) {
		return werror.NewBadRequest(fmt.Sprintf("cluster %s/%s status is provisioning, please wait for it to be provisioned", util.FleetLocalNamespaceName, util.LocalClusterName))
	}

	return nil
}

func (v *upgradeValidator) getKubeletConfigz(nodeName, kubeletURL string) (*kubeletconfigv1.KubeletConfiguration, error) {
	url := fmt.Sprintf("%s/configz", kubeletURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, werror.NewInternalError(fmt.Sprintf("node %s, can't make http.NewRequest to get %s, err: %+v", nodeName, url, err))
	}
	req.Header.Set("Authorization", "Bearer "+v.bearToken)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, werror.NewInternalError(fmt.Sprintf("node %s, can't make http request to get %s, err: %+v", nodeName, url, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, werror.NewInternalError(fmt.Sprintf("node %s, http response from %s is %d", nodeName, url, resp.StatusCode))
	}

	kubeletConfiguration := &kubeletconfigv1.KubeletConfiguration{}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, werror.NewInternalError(fmt.Sprintf("node %s, can't read response from %s, err: %+v", nodeName, url, err))
	}
	if err = json.Unmarshal(body, kubeletConfiguration); err != nil {
		return nil, werror.NewInternalError(fmt.Sprintf("node %s, can't parse json response %s, err: %+v", nodeName, string(body), err))
	}
	return kubeletConfiguration, nil
}

func (v *upgradeValidator) getKubeletStatsSummary(nodeName, kubeletURL string) (*kubeletstatsv1.Summary, error) {
	url := fmt.Sprintf("%s/stats/summary", kubeletURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, werror.NewInternalError(fmt.Sprintf("node %s, can't make http.NewRequest to get %s, err: %+v", nodeName, url, err))
	}
	req.Header.Set("Authorization", "Bearer "+v.bearToken)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, werror.NewInternalError(fmt.Sprintf("node %s, can't make http request to get %s, err: %+v", nodeName, url, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, werror.NewInternalError(fmt.Sprintf("node %s, http response from %s is %d", nodeName, url, resp.StatusCode))
	}

	summary := &kubeletstatsv1.Summary{}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, werror.NewInternalError(fmt.Sprintf("node %s, can't read response from %s, err: %+v", nodeName, url, err))
	}
	if err = json.Unmarshal(body, summary); err != nil {
		return nil, werror.NewInternalError(fmt.Sprintf("node %s, can't parse json response %s, err: %+v", nodeName, string(body), err))
	}
	return summary, nil
}
