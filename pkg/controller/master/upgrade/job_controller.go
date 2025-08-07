package upgrade

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	jobv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade/repoinfo"
	ctlclusterv1 "github.com/harvester/harvester/pkg/generated/controllers/cluster.x-k8s.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	upgradev1 "github.com/harvester/harvester/pkg/generated/controllers/upgrade.cattle.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	StateUpgrading               = "Upgrading"
	StatePreparingLoggingInfra   = "PreparingLoggingInfra"
	StateLoggingInfraPrepared    = "LoggingInfraPrepared"
	StateCreatingUpgradeImage    = "CreatingUpgradeImage"
	StatePreparingRepo           = "PreparingRepo"
	StateRepoPrepared            = "RepoPrepared"
	StatePreparingNodes          = "PreparingNodes"
	StateUpgradingSystemServices = "UpgradingSystemServices"
	StateUpgradingNodes          = "UpgradingNodes"
	StateSucceeded               = "Succeeded"
	StateFailed                  = "Failed"

	nodeStateImagesPreloading       = "Images preloading"
	nodeStateImagesPreloaded        = "Images preloaded"
	nodeStatePreDraining            = "Pre-draining"
	nodeStatePreDrained             = "Pre-drained"
	nodeStatePostDraining           = "Post-draining"
	nodeStateWaitingReboot          = "Waiting Reboot"
	upgradePlanLabel                = "upgrade.cattle.io/plan"
	upgradeNodeLabel                = "upgrade.cattle.io/node"
	upgradeStateLabel               = "harvesterhci.io/upgradeState"
	upgradeJobTypeLabel             = "harvesterhci.io/upgradeJobType"
	upgradeJobTypePreDrain          = "pre-drain"
	upgradeJobTypePostDrain         = "post-drain"
	upgradeJobTypeRestoreVM         = "restore-vm"
	upgradeJobTypeSingleNodeUpgrade = "single-node-upgrade"
)

// jobHandler syncs upgrade CRD status on upgrade job changes
type jobHandler struct {
	namespace     string
	planCache     upgradev1.PlanCache
	upgradeClient ctlharvesterv1.UpgradeClient
	upgradeCache  ctlharvesterv1.UpgradeCache

	machineCache   ctlclusterv1.MachineCache
	secretClient   ctlcorev1.SecretClient
	nodeClient     ctlcorev1.NodeClient
	nodeCache      ctlcorev1.NodeCache
	jobClient      jobv1.JobClient
	jobCache       jobv1.JobCache
	configMapCache ctlcorev1.ConfigMapCache
	settingCache   ctlharvesterv1.SettingCache
}

func (h *jobHandler) OnChanged(_ string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.DeletionTimestamp != nil || job.Labels == nil || (job.Namespace != upgradeNamespace && job.Namespace != sucNamespace) {
		return job, nil
	}

	upgradeControllerLock.Lock()
	defer upgradeControllerLock.Unlock()

	planName := job.Labels[upgradePlanLabel]
	nodeName := job.Labels[upgradeNodeLabel]

	switch {
	case planName != "" && nodeName != "":
		return h.syncPlanJob(job, planName, nodeName)
	case job.Labels[harvesterUpgradeComponentLabel] == nodeComponent:
		return h.syncNodeJob(job)
	case job.Labels[harvesterUpgradeComponentLabel] == manifestComponent:
		return h.syncManifestJob(job)
	}

	return job, nil
}

