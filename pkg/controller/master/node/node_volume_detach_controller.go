package node

import (
	"context"
	"fmt"
	"strings"
	"time"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/util"
)

const (
	nodeVolumeDetachControllerName  = "node-volume-detach-controller"
	nodeVolumeDetachEnqueueInterval = 5 * time.Second
)

// nodeVolumeDetachController is responsible for detaching volumes from a node when the node is in draining
// fix https://github.com/harvester/harvester/issues/3681
type nodeVolumeDetachController struct {
	nodeController ctlcorev1.NodeController
	podCache       ctlcorev1.PodCache
	pvcCache       ctlcorev1.PersistentVolumeClaimCache
	volumes        ctllhv1.VolumeClient
	volumeCache    ctllhv1.VolumeCache
	snapshotCache  ctlsnapshotv1.VolumeSnapshotCache
	upgradeCache   ctlharvesterv1.UpgradeCache
}

// VolumeDetachRegister registers the node volume detach controller
func VolumeDetachRegister(ctx context.Context, management *config.Management, _ config.Options) error {
	nodes := management.CoreFactory.Core().V1().Node()
	pods := management.CoreFactory.Core().V1().Pod()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	volumes := management.LonghornFactory.Longhorn().V1beta2().Volume()
	snapshots := management.SnapshotFactory.Snapshot().V1().VolumeSnapshot()
	upgrades := management.HarvesterFactory.Harvesterhci().V1beta1().Upgrade()
	nodeVolumeDetachController := &nodeVolumeDetachController{
		nodeController: nodes,
		podCache:       pods.Cache(),
		pvcCache:       pvcs.Cache(),
		volumes:        volumes,
		volumeCache:    volumes.Cache(),
		snapshotCache:  snapshots.Cache(),
		upgradeCache:   upgrades.Cache(),
	}

	nodes.OnChange(ctx, nodeVolumeDetachControllerName, nodeVolumeDetachController.OnNodeChanged)

	return nil
}

// OnNodeChanged is called when a node is changed
func (c *nodeVolumeDetachController) OnNodeChanged(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}

	ok, err := c.isInUpgradePreDraining(node)
	if !ok || err != nil {
		return node, err
	}
	logrus.Infof("node %s is in upgrade pre-draining state, try to detach unused volumes", node.Name)

	volumes, err := c.getNodePVCVolumes(node)
	if err != nil {
		return node, err
	}
	waitVolumeNames := make([]string, 0, len(volumes))
	for _, volume := range volumes {
		switch volume.Status.State {
		case lhv1beta2.VolumeStateCreating, lhv1beta2.VolumeStateAttaching, lhv1beta2.VolumeStateDetaching:
			waitVolumeNames = append(waitVolumeNames, volume.Name)
		case lhv1beta2.VolumeStateAttached:
			pvc, err := c.getPVCByVolume(volume)
			if err != nil {
				return node, err
			}
			logrus.Debugf("check whether the volume %s of pvc %s/%s can be detached", volume.Name, pvc.Namespace, pvc.Name)
			wait, err := c.checkDetachVolume(pvc)
			if err != nil {
				return node, err
			}
			if wait {
				waitVolumeNames = append(waitVolumeNames, volume.Name)
			}
		}
	}

	if len(waitVolumeNames) > 0 {
		logrus.Infof("requeue the node %s while waiting for these volumes can be detached: %s", node.Name, strings.Join(waitVolumeNames, ", "))
		c.nodeController.EnqueueAfter(node.Name, nodeVolumeDetachEnqueueInterval)
	}

	return node, nil
}

// isInUpgradePreDraining checks whether the node is in the pre-draining state of an upgrade
func (c *nodeVolumeDetachController) isInUpgradePreDraining(node *corev1.Node) (bool, error) {
	if !isNodeInDraining(node) {
		return false, nil
	}

	upgrades, err := c.upgradeCache.List(util.HarvesterSystemNamespaceName, labels.NewSelector())
	if err != nil {
		return false, err
	}
	for _, upgrade := range upgrades {
		if !harvesterv1.UpgradeCompleted.IsTrue(upgrade) && !harvesterv1.UpgradeCompleted.IsFalse(upgrade) {
			return true, nil
		}
	}
	return false, nil
}

