package upgrade

import (
	"fmt"
	"time"

	jobV1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlclusterv1 "github.com/harvester/harvester/pkg/generated/controllers/cluster.x-k8s.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
)

const (
	rancherMachineNamespace       = "fleet-local"
	rancherPlanSecretNamespace    = "fleet-local"
	rancherPlanSecretType         = "rke.cattle.io/machine-plan"
	rancherPlanSecretMachineLabel = "rke.cattle.io/machine-name"
)

// secretHandler watches pre-drain and pos-drain annotations set by Rancher and create corresponding node jobs
type secretHandler struct {
	namespace        string
	upgradeClient    ctlharvesterv1.UpgradeClient
	upgradeCache     ctlharvesterv1.UpgradeCache
	jobClient        jobV1.JobClient
	jobCache         jobV1.JobCache
	machineCache     ctlclusterv1.MachineCache
	secretController ctlcorev1.SecretController
}

func (h *secretHandler) OnChanged(_ string, secret *v1.Secret) (*v1.Secret, error) {
	if secret == nil || secret.DeletionTimestamp != nil || secret.Namespace != rancherPlanSecretNamespace || secret.Annotations == nil || secret.Type != rancherPlanSecretType {
		return secret, nil
	}

	if secret.Annotations[rke2PreDrainAnnotation] == "" && secret.Annotations[rke2PostDrainAnnotation] == "" {
		return secret, nil
	}

	if secret.Annotations[rke2PreDrainAnnotation] == secret.Annotations[preDrainAnnotation] && secret.Annotations[rke2PostDrainAnnotation] == secret.Annotations[postDrainAnnotation] {
		return secret, nil
	}

	upgradeControllerLock.Lock()
	defer upgradeControllerLock.Unlock()

	upgrade, err := ensureSingleUpgrade(h.namespace, h.upgradeCache)
	if err != nil {
		return nil, err
	}

	if upgrade.Labels[upgradeStateLabel] != StateUpgradingNodes {
		return secret, nil
	}

	machineName, ok := secret.Labels[rancherPlanSecretMachineLabel]
	if !ok {
		return secret, nil
	}

	machine, err := h.machineCache.Get(rancherMachineNamespace, machineName)
	if err != nil {
		return secret, nil
	}

	if machine.Status.NodeRef == nil {
		return secret, nil
	}
	nodeName := machine.Status.NodeRef.Name

	if upgrade.Status.NodeStatuses == nil || upgrade.Status.NodeStatuses[nodeName].State == "" {
		return secret, nil
	}

	upgradeCpy := upgrade.DeepCopy()

	switch upgrade.Status.NodeStatuses[nodeName].State {
	case nodeStateImagesPreloaded:
		if !isUnderPreDrain(secret) {
			break
		}
		if shouldPauseNodeUpgrade(upgrade, nodeName) {
			logrus.Infof("Pause pre-drain job creation for node %s", nodeName)
			setNodeUpgradeStatus(upgradeCpy, nodeName, nodeStateUpgradePaused, "AdministrativelyPaused", "Node upgrade paused as requested by the user")
			logrus.Infof("Update upgrade %s/%s", upgrade.Namespace, upgrade.Name)
			_, err := h.upgradeClient.Update(upgradeCpy)
			return secret, err
		}
		if err := checkEligibleToDrain(upgrade, nodeName); err != nil {
			return nil, err
		}
		logrus.Debugf("Create pre-drain job on %s", nodeName)
		if err := h.createHookJob(upgrade, nodeName, upgradeJobTypePreDrain, nodeStatePreDraining); err != nil {
			return nil, err
		}
	case nodeStateUpgradePaused:
		if !isUnderPreDrain(secret) {
			break
		}
		if shouldPauseNodeUpgrade(upgrade, nodeName) {
			logrus.Debugf("Continue pausing pre-drain job creation for node %s", nodeName)
			return secret, nil
		}
		logrus.Infof("Unpause pre-drain job creation for node %s", nodeName)
		setNodeUpgradeStatus(upgradeCpy, nodeName, nodeStateImagesPreloaded, "", "")
		logrus.Infof("Update upgrade %s/%s", upgrade.Namespace, upgrade.Name)
		if _, err := h.upgradeClient.Update(upgradeCpy); err != nil {
			return secret, err
		}
		h.secretController.EnqueueAfter(secret.Namespace, secret.Name, 5*time.Second)
		return secret, err
	case nodeStatePreDrained:
		if secret.Annotations[rke2PostDrainAnnotation] != secret.Annotations[postDrainAnnotation] {
			if err := checkEligibleToDrain(upgrade, nodeName); err != nil {
				return nil, err
			}
			logrus.Debugf("Create post-drain job on %s", nodeName)
			if err := h.createHookJob(upgrade, nodeName, upgradeJobTypePostDrain, nodeStatePostDraining); err != nil {
				return nil, err
			}
		}
	}

	return secret, nil
}

func (h *secretHandler) createHookJob(upgrade *harvesterv1.Upgrade, nodeName string, jobType string, nextState string) error {
	err := h.checkPendingHookJobs(upgrade.Name)
	if err != nil {
		return err
	}

	repoInfo, err := getCachedRepoInfo(upgrade)
	if err != nil {
		return err
	}

	_, err = h.jobClient.Create(applyNodeJob(upgrade, repoInfo, nodeName, jobType))
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	toUpdate := upgrade.DeepCopy()
	setNodeUpgradeStatus(toUpdate, nodeName, nextState, "", "")
	if _, err := h.upgradeClient.Update(toUpdate); err != nil {
		return err
	}

	return nil
}

func (h *secretHandler) checkPendingHookJobs(upgrade string) error {
	sets := labels.Set{
		harvesterUpgradeLabel: upgrade,
	}
	jobs, err := h.jobCache.List(h.namespace, sets.AsSelector())
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if job.Status.Succeeded == 0 {
			return fmt.Errorf("there are pending jobs: (%s/%s)", job.Namespace, job.Name)
		}
	}
	return nil
}

func checkEligibleToDrain(upgrade *harvesterv1.Upgrade, nodeName string) error {
	// To make sure there will be only one node in the cluster can be put into the pre-drain or post-drain state
	for name, status := range upgrade.Status.NodeStatuses {
		if name == nodeName {
			continue
		}
		if status.State == StateSucceeded || status.State == nodeStateImagesPreloaded {
			continue
		}
		return fmt.Errorf("%s is in \"%s\" state so %s is not allowed to run any kind of job", name, status, nodeName)
	}
	return nil
}

func isUnderPreDrain(secret *v1.Secret) bool {
	return secret.Annotations[rke2PreDrainAnnotation] != secret.Annotations[preDrainAnnotation]
}