func (h *jobHandler) syncNodeJob(job *batchv1.Job) (*batchv1.Job, error) {
	jobType, ok := job.Labels[upgradeJobTypeLabel]
	if !ok {
		return nil, errors.New("sync a job without type")
	}

	nodeName, ok := job.Labels[harvesterNodeLabel]
	if !ok {
		return job, nil
	}

	node, err := h.nodeCache.Get(nodeName)
	if err != nil {
		return job, nil
	}

	machineName, ok := node.Annotations[clusterv1.MachineAnnotation]
	if !ok {
		return job, nil
	}

	upgradeName, ok := job.Labels[harvesterUpgradeLabel]
	if !ok {
		return job, nil
	}
	upgrade, err := h.upgradeCache.Get(h.namespace, upgradeName)
	if err != nil {
		return job, err
	}

	repoInfo, err := getCachedRepoInfo(upgrade)
	if err != nil {
		return job, err
	}

	toUpdate := upgrade.DeepCopy()

	preDrained := false
	postDrained := false
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == "True" {
			setNodeUpgradeStatus(toUpdate, nodeName, StateFailed, condition.Reason, condition.Message)
		} else if condition.Type == batchv1.JobComplete && condition.Status == "True" {
			nodeState := upgrade.Status.NodeStatuses[nodeName].State
			if jobType == upgradeJobTypePreDrain && nodeState == nodeStatePreDraining {
				logrus.Debugf("Pre-drain job %s is done.", job.Name)
				setNodeUpgradeStatus(toUpdate, nodeName, nodeStatePreDrained, "", "")
				preDrained = true
			} else if jobType == upgradeJobTypePostDrain && nodeState == nodeStatePostDraining {
				logrus.Debugf("Post-drain job %s is done.", job.Name)
				if repoInfo.Release.OS == node.Status.NodeInfo.OSImage {
					if err = h.sendRestoreVMJob(upgrade, node, repoInfo); err != nil {
						return job, err
					}
					setNodeUpgradeStatus(toUpdate, nodeName, StateSucceeded, "", "")
					postDrained = true
				} else {
					setNodeUpgradeStatus(toUpdate, nodeName, nodeStateWaitingReboot, "", "")
					if err := h.setNodeWaitRebootLabel(node, repoInfo); err != nil {
						return nil, err
					}
					// postDrain ack will be handled in node controller
				}
			} else if jobType == upgradeJobTypeSingleNodeUpgrade {
				logrus.Debugf("Single-node-upgrade job %s is done.", job.Name)
				if repoInfo.Release.OS == node.Status.NodeInfo.OSImage {
					if err = h.sendRestoreVMJob(upgrade, node, repoInfo); err != nil {
						return job, err
					}
					setNodeUpgradeStatus(toUpdate, nodeName, StateSucceeded, "", "")
				} else {
					setNodeUpgradeStatus(toUpdate, nodeName, nodeStateWaitingReboot, "", "")
					if err := h.setNodeWaitRebootLabel(node, repoInfo); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	if !reflect.DeepEqual(upgrade, toUpdate) {
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return job, err
		}
	}

	// find machine plan secret
	secrets, err := h.secretClient.List(rancherPlanSecretNamespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", rancherPlanSecretMachineLabel, machineName),
		FieldSelector: fmt.Sprintf("type=%s", rancherPlanSecretType),
	})

	if err != nil {
		return job, err
	}

	if len(secrets.Items) != 1 {
		return job, fmt.Errorf("found %d plan secret for machine %s", len(secrets.Items), machineName)
	}

	secret := secrets.Items[0]

	if preDrained {
		toUpdate := secret.DeepCopy()
		toUpdate.Annotations[preDrainAnnotation] = secret.Annotations[rke2PreDrainAnnotation]
		if _, err := h.secretClient.Update(toUpdate); err != nil {
			return nil, err
		}
	}

	if postDrained {
		toUpdate := secret.DeepCopy()
		toUpdate.Annotations[postDrainAnnotation] = secret.Annotations[rke2PostDrainAnnotation]
		if _, err := h.secretClient.Update(toUpdate); err != nil {
			return nil, err
		}
	}

	return job, nil
}

func (h *jobHandler) syncPlanJob(job *batchv1.Job, planName string, nodeName string) (*batchv1.Job, error) {
	plan, err := h.planCache.Get(sucNamespace, planName)
	if err != nil {
		return job, err
	}
	upgradeName, ok := plan.Labels[harvesterUpgradeLabel]
	if !ok {
		return job, nil
	}
	upgrade, err := h.upgradeCache.Get(h.namespace, upgradeName)
	if err != nil {
		return job, err
	}

	if upgrade.Labels[upgradeStateLabel] != StatePreparingNodes {
		return job, nil
	}

	toUpdate := upgrade.DeepCopy()

	if job.Status.Active > 0 {
		setNodeUpgradeStatus(toUpdate, nodeName, nodeStateImagesPreloading, "", "")
	}

	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == "True" {
			setNodeUpgradeStatus(toUpdate, nodeName, StateFailed, condition.Reason, condition.Message)
		} else if condition.Type == batchv1.JobComplete && condition.Status == "True" {
			setNodeUpgradeStatus(toUpdate, nodeName, nodeStateImagesPreloaded, "", "")
		}
	}
	if !reflect.DeepEqual(upgrade, toUpdate) {
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return job, err
		}
	}

	return job, nil
}

