package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"time"

	semverv3 "github.com/Masterminds/semver/v3"
	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	rkev1 "github.com/rancher/rancher/pkg/apis/rke.cattle.io/v1"
	provisioningctrl "github.com/rancher/rancher/pkg/generated/controllers/provisioning.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/condition"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade/repoinfo"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	kubevirtctrl "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	upgradectlv1 "github.com/harvester/harvester/pkg/generated/controllers/upgrade.cattle.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/upgradehelper/versionguard"
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

	replicaReplenishmentWaitIntervalSetting  = "replica-replenishment-wait-interval"
	replicaReplenishmentAnnotation           = "harvesterhci.io/" + replicaReplenishmentWaitIntervalSetting
	extendedReplicaReplenishmentWaitInterval = 1800

	imageCleanupPlanCompletedAnnotation = "harvesterhci.io/image-cleanup-plan-completed"
	skipVersionCheckAnnotation          = "harvesterhci.io/skip-version-check"
	defaultImagePreloadConcurrency      = 1

	preUpgradeVMsAnnotation = "harvesterhci.io/pre-upgrade-vms"

	vmReady condition.Cond = "Ready"
)

type PreUpgradeRunningVM struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

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

	clusterClient provisioningctrl.ClusterClient
	clusterCache  provisioningctrl.ClusterCache

	lhSettingClient ctllhv1.SettingClient
	lhSettingCache  ctllhv1.SettingCache

	kubeVirtCache kubevirtctrl.KubeVirtCache

	vmRestClient rest.Interface
}

