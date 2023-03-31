package volume

import (
	"fmt"
	"time"

	lhv1beta1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	attachVolumeEnqueueInterval = 1 * time.Second
)

type PodController struct {
	podController v1.PodController
	pvcCache      v1.PersistentVolumeClaimCache
	volumes       ctllonghornv1.VolumeClient
	volumeCache   ctllonghornv1.VolumeCache
}

// Detach unused volumes, so attached volumes don't block node drain.
func (c *PodController) AttachVolumesOnChange(_ string, pod *corev1.Pod) (*corev1.Pod, error) {
	if pod == nil || pod.DeletionTimestamp != nil {
		return pod, nil
	}

	// only attach volumes for pending pod
	if pod.Status.Phase != corev1.PodPending {
		return pod, nil
	}

	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}

		pvc, err := c.pvcCache.Get(pod.Namespace, volume.PersistentVolumeClaim.ClaimName)
		if err != nil {
			return pod, fmt.Errorf("can't find pvc %s/%s, err: %w", pod.Namespace, volume.PersistentVolumeClaim.ClaimName, err)
		}
		if util.GetProvisionedPVCProvisioner(pvc) != longhorntypes.LonghornDriverName {
			continue
		}
		if pvc.Spec.VolumeName == "" {
			c.podController.EnqueueAfter(pod.Namespace, pod.Name, attachVolumeEnqueueInterval)
			break
		}

		volume, err := c.volumeCache.Get(util.LonghornSystemNamespaceName, pvc.Spec.VolumeName)
		if err != nil {
			return pod, fmt.Errorf("can't find volume %s, err: %w", pvc.Spec.VolumeName, err)
		}
		if isVolumeAttached(volume) {
			continue
		}
		if volume.Status.State == lhv1beta1.VolumeStateDetaching {
			c.podController.EnqueueAfter(pod.Namespace, pod.Name, attachVolumeEnqueueInterval)
			break
		}
		if volume.Status.State == lhv1beta1.VolumeStateDetached && volume.Spec.NodeID != volume.Status.OwnerID {
			logrus.Infof("Attach volume %s to node %s", volume.Name, volume.Status.OwnerID)
			volCpy := volume.DeepCopy()
			volCpy.Spec.NodeID = volCpy.Status.OwnerID
			if _, err = c.volumes.Update(volCpy); err != nil {
				return pod, fmt.Errorf("can't update volume %s, err: %w", pvc.Spec.VolumeName, err)
			}
		}
	}

	return pod, nil
}