func (h *jobHandler) syncManifestJob(job *batchv1.Job) (*batchv1.Job, error) {
	upgradeName, ok := job.Labels[harvesterUpgradeLabel]
	if !ok {
		return job, nil
	}

	upgrade, err := h.upgradeCache.Get(h.namespace, upgradeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return job, nil
		}
		return job, err
	}

	if upgrade.Labels[harvesterLatestUpgradeLabel] != "true" {
		return job, nil
	}

	if !harvesterv1.SystemServicesUpgraded.IsUnknown(upgrade) || job.Status.Active > 0 {
		return job, nil
	}

	toUpdate := upgrade.DeepCopy()
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == "True" {
			setHelmChartUpgradeStatus(toUpdate, v1.ConditionFalse, condition.Reason, condition.Message)
		} else if condition.Type == batchv1.JobComplete && condition.Status == "True" {
			setHelmChartUpgradeStatus(toUpdate, v1.ConditionTrue, "", "")
		}
	}
	if !reflect.DeepEqual(upgrade, toUpdate) {
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return job, err
		}
	}

	return job, nil
}

func (h *jobHandler) setNodeWaitRebootLabel(node *v1.Node, repoInfo *repoinfo.RepoInfo) error {
	nodeUpdate := node.DeepCopy()
	nodeUpdate.Annotations[harvesterNodePendingOSImage] = repoInfo.Release.OS
	_, err := h.nodeClient.Update(nodeUpdate)
	return err
}

func (h *jobHandler) sendRestoreVMJob(upgrade *harvesterv1.Upgrade, node *v1.Node, repoInfo *repoinfo.RepoInfo) error {
	restoreVM, err := util.IsRestoreVM(h.settingCache)
	if err != nil {
		logrus.WithFields(logrus.Fields{"name": upgrade.Name, "node": node.Name}).WithError(err).
			Errorf("Failed to get setting UpgradeConfig, skip restore VM job")
		return nil
	}
	// skip if restore VM is not enabled
	if !restoreVM {
		return nil
	}
	// skip if the node is a witness node
	if _, found := node.Labels[util.HarvesterWitnessNodeLabelKey]; found {
		return nil
	}
	cmName := util.GetRestoreVMConfigMapName(upgrade.Name)
	// skip if there is no restore VM config map or the config map does not contain any VM names
	cm, err := h.configMapCache.Get(upgrade.Namespace, cmName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get ConfigMap %s: %w", cmName, err)
	}
	if cm.Data == nil || strings.TrimSpace(cm.Data[node.Name]) == "" {
		return nil
	}

	restoreVMJob := applyRestoreVMJob(upgrade, repoInfo, node.Name)
	_, err = h.jobCache.Get(restoreVMJob.Namespace, restoreVMJob.Name)
	if err == nil {
		// job already exists, no need to create it again
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get restore VM job for node %s: %w", node.Name, err)
	}
	// job does not exist, create it
	_, err = h.jobClient.Create(restoreVMJob)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create restore VM job for node %s: %w", node.Name, err)
	}
	logrus.WithFields(logrus.Fields{"upgrade": upgrade.Name, "node": node.Name}).
		Info("Restore VM job has been created successfully")
	return nil
}
