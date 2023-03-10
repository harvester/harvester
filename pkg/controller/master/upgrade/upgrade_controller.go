package upgrade

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	semverv3 "github.com/Masterminds/semver/v3"
	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	rkev1 "github.com/rancher/rancher/pkg/apis/rke.cattle.io/v1"
	provisioningctrl "github.com/rancher/rancher/pkg/generated/controllers/provisioning.cattle.io/v1"
	ctlappsv1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	kubevirtctrl "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	upgradectlv1 "github.com/harvester/harvester/pkg/generated/controllers/upgrade.cattle.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

var (
	upgradeControllerLock sync.Mutex
	rke2DrainNodes        = true
)

const (
	//system upgrade controller is deployed in cattle-system namespace
	upgradeNamespace               = "harvester-system"
	sucNamespace                   = "cattle-system"
	upgradeServiceAccount          = "system-upgrade-controller"
	harvesterSystemNamespace       = "harvester-system"
	harvesterUpgradeLabel          = "harvesterhci.io/upgrade"
	harvesterManagedLabel          = "harvesterhci.io/managed"
	harvesterLatestUpgradeLabel    = "harvesterhci.io/latestUpgrade"
	harvesterUpgradeComponentLabel = "harvesterhci.io/upgradeComponent"
	harvesterNodeLabel             = "harvesterhci.io/node"
	upgradeImageRepository         = "rancher/harvester-upgrade"

	harvesterNodePendingOSImage = "harvesterhci.io/pendingOSImage"

	preDrainAnnotation  = "harvesterhci.io/pre-hook"
	postDrainAnnotation = "harvesterhci.io/post-hook"

	rke2PreDrainAnnotation  = "rke.cattle.io/pre-drain"
	rke2PostDrainAnnotation = "rke.cattle.io/post-drain"

	upgradeComponentRepo = "repo"
)

// upgradeHandler Creates Plan CRDs to trigger upgrades
type upgradeHandler struct {
	ctx               context.Context
	namespace         string
	nodeCache         ctlcorev1.NodeCache
	jobClient         v1.JobClient
	jobCache          v1.JobCache
	upgradeClient     ctlharvesterv1.UpgradeClient
	upgradeCache      ctlharvesterv1.UpgradeCache
	upgradeController ctlharvesterv1.UpgradeController
	upgradeLogClient  ctlharvesterv1.UpgradeLogClient
	upgradeLogCache   ctlharvesterv1.UpgradeLogCache
	versionCache      ctlharvesterv1.VersionCache
	planClient        upgradectlv1.PlanClient
	planCache         upgradectlv1.PlanCache

	vmImageClient ctlharvesterv1.VirtualMachineImageClient
	vmImageCache  ctlharvesterv1.VirtualMachineImageCache
	vmClient      kubevirtctrl.VirtualMachineClient
	vmCache       kubevirtctrl.VirtualMachineCache
	serviceClient ctlcorev1.ServiceClient
	pvcClient     ctlcorev1.PersistentVolumeClaimClient

	clusterClient    provisioningctrl.ClusterClient
	clusterCache     provisioningctrl.ClusterCache
	deploymentClient ctlappsv1.DeploymentClient
}