// getNodePVCVolumes gets the volumes created by PVCs on the node
func (c *nodeVolumeDetachController) getNodePVCVolumes(node *corev1.Node) ([]*lhv1beta2.Volume, error) {
	volumes, err := c.volumeCache.GetByIndex(indexeres.VolumeByNodeIndex, node.Name)
	if err != nil {
		return nil, err
	}

	pvcVolumes := make([]*lhv1beta2.Volume, 0, len(volumes))

	for _, volume := range volumes {
		if volume.Status.KubernetesStatus.PVCName == "" {
			logrus.Debugf("skip the volume %s since it is not created by a pvc", volume.Name)
			continue
		}
		pvcVolumes = append(pvcVolumes, volume)
	}

	return pvcVolumes, nil
}

// getPVCByVolume gets the PVC of the volume
func (c *nodeVolumeDetachController) getPVCByVolume(volume *lhv1beta2.Volume) (*corev1.PersistentVolumeClaim, error) {
	return c.pvcCache.Get(volume.Status.KubernetesStatus.Namespace, volume.Status.KubernetesStatus.PVCName)
}

// checkDetachVolume checks whether a volume can be detached from a node
func (c *nodeVolumeDetachController) checkDetachVolume(pvc *corev1.PersistentVolumeClaim) (wait bool, err error) {
	// 1. check whether pv is bound
	if pvc.Status.Phase == corev1.ClaimPending {
		return true, nil
	}

	// 2. check whether any pod uses the PVC
	pods, err := c.podCache.GetByIndex(indexeres.PodByPVCIndex, fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name))
	if err != nil {
		return false, fmt.Errorf("can't get pods by index %s with pvc %s/%s, err: %w", indexeres.PodByPVCIndex, pvc.Namespace, pvc.Name, err)
	}
	for _, pod := range pods {
		switch pod.Status.Phase {
		case corev1.PodRunning, corev1.PodPending:
			logrus.Debugf("pod %s/%s is %s, don't detach pvc: %s/%s", pod.Namespace, pod.Name, pod.Status.Phase, pvc.Namespace, pvc.Name)
			return true, nil
		}
	}

	// 3. check whether any in progress volume snapshot uses the PVC
	volumeSnapshots, err := c.snapshotCache.GetByIndex(indexeres.VolumeSnapshotBySourcePVCIndex, fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name))
	if err != nil {
		return false, fmt.Errorf("can't get volume snapshots by index %s with pvc %s/%s, err: %w", indexeres.VolumeSnapshotBySourcePVCIndex, pvc.Namespace, pvc.Name, err)
	}
	for _, volumeSnapshot := range volumeSnapshots {
		if volumeSnapshot.Status == nil || volumeSnapshot.Status.ReadyToUse == nil || !*volumeSnapshot.Status.ReadyToUse {
			logrus.Debugf("volumeSnapshot %s/%s is in processing, don't detach pvc: %s/%s", volumeSnapshot.Namespace, volumeSnapshot.Name, pvc.Namespace, pvc.Name)
			return true, nil
		}

		// 4. check whether any pending pvc's source pvc is it
		childPVCs, err := c.pvcCache.GetByIndex(indexeres.PVCByDataSourceVolumeSnapshotIndex, fmt.Sprintf("%s/%s", volumeSnapshot.Namespace, volumeSnapshot.Name))
		if err != nil {
			return false, fmt.Errorf("can't get pvcs by index %s with volume snapshot %s/%s, err: %w", indexeres.PVCByDataSourceVolumeSnapshotIndex, volumeSnapshot.Namespace, volumeSnapshot.Name, err)
		}
		for _, childPVC := range childPVCs {
			if childPVC.Status.Phase == corev1.ClaimPending {
				logrus.Debugf("child pvc %s/%s is not bound, don't detach pvc: %s/%s", childPVC.Namespace, childPVC.Name, pvc.Namespace, pvc.Name)
				return true, nil
			}
		}
	}

	return false, nil
}

// isNodeInDraining checks whether a node is in draining
// triggered by command: kubectl taint node $HARVESTER_UPGRADE_NODE_NAME --overwrite kubevirt.io/drain=draining:NoSchedule
// in package/upgrade/upgrade_node.sh
func isNodeInDraining(node *corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == util.UpgradeNodeDrainTaintKey && taint.Value == util.UpgradeNodeDrainTaintValue && taint.Effect == corev1.TaintEffectNoSchedule {
			return true
		}
	}

	return false
}