func (h *upgradeHandler) OnChanged(_ string, upgrade *harvesterv1.Upgrade) (*harvesterv1.Upgrade, error) {
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

		if err := h.storeVMState(toUpdate); err != nil {
			return nil, err
		}

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
			if err != nil && apierrors.IsAlreadyExists(err) {
				image, err = h.vmImageClient.Get(harvesterSystemNamespace, upgrade.Name, metav1.GetOptions{})
				if err != nil {
					setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, err.Error(), "")
					return h.upgradeClient.Update(toUpdate)
				}
				logrus.Infof("Reuse the existing image: %s/%s", image.Namespace, image.Name)
			} else if err != nil && !apierrors.IsAlreadyExists(err) {
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

	// clean upgrade repo VMs and images if a upgrade succeeds.
	if harvesterv1.UpgradeCompleted.IsTrue(upgrade) {
		// try to clean up images before purging the repo VM
		_, exists := upgrade.Annotations[imageCleanupPlanCompletedAnnotation]
		if exists {
			return nil, h.cleanup(upgrade, harvesterv1.UpgradeCompleted.IsTrue(upgrade))
		}

		// repo VM is required for the image cleaning procedure, bring it up if it's down
		logrus.Info("Try to start repo VM for image pruning")
		if err := repo.startVM(); err != nil {
			return upgrade, err
		}

		// moved the restoreVMState call after the start repo vm as we need to make sure kubevirt is up and running
		// thus we can bring up the vm to the state before upgrade
		h.restoreVMState(upgrade)

		if err := h.cleanupImages(upgrade, repo); err != nil {
			logrus.Warningf("Unable to cleanup images: %s", err.Error())
			toUpdate := upgrade.DeepCopy()
			toUpdate.Annotations = make(map[string]string)
			toUpdate.Annotations[imageCleanupPlanCompletedAnnotation] = strconv.FormatBool(true)
			return h.upgradeClient.Update(toUpdate)
		}

		return upgrade, nil
	}

	// upgrade failed
	if harvesterv1.UpgradeCompleted.IsFalse(upgrade) {
		// try to restore vm to the state before upgrade
		// we didn't wait for KubeVirt to reach the Deployed phase because the upgrade failed,
		// as a result, some services might not be ready and may never fully start.
		h.restoreVMState(upgrade)
		// clean upgrade repo VMs.
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

		// Upgrade Repo Info Retrieval
		backoff := wait.Backoff{
			Steps:    30,
			Duration: 10 * time.Second,
			Factor:   1.0,
			Jitter:   0.1,
		}
		var repoInfo *repoinfo.RepoInfo
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
		toUpdate.Status.RepoInfo = repoInfoStr

		// Upgrade Eligibility Check
		isEligible, reason := upgradeEligibilityCheck(upgrade)

		if !isEligible {
			setUpgradeCompletedCondition(toUpdate, StateFailed, corev1.ConditionFalse, reason, "")
			return h.upgradeClient.Update(toUpdate)
		}

		return h.prepareNodesForUpgrade(toUpdate, repoInfoStr)
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
			// save the original value of replica-replenishment-wait-interval setting and extend it with a longer value
			// skip if the value is already larger than extendedReplicaReplenishmentWaitInterval
			replicaReplenishmentWaitIntervalValue, err := h.getReplicaReplenishmentValue()
			if err != nil {
				return nil, err
			}
			if replicaReplenishmentWaitIntervalValue < extendedReplicaReplenishmentWaitInterval {
				if err := h.saveReplicaReplenishmentToUpgradeAnnotation(toUpdate); err != nil {
					return nil, err
				}
				if err := h.setReplicaReplenishmentValue(extendedReplicaReplenishmentWaitInterval); err != nil {
					return nil, err
				}
			}

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

func (h *upgradeHandler) cleanupImages(upgrade *harvesterv1.Upgrade, repo *Repo) error {
	toBePurgedImageList, err := repo.getImagesDiffList()
	if err != nil {
		return err
	}

	if len(toBePurgedImageList) == 0 {
		return fmt.Errorf("no images to be purged")
	}

	logrus.Info("Start purging unneeded container images on the nodes")
	if _, err := h.planClient.Create(prepareCleanupPlan(upgrade, toBePurgedImageList)); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (h *upgradeHandler) cleanup(upgrade *harvesterv1.Upgrade, cleanJobs bool) error {
	// delete vm and images
	repo := NewUpgradeRepo(h.ctx, upgrade, h)
	if err := repo.deleteVM(); err != nil {
		return err
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
			Registries:          clusterToUpdate.Spec.RKEConfig.Registries,
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

	// restore Longhorn replica-replenishment-wait-interval setting (multi-node cluster only)
	if upgrade.Status.SingleNode == "" {
		if err := h.loadReplicaReplenishmentFromUpgradeAnnotation(upgrade); err != nil {
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

func getCachedRepoInfo(upgrade *harvesterv1.Upgrade) (*repoinfo.RepoInfo, error) {
	repoInfo := &repoinfo.RepoInfo{}
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

func upgradeEligibilityCheck(upgrade *harvesterv1.Upgrade) (bool, string) {
	skipVersionCheckStr, ok := upgrade.Annotations[skipVersionCheckAnnotation]
	if ok {
		skipVersionCheck, err := strconv.ParseBool(skipVersionCheckStr)
		if err == nil && skipVersionCheck {
			logrus.Info("Skip minimum upgradable version check")
			return true, ""
		}
	}

	if err := versionguard.Check(upgrade, true, ""); err != nil {
		return false, err.Error()
	}

	return true, ""
}

func (h *upgradeHandler) prepareNodesForUpgrade(upgrade *harvesterv1.Upgrade, repoInfoStr string) (*harvesterv1.Upgrade, error) {
	upgradeConfig, err := settings.DecodeConfig[settings.UpgradeConfig](settings.UpgradeConfigSet.Get())
	if err != nil {
		return upgrade, err
	}
	logrus.WithFields(logrus.Fields{
		"namespace":      upgrade.Namespace,
		"name":           upgrade.Name,
		"upgrade_config": upgradeConfig,
	}).Info("start preparing nodes for upgrade")

	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return upgrade, err
	}

	var imagePreloadConcurrency int
	switch upgradeConfig.PreloadOption.Strategy.Type {
	case settings.SkipType:
		for _, node := range nodes {
			setNodeUpgradeStatus(upgrade, node.Name, nodeStateImagesPreloaded, "", "")
		}

		upgrade.Labels[upgradeStateLabel] = StatePreparingNodes
		upgrade.Status.RepoInfo = repoInfoStr
		setNodesPreparedCondition(upgrade, corev1.ConditionTrue, "", "")
		return h.upgradeClient.Update(upgrade)
	case settings.SequentialType:
		imagePreloadConcurrency = defaultImagePreloadConcurrency
	case settings.ParallelType:
		// Concurrency setting matters only when the strategy type is "parallel"
		imagePreloadConcurrency = upgradeConfig.PreloadOption.Strategy.Concurrency
		if imagePreloadConcurrency < 0 {
			return upgrade, fmt.Errorf("invalid image preload strategy concurrency: %d", imagePreloadConcurrency)
		} else if imagePreloadConcurrency == 0 || imagePreloadConcurrency > len(nodes) {
			// imagePreloadConcurrency is capped to the cluster's node count
			// setting the concurrency to 0 is a convenient way to always track the cluster's size
			imagePreloadConcurrency = len(nodes)
		}
	default:
		return upgrade, fmt.Errorf("invalid image preload strategy type: %s", upgradeConfig.PreloadOption.Strategy.Type)
	}

	if _, err := h.planClient.Create(preparePlan(upgrade, imagePreloadConcurrency)); err != nil && !apierrors.IsAlreadyExists(err) {
		setUpgradeCompletedCondition(upgrade, StateFailed, corev1.ConditionFalse, err.Error(), "")
		return h.upgradeClient.Update(upgrade)
	}

	upgrade.Labels[upgradeStateLabel] = StatePreparingNodes
	upgrade.Status.RepoInfo = repoInfoStr
	harvesterv1.NodesPrepared.CreateUnknownIfNotExists(upgrade)
	return h.upgradeClient.Update(upgrade)
}

func (h *upgradeHandler) getReplicaReplenishmentValue() (int, error) {
	replicaReplenishmentWaitInterval, err := h.lhSettingCache.Get(util.LonghornSystemNamespaceName, replicaReplenishmentWaitIntervalSetting)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(replicaReplenishmentWaitInterval.Value)
}

func (h *upgradeHandler) saveReplicaReplenishmentToUpgradeAnnotation(upgrade *harvesterv1.Upgrade) error {
	replicaReplenishmentWaitIntervalValue, err := h.getReplicaReplenishmentValue()
	if err != nil {
		return err
	}
	if upgrade.Annotations == nil {
		upgrade.Annotations = make(map[string]string)
	}
	upgrade.Annotations[replicaReplenishmentAnnotation] = strconv.Itoa(replicaReplenishmentWaitIntervalValue)
	return nil
}

func (h *upgradeHandler) loadReplicaReplenishmentFromUpgradeAnnotation(upgrade *harvesterv1.Upgrade) error {
	str, ok := upgrade.Annotations[replicaReplenishmentAnnotation]
	if !ok {
		logrus.Warn("no original replica-replenishment-wait-interval value set")
		return nil
	}
	value, err := strconv.Atoi(str)
	if err != nil {
		return err
	}
	return h.setReplicaReplenishmentValue(value)
}

func (h *upgradeHandler) setReplicaReplenishmentValue(value int) error {
	replicaReplenishmentWaitInterval, err := h.lhSettingCache.Get(util.LonghornSystemNamespaceName, replicaReplenishmentWaitIntervalSetting)
	if err != nil {
		return err
	}
	toUpdate := replicaReplenishmentWaitInterval.DeepCopy()
	toUpdate.Value = strconv.Itoa(value)
	if !reflect.DeepEqual(toUpdate, replicaReplenishmentWaitInterval) {
		if _, err := h.lhSettingClient.Update(toUpdate); err != nil {
			return err
		}
	}
	return nil
}

// storeVMState stores VM state only in single-node environments.
// this method stores information of VMs with Ready=true (i.e. running state) in the upgrade annotation
func (h *upgradeHandler) storeVMState(upgrade *harvesterv1.Upgrade) error {
	logFields := logrus.Fields{
		"namespace": upgrade.Namespace,
		"name":      upgrade.Name,
	}
	isSingleNodeRestoreVM, err := h.isSingleNodeRestoreVM(upgrade)
	if err != nil {
		return err
	}
	if !isSingleNodeRestoreVM {
		logrus.WithFields(logFields).Info("Skip store VM state")
		return nil
	}

	if upgrade.Annotations != nil && upgrade.Annotations[preUpgradeVMsAnnotation] != "" {
		logrus.WithFields(logFields).Infof("Skip store VM state since %s is already set", preUpgradeVMsAnnotation)
		return nil
	}

	vms, err := h.vmCache.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to list all VMs")
		return err
	}

	runningVMs := make([]*PreUpgradeRunningVM, 0)
	for _, vm := range vms {
		if vmReady.IsTrue(vm) {
			vm := &PreUpgradeRunningVM{
				Namespace: vm.Namespace,
				Name:      vm.Name,
			}
			runningVMs = append(runningVMs, vm)
		}
	}
	runningVMsJSON, err := json.Marshal(runningVMs)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to marshal PreUpgradeRunningVM list to json")
		return err
	}

	if upgrade.Annotations == nil {
		upgrade.Annotations = make(map[string]string)
	}
	upgrade.Annotations[preUpgradeVMsAnnotation] = string(runningVMsJSON)
	return nil
}

// restoreVMState attempts to restore all VMs to their pre-upgrade state.
// This function is designed for single-node environments. There is no guarantee
// that all VMs will successfully return to their previous state. If any VM fails to start,
// the error is logged, but the process continues without interruption.
func (h *upgradeHandler) restoreVMState(upgrade *harvesterv1.Upgrade) {
	logFields := logrus.Fields{
		"namespace": upgrade.Namespace,
		"name":      upgrade.Name,
	}
	// ignore the error here as the method should be no-op if there is
	// something wrong in isSingleNodeRestoreVM, and we want to proceed
	// to the next step: cleanup jobs, stop repo vm... after upgrade success/failed.
	isSingleNodeRestoreVM, _ := h.isSingleNodeRestoreVM(upgrade)
	if !isSingleNodeRestoreVM {
		logrus.WithFields(logFields).Info("Skip restore VM")
		return
	}

	if upgrade.Annotations == nil || upgrade.Annotations[preUpgradeVMsAnnotation] == "" {
		logrus.WithFields(logFields).Infof("Skip restore VM state since %s is not set", preUpgradeVMsAnnotation)
		return
	}

	preUpgradeRunningVMs := []PreUpgradeRunningVM{}
	err := json.Unmarshal([]byte(upgrade.Annotations[preUpgradeVMsAnnotation]), &preUpgradeRunningVMs)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to unmarshal json to PreUpgradeRunningVM list")
		return
	}

	for _, vmInfo := range preUpgradeRunningVMs {
		vm, err := h.vmCache.Get(vmInfo.Namespace, vmInfo.Name)
		if err != nil {
			logrus.WithFields(logFields).WithError(err).Errorf("Failed to get VM %s/%s from cache", vmInfo.Namespace, vmInfo.Name)
			continue
		}
		// the vm is already in running state
		if vmReady.IsTrue(vm) {
			continue
		}
		if err = h.startVM(context.Background(), vm); err != nil {
			logrus.WithFields(logFields).WithError(err).Errorf("Failed to start vm %s/%s after upgrade", vmInfo.Namespace, vmInfo.Name)
		}
	}
}

func (h *upgradeHandler) isSingleNodeRestoreVM(upgrade *harvesterv1.Upgrade) (bool, error) {
	singleNode, err := h.isSingleNodeCluster()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace": upgrade.Namespace,
			"name":      upgrade.Name,
		}).WithError(err).Error("Failed to check if cluster is single node")
		return false, err
	}
	upgradeConfig, err := settings.DecodeConfig[settings.UpgradeConfig](settings.UpgradeConfigSet.Get())
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace": upgrade.Namespace,
			"name":      upgrade.Name,
		}).WithError(err).Error("Failed to get UpgradeConfig")
		return false, err
	}
	return singleNode != "" && upgradeConfig.RestoreVM, nil
}

func (h *upgradeHandler) startVM(ctx context.Context, vm *kubevirtv1.VirtualMachine) error {
	body, err := json.Marshal(kubevirtv1.StartOptions{})
	if err != nil {
		return err
	}

	res := h.vmRestClient.Put().
		Namespace(vm.Namespace).
		Resource("virtualmachines").
		Name(vm.Name).
		SubResource("start").
		Body(body).
		Do(ctx)
	return res.Error()
}
