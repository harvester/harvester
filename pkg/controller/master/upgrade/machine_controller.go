package upgrade

import (
	"fmt"

	v1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

const (
	rancherMachineNamespace = "fleet-local"
)

// machineHandler watches pre-drain and pos-drain annotations set by Rancher and create corresponding node jobs
type machineHandler struct {
	namespace     string
	upgradeClient ctlharvesterv1.UpgradeClient
	upgradeCache  ctlharvesterv1.UpgradeCache
	jobClient     v1.JobClient
	jobCache      v1.JobCache
}

func (h *machineHandler) OnChanged(key string, machine *clusterv1.Machine) (*clusterv1.Machine, error) {
	if machine == nil || machine.DeletionTimestamp != nil || machine.Namespace != rancherMachineNamespace || machine.Annotations == nil {
		return machine, nil
	}

	if machine.Annotations[rke2PreDrainAnnotation] == "" && machine.Annotations[rke2PostDrainAnnotation] == "" {
		return machine, nil
	}

	if machine.Annotations[rke2PreDrainAnnotation] == machine.Annotations[preDrainAnnotation] && machine.Annotations[rke2PostDrainAnnotation] == machine.Annotations[postDrainAnnotation] {
		return machine, nil
	}

	upgradeControllerLock.Lock()
	defer upgradeControllerLock.Unlock()

	upgrade, err := ensureSingleUpgrade(h.namespace, h.upgradeCache)
	if err != nil {
		return nil, err
	}

	if upgrade.Labels[upgradeStateLabel] != StateUpgradingNodes {
		return machine, nil
	}

	if machine.Status.NodeRef == nil {
		return machine, nil
	}
	nodeName := machine.Status.NodeRef.Name

	if upgrade.Status.NodeStatuses == nil || upgrade.Status.NodeStatuses[nodeName].State == "" {
		return machine, nil
	}
	if upgrade.Status.NodeStatuses == nil {
		return machine, nil
	}

	switch upgrade.Status.NodeStatuses[nodeName].State {
	case nodeStateImagesPreloaded:
		if machine.Annotations[rke2PreDrainAnnotation] != machine.Annotations[preDrainAnnotation] {
			logrus.Debugf("Create pre-drain job on %s", nodeName)
			if err := h.createHookJob(upgrade, nodeName, upgradeJobTypePreDrain, nodeStatePreDraining); err != nil {
				return nil, err
			}
		}
	case nodeStatePreDrained:
		if machine.Annotations[rke2PostDrainAnnotation] != machine.Annotations[postDrainAnnotation] {
			logrus.Debugf("Create post-drain job on %s", nodeName)
			if err := h.createHookJob(upgrade, nodeName, upgradeJobTypePostDrain, nodeStatePostDraining); err != nil {
				return nil, err
			}
		}
	}

	return machine, nil
}

func (h *machineHandler) createHookJob(upgrade *harvesterv1.Upgrade, nodeName string, jobType string, nextState string) error {
	err := h.checkPendingHookJobs(upgrade.Name)
	if err != nil {
		return err
	}

	repoInfo, err := getCachedRepoInfo(upgrade)
	if err != nil {
		return err
	}

	_, err = h.jobClient.Create(applyNodeJob(upgrade, repoInfo, nodeName, jobType))
	if err != nil {
		return err
	}

	toUpdate := upgrade.DeepCopy()
	setNodeUpgradeStatus(toUpdate, nodeName, nextState, "", "")
	if _, err := h.upgradeClient.Update(toUpdate); err != nil {
		return err
	}

	return nil
}

func (h *machineHandler) checkPendingHookJobs(upgrade string) error {
	sets := labels.Set{
		harvesterUpgradeLabel: upgrade,
	}
	jobs, err := h.jobCache.List(h.namespace, sets.AsSelector())
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if job.Status.Succeeded == 0 {
			return fmt.Errorf("There are pending jobs: (%s/%s)", job.Namespace, job.Name)
		}
	}
	return nil
}
