package volume

import (
	"fmt"
	"time"

	lhv1beta1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1beta1"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/util"
)

const (
	detachVolumeEnqueueInterval = 5 * time.Second
)

type Controller struct {
	pvcCache         v1.PersistentVolumeClaimCache
	volumes          ctllonghornv1.VolumeClient
	volumeController ctllonghornv1.VolumeController
	volumeCache      ctllonghornv1.VolumeCache
	snapshotCache    ctlsnapshotv1.VolumeSnapshotCache
	vmCache          ctlkubevirtv1.VirtualMachineCache
}

// Detach unused volumes, so attached volumes don't block node drain.
func (c *Controller) DetachVolumesOnChange(_ string, volume *lhv1beta1.Volume) (*lhv1beta1.Volume, error) {
	if volume == nil || volume.DeletionTimestamp != nil || volume.Status.KubernetesStatus.PVCName == "" {
		return volume, nil
	}

	if isVolumeDetached(volume) {
		return volume, nil
	}

	pvc, err := c.pvcCache.Get(volume.Status.KubernetesStatus.Namespace, volume.Status.KubernetesStatus.PVCName)
	if err != nil {
		return volume, fmt.Errorf("can't find pvc %s/%s, err: %w", volume.Status.KubernetesStatus.Namespace, volume.Status.KubernetesStatus.PVCName, err)
	}
	if pvc.Status.Phase == corev1.ClaimPending {
		c.volumeController.EnqueueAfter(volume.Namespace, volume.Name, detachVolumeEnqueueInterval)
		return volume, nil
	}

	canDetach, watchAgain, err := c.checkDetachVolume(pvc)
	if err != nil {
		return volume, err
	}
	logrus.Debugf("pvc: %s/%s, canDetach: %t, watchAgain: %t", pvc.Namespace, pvc.Name, canDetach, watchAgain)
	if canDetach {
		if err := c.detachVolume(pvc); err != nil {
			return volume, err
		}
	}
	if watchAgain {
		c.volumeController.EnqueueAfter(volume.Namespace, volume.Name, detachVolumeEnqueueInterval)
	}

	return volume, nil
}

func (c *Controller) checkDetachVolume(pvc *corev1.PersistentVolumeClaim) (canDetach, watchAgain bool, err error) {
	// 1. check whether any pod uses the PVC
	volume, err := c.volumeCache.Get(util.LonghornSystemNamespaceName, pvc.Spec.VolumeName)
	if err != nil {
		return false, false, fmt.Errorf("can't find volume %s/%s, err: %w", util.LonghornSystemNamespaceName, pvc.Spec.VolumeName, err)
	}
	for _, workload := range volume.Status.KubernetesStatus.WorkloadsStatus {
		// For running workload, we don't want to watch again
		if workload.PodStatus == string(corev1.PodRunning) || workload.PodStatus == string(corev1.PodPending) {
			logrus.Debugf("workload %s/%s is %s, don't detach pvc: %s/%s", volume.Status.KubernetesStatus.Namespace, workload.WorkloadName, workload.PodStatus, pvc.Namespace, pvc.Name)
			return false, false, nil
		}
		if workload.WorkloadType == kubevirtv1.VirtualMachineInstanceGroupVersionKind.Kind {
			var done bool
			canDetach, watchAgain, done, err = c.checkVMStatus(volume, workload)
			if done {
				return canDetach, watchAgain, err
			}
		}
	}

	// 2. check whether any in progress volume snapshot uses the PVC
	volumeSnapshots, err := c.snapshotCache.GetByIndex(indexeres.VolumeSnapshotBySourcePVCIndex, fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name))
	if err != nil {
		return false, false, fmt.Errorf("can't get volume snapshots by index %s with pvc %s/%s, err: %w", indexeres.VolumeSnapshotBySourcePVCIndex, pvc.Namespace, pvc.Name, err)
	}
	for _, volumeSnapshot := range volumeSnapshots {
		if volumeSnapshot.Status == nil || volumeSnapshot.Status.ReadyToUse == nil || !*volumeSnapshot.Status.ReadyToUse {
			logrus.Debugf("volumeSnapshot %s/%s is in processing, don't detach pvc: %s/%s", volumeSnapshot.Namespace, volumeSnapshot.Name, pvc.Namespace, pvc.Name)
			return false, true, nil
		}

		// 3. check whether any pending pvc's source pvc is it
		childPVCs, err := c.pvcCache.GetByIndex(indexeres.PVCByDataSourceVolumeSnapshotIndex, fmt.Sprintf("%s/%s", volumeSnapshot.Namespace, volumeSnapshot.Name))
		if err != nil {
			return false, false, fmt.Errorf("can't get pvcs by index %s with volume snapshot %s/%s, err: %w", indexeres.PVCByDataSourceVolumeSnapshotIndex, volumeSnapshot.Namespace, volumeSnapshot.Name, err)
		}
		for _, childPVC := range childPVCs {
			if childPVC.Status.Phase == corev1.ClaimPending {
				logrus.Debugf("pvc %s/%s is not bound, don't detach pvc: %s/%s", childPVC.Namespace, childPVC.Name, pvc.Namespace, pvc.Name)
				return false, true, nil
			}
		}
	}
	return true, false, nil
}

func (c *Controller) checkVMStatus(volume *lhv1beta1.Volume, workload lhv1beta1.WorkloadStatus) (canDetach, watchAgain, done bool, err error) {
	vmiNamespace := volume.Status.KubernetesStatus.Namespace
	vmiName := workload.WorkloadName
	vm, err := c.vmCache.Get(vmiNamespace, vmiName)
	if err != nil {
		return false, false, true, err
	}
	// if the VM's runStrategy is not Halted, vmi and pod will be recreated, we can't detach the volume
	runStrategy, err := vm.RunStrategy()
	if err != nil {
		return false, false, true, err
	}
	if runStrategy != kubevirtv1.RunStrategyHalted {
		return false, false, true, nil
	}
	// if the VM's runStrategy is Halted, we also need to retry util the VM's status become to Stopped
	if vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusStopped {
		return false, true, true, nil
	}
	return false, false, false, nil
}

func (c *Controller) detachVolume(pvc *corev1.PersistentVolumeClaim) error {
	volume, err := c.volumeCache.Get(util.LonghornSystemNamespaceName, pvc.Spec.VolumeName)
	if err != nil {
		return fmt.Errorf("can't find volume %s/%s, err: %w", util.LonghornSystemNamespaceName, pvc.Spec.VolumeName, err)
	}

	if volume.Status.State == lhv1beta1.VolumeStateAttached || volume.Status.State == lhv1beta1.VolumeStateAttaching {
		volCpy := volume.DeepCopy()
		volCpy.Spec.NodeID = ""
		logrus.Infof("detach volume %s", volCpy.Name)
		if _, err = c.volumes.Update(volCpy); err != nil {
			return err
		}
	}
	return nil
}