func (h *upgradeHandler) OnChanged(key string, upgrade *harvesterv1.Upgrade) (*harvesterv1.Upgrade, error) {
	if upgrade == nil || upgrade.DeletionTimestamp != nil {
		return upgrade, nil
	}

	upgradeControllerLock.Lock()
	defer upgradeControllerLock.Unlock()

	repo := NewUpgradeRepo(h.ctx, upgrade, h)

	if harvesterv1.UpgradeCompleted.GetStatus(upgrade) == "" {
		logrus.Infof("Initialize upgrade %s/%s", upgrade.Namespace, upgrade.Name)

		if err := h.resetLatestUpgradeLabel(upgrade.Name); err != nil {
			return nil, err
		}

		toUpdate := upgrade.DeepCopy()
		initStatus(toUpdate)

		if !upgrade.Spec.LogEnabled {
			logrus.Info("Upgrade observability is administratively disabled")
			setLogReadyCondition(toUpdate, corev1.ConditionFalse, "Disabled", "Upgrade observability is administratively disabled")
			toUpdate.Labels[upgradeStateLabel] = StateLoggingInfraPrepared
			return h.upgradeClient.Update(toUpdate)
		}
		logrus.Info("Enabling upgrade observability")
		upgradeLog, err := h.upgradeLogClient.Create(prepareUpgradeLog(upgrade))
		if err != nil && !apierrors.IsAlreadyExists(err) {
			logrus.Warn("Failed to create the upgradeLog resource")
			setLogReadyCondition(toUpdate, corev1.ConditionFalse, err.Error(), "")
		} else {
			toUpdate.Status.UpgradeLog = upgradeLog.Name
		}
		harvesterv1.LogReady.CreateUnknownIfNotExists(toUpdate)
		return h.upgradeClient.Update(toUpdate)
	}

	if (harvesterv1.LogReady.IsTrue(upgrade) || harvesterv1.LogReady.IsFalse(upgrade)) && harvesterv1.ImageReady.GetStatus(upgrade) == "" {
		logrus.Info("Creating upgrade repo image")
		toUpdate := upgrade.DeepCopy()

		if upgrade.Spec.Image == "" {
			version, err := h.versionCache.Get(h.namespace, upgrade.Spec.Version)
			if err != nil {
				setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
				return h.upgradeClient.Update(toUpdate)
			}

			image, err := repo.CreateImageFromISO(version.Spec.ISOURL, version.Spec.ISOChecksum)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
				return h.upgradeClient.Update(toUpdate)
			}
			toUpdate.Status.ImageID = fmt.Sprintf("%s/%s", image.Namespace, image.Name)
		} else {
			image, err := repo.GetImage(upgrade.Spec.Image)
			if err != nil {
				setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
				return h.upgradeClient.Update(toUpdate)
			}
			toUpdate.Status.ImageID = fmt.Sprintf("%s/%s", image.Namespace, image.Name)

			// The image might not be imported yet. Set upgrade label and let
			// vmImageHandler deal with it.
			imageUpdate := image.DeepCopy()
			if imageUpdate.Labels == nil {
				imageUpdate.Labels = map[string]string{}
			}
			imageUpdate.Labels[harvesterUpgradeLabel] = upgrade.Name
			if _, err := h.vmImageClient.Update(imageUpdate); err != nil {
				return nil, err
			}
		}
		harvesterv1.ImageReady.CreateUnknownIfNotExists(toUpdate)
		return h.upgradeClient.Update(toUpdate)
	}

	logrus.Infof("handle upgrade %s/%s with labels %v", upgrade.Namespace, upgrade.Name, upgrade.Labels)

	// only run further operations for latest upgrade
	if upgrade.Labels == nil || upgrade.Labels[harvesterLatestUpgradeLabel] != "true" {
		return upgrade, nil
	}

	// clean upgrade repo VMs and images if a upgrade succeeds or fails.
	if harvesterv1.UpgradeCompleted.IsTrue(upgrade) || harvesterv1.UpgradeCompleted.IsFalse(upgrade) {
		return nil, h.cleanup(upgrade, harvesterv1.UpgradeCompleted.IsTrue(upgrade))
	}

	if harvesterv1.ImageReady.IsTrue(upgrade) && harvesterv1.RepoProvisioned.GetStatus(upgrade) == "" {
		logrus.Info("Starting upgrade repo VM")
		toUpdate := upgrade.DeepCopy()
		if err := repo.Bootstrap(); err != nil && !apierrors.IsAlreadyExists(err) {
			setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
			return h.upgradeClient.Update(toUpdate)
		}
		toUpdate.Labels[upgradeStateLabel] = StatePreparingRepo
		harvesterv1.RepoProvisioned.CreateUnknownIfNotExists(toUpdate)
		return h.upgradeClient.Update(toUpdate)
	}

	if harvesterv1.RepoProvisioned.IsTrue(upgrade) && harvesterv1.NodesPrepared.GetStatus(upgrade) == "" {
		toUpdate := upgrade.DeepCopy()
		singleNode, err := h.isSingleNodeCluster()
		if err != nil {
			return nil, err
		}
		toUpdate.Status.SingleNode = singleNode

		backoff := wait.Backoff{
			Steps:    30,
			Duration: 10 * time.Second,
			Factor:   1.0,
			Jitter:   0.1,
		}
		var repoInfo *RepoInfo
		if err := retry.OnError(backoff, util.IsRetriableNetworkError, func() error {
			repoInfo, err = repo.getInfo()
			if err != nil {
				logrus.Warnf("Repo info retrieval failed with: %s", err)
				return err
			}
			return nil
		}); err != nil {
			setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
			return h.upgradeClient.Update(toUpdate)
		}

		repoInfoStr, err := repoInfo.Marshall()
		if err != nil {
			setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
			return h.upgradeClient.Update(toUpdate)
		}

		logrus.Info("Check minimum upgradable version")
		if err := isVersionUpgradable(toUpdate.Status.PreviousVersion, repoInfo.Release.MinUpgradableVersion); err != nil {
			setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
			return h.upgradeClient.Update(toUpdate)
		}

		logrus.Debug("Start preparing nodes for upgrade")
		if _, err := h.planClient.Create(preparePlan(upgrade)); err != nil && !apierrors.IsAlreadyExists(err) {
			setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
			return h.upgradeClient.Update(toUpdate)
		}

		toUpdate.Labels[upgradeStateLabel] = StatePreparingNodes
		toUpdate.Status.RepoInfo = repoInfoStr
		harvesterv1.NodesPrepared.CreateUnknownIfNotExists(toUpdate)
		return h.upgradeClient.Update(toUpdate)
	}

	if harvesterv1.NodesPrepared.IsTrue(upgrade) && harvesterv1.SystemServicesUpgraded.GetStatus(upgrade) == "" {
		toUpdate := upgrade.DeepCopy()
		repoInfo, err := getCachedRepoInfo(upgrade)
		if err != nil {
			setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
			return h.upgradeClient.Update(toUpdate)
		}

		if _, err := h.jobClient.Create(applyManifestsJob(upgrade, repoInfo)); err != nil && !apierrors.IsAlreadyExists(err) {
			setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
			return h.upgradeClient.Update(toUpdate)
		}
		toUpdate.Labels[upgradeStateLabel] = StateUpgradingSystemServices
		setHelmChartUpgradeStatus(toUpdate, corev1.ConditionUnknown, "", "")
		return h.upgradeClient.Update(toUpdate)
	}

	if harvesterv1.SystemServicesUpgraded.IsTrue(upgrade) && harvesterv1.NodesUpgraded.GetStatus(upgrade) == "" {
		info, err := getCachedRepoInfo(upgrade)
		if err != nil {
			return nil, err
		}

		toUpdate := upgrade.DeepCopy()
		singleNodeName := upgrade.Status.SingleNode
		if singleNodeName != "" {
			logrus.Info("Start single node upgrade job")
			if _, err = h.jobClient.Create(applyNodeJob(upgrade, info, singleNodeName, upgradeJobTypeSingleNodeUpgrade)); err != nil && !apierrors.IsAlreadyExists(err) {
				setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
				return h.upgradeClient.Update(toUpdate)
			}
		} else {
			// go with RKE2 pre-drain/post-drain hooks
			logrus.Infof("Start upgrading Kubernetes runtime to %s", info.Release.Kubernetes)
			if err := h.upgradeKubernetes(info.Release.Kubernetes); err != nil {
				setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
				return h.upgradeClient.Update(toUpdate)
			}
		}

		toUpdate.Labels[upgradeStateLabel] = StateUpgradingNodes
		harvesterv1.NodesUpgraded.CreateUnknownIfNotExists(toUpdate)
		return h.upgradeClient.Update(toUpdate)
	}

	return upgrade, nil
}

func (h *upgradeHandler) OnRemove(_ string, upgrade *harvesterv1.Upgrade) (*harvesterv1.Upgrade, error) {
	if upgrade == nil {
		return nil, nil
	}

	logrus.Debugf("Deleting upgrade %s", upgrade.Name)
	return upgrade, h.cleanup(upgrade, true)
}

func (h *upgradeHandler) cleanup(upgrade *harvesterv1.Upgrade, cleanJobs bool) error {
	// delete vm and images
	repo := NewUpgradeRepo(h.ctx, upgrade, h)
	if err := repo.deleteVM(); err != nil {
		return err
	}

	// For single node case, we scale fleet agent deployment replicas to 0 before upgrading RKE2.
	// Restore the replica back when upgrade is done.
	if upgrade.Status.SingleNode != "" {
		fleetAgentDP, err := h.deploymentClient.Get("cattle-fleet-local-system", "fleet-agent", metav1.GetOptions{})
		if err != nil {
			return err
		}
		logrus.Debugf("Replicas of fleet agent deployment: %d", *fleetAgentDP.Spec.Replicas)
		if fleetAgentDP.Spec.Replicas != nil && *fleetAgentDP.Spec.Replicas == 0 {
			logrus.Infof("Scale replicas of fleet agent deployment to 1")
			toUpdate := fleetAgentDP.DeepCopy()
			*toUpdate.Spec.Replicas = 1
			if _, err := h.deploymentClient.Update(toUpdate); err != nil {
				return err
			}
		}
	}

	// remove rkeConfig in fleet-local/local cluster
	cluster, err := h.clusterCache.Get("fleet-local", "local")
	if err != nil {
		return err
	}
	clusterToUpdate := cluster.DeepCopy()
	provisionGeneration := clusterToUpdate.Spec.RKEConfig.ProvisionGeneration
	clusterToUpdate.Spec.RKEConfig = &provisioningv1.RKEConfig{
		RKEClusterSpecCommon: rkev1.RKEClusterSpecCommon{
			ProvisionGeneration: provisionGeneration,
		},
	}
	logrus.Infof("Reset RKEConfig and set provisionGeneration to %d", provisionGeneration)
	if !reflect.DeepEqual(clusterToUpdate, cluster) {
		logrus.Info("Update cluster fleet-local/local")
		if _, err := h.clusterClient.Update(clusterToUpdate); err != nil {
			return err
		}
	}

	// SUC plans are in other namespaces, we need to delete them manually.
	sets := labels.Set{
		harvesterUpgradeLabel: upgrade.Name,
	}
	plans, err := h.planCache.List(sucNamespace, sets.AsSelector())
	if err != nil {
		return err
	}

	// clean jobs and plans
	for _, plan := range plans {
		if cleanJobs {
			set := labels.Set{
				upgradePlanLabel: plan.Name,
			}
			jobs, err := h.jobCache.List(plan.Namespace, set.AsSelector())
			if err != nil {
				return err
			}
			for _, job := range jobs {
				logrus.Debugf("Deleting job %s/%s", job.Namespace, job.Name)
				if err := h.jobClient.Delete(job.Namespace, job.Name, &metav1.DeleteOptions{}); err != nil {
					return err
				}
			}
		}

		logrus.Debugf("Deleting plan %s/%s", plan.Namespace, plan.Name)
		if err := h.planClient.Delete(plan.Namespace, plan.Name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	// tear down logging infra if any
	if harvesterv1.LogReady.IsTrue(upgrade) && upgrade.Status.UpgradeLog != "" {
		upgradeLog, err := h.upgradeLogCache.Get(upgradeNamespace, upgrade.Status.UpgradeLog)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		upgradeLogToUpdate := upgradeLog.DeepCopy()
		harvesterv1.UpgradeEnded.SetStatus(upgradeLogToUpdate, string(corev1.ConditionTrue))
		harvesterv1.UpgradeEnded.Reason(upgradeLogToUpdate, "")
		harvesterv1.UpgradeEnded.Message(upgradeLogToUpdate, "")
		if !reflect.DeepEqual(upgradeLogToUpdate, upgradeLog) {
			logrus.Infof("Update upgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)
			if _, err := h.upgradeLogClient.Update(upgradeLogToUpdate); err != nil {
				return err
			}
		}
	}

	return nil
}

func (h *upgradeHandler) isSingleNodeCluster() (string, error) {
	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return "", err
	}
	if len(nodes) == 1 {
		return nodes[0].Name, nil

	}
	return "", nil
}

func initStatus(upgrade *harvesterv1.Upgrade) {
	harvesterv1.UpgradeCompleted.CreateUnknownIfNotExists(upgrade)
	if upgrade.Labels == nil {
		upgrade.Labels = make(map[string]string)
	}
	upgrade.Labels[upgradeStateLabel] = StatePreparingLoggingInfra
	upgrade.Labels[harvesterLatestUpgradeLabel] = "true"
	upgrade.Status.PreviousVersion = settings.ServerVersion.Get()
}

func (h *upgradeHandler) resetLatestUpgradeLabel(latestUpgradeName string) error {
	sets := labels.Set{
		harvesterLatestUpgradeLabel: "true",
	}
	upgrades, err := h.upgradeCache.List(h.namespace, sets.AsSelector())
	if err != nil {
		return err
	}
	for _, upgrade := range upgrades {
		if upgrade.Name == latestUpgradeName {
			continue
		}
		toUpdate := upgrade.DeepCopy()
		delete(toUpdate.Labels, harvesterLatestUpgradeLabel)
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return err
		}
	}
	return nil
}

func (h *upgradeHandler) upgradeKubernetes(kubernetesVersion string) error {
	cluster, err := h.clusterCache.Get("fleet-local", "local")
	if err != nil {
		return err
	}

	toUpdate := cluster.DeepCopy()
	toUpdate.Spec.KubernetesVersion = kubernetesVersion

	if toUpdate.Spec.RKEConfig == nil {
		toUpdate.Spec.RKEConfig = &provisioningv1.RKEConfig{}
	}

	toUpdate.Spec.RKEConfig.ProvisionGeneration++
	toUpdate.Spec.RKEConfig.UpgradeStrategy.ControlPlaneConcurrency = "1"
	toUpdate.Spec.RKEConfig.UpgradeStrategy.WorkerConcurrency = "1"
	toUpdate.Spec.RKEConfig.UpgradeStrategy.ControlPlaneDrainOptions.DeleteEmptyDirData = rke2DrainNodes
	toUpdate.Spec.RKEConfig.UpgradeStrategy.ControlPlaneDrainOptions.Enabled = rke2DrainNodes
	toUpdate.Spec.RKEConfig.UpgradeStrategy.ControlPlaneDrainOptions.Force = rke2DrainNodes
	toUpdate.Spec.RKEConfig.UpgradeStrategy.ControlPlaneDrainOptions.IgnoreDaemonSets = &rke2DrainNodes
	toUpdate.Spec.RKEConfig.UpgradeStrategy.WorkerDrainOptions.DeleteEmptyDirData = rke2DrainNodes
	toUpdate.Spec.RKEConfig.UpgradeStrategy.WorkerDrainOptions.Enabled = rke2DrainNodes
	toUpdate.Spec.RKEConfig.UpgradeStrategy.WorkerDrainOptions.Force = rke2DrainNodes
	toUpdate.Spec.RKEConfig.UpgradeStrategy.WorkerDrainOptions.IgnoreDaemonSets = &rke2DrainNodes

	updateDrainHooks(&toUpdate.Spec.RKEConfig.UpgradeStrategy.ControlPlaneDrainOptions.PreDrainHooks, preDrainAnnotation)
	updateDrainHooks(&toUpdate.Spec.RKEConfig.UpgradeStrategy.ControlPlaneDrainOptions.PostDrainHooks, postDrainAnnotation)
	updateDrainHooks(&toUpdate.Spec.RKEConfig.UpgradeStrategy.WorkerDrainOptions.PreDrainHooks, preDrainAnnotation)
	updateDrainHooks(&toUpdate.Spec.RKEConfig.UpgradeStrategy.WorkerDrainOptions.PostDrainHooks, postDrainAnnotation)

	_, err = h.clusterClient.Update(toUpdate)
	return err
}

func updateDrainHooks(hooks *[]rkev1.DrainHook, annotation string) {
	for _, hook := range *hooks {
		if hook.Annotation == annotation {
			return
		}
	}

	*hooks = append(*hooks, rkev1.DrainHook{
		Annotation: annotation,
	})
}

func ensureSingleUpgrade(namespace string, upgradeCache ctlharvesterv1.UpgradeCache) (*harvesterv1.Upgrade, error) {
	sets := labels.Set{
		harvesterLatestUpgradeLabel: "true",
	}

	onGoingUpgrades, err := upgradeCache.List(namespace, sets.AsSelector())
	if err != nil {
		return nil, err
	}

	if len(onGoingUpgrades) != 1 {
		return nil, fmt.Errorf("There are %d on-going upgrades", len(onGoingUpgrades))
	}

	return onGoingUpgrades[0], nil
}

func getCachedRepoInfo(upgrade *harvesterv1.Upgrade) (*RepoInfo, error) {
	repoInfo := &RepoInfo{}
	if err := repoInfo.Load(upgrade.Status.RepoInfo); err != nil {
		return nil, err
	}
	return repoInfo, nil
}

func isVersionUpgradable(currentVersion, minUpgradableVersion string) error {
	if minUpgradableVersion == "" {
		logrus.Debug("No minimum upgradable version specified, continue the upgrading")
		return nil
	}

	// short-circuit the equal cases as the library doesn't support the hack applied below
	if currentVersion == minUpgradableVersion {
		logrus.Debug("Upgrade from the exact same version as the minimum requirement")
		return nil
	}
	// to enable comparisons against prerelease versions
	constraint := fmt.Sprintf(">= %s-z", minUpgradableVersion)

	c, err := semverv3.NewConstraint(constraint)
	if err != nil {
		return err
	}
	v, err := semverv3.NewVersion(currentVersion)
	if err != nil {
		return err
	}

	if a := c.Check(v); !a {
		message := fmt.Sprintf("The current version %s is less than the minimum upgradable version %s.", currentVersion, minUpgradableVersion)
		return fmt.Errorf("%s", message)
	}

	return nil
}
