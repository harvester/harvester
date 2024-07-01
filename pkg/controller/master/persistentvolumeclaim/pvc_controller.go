package persistentvolumeclaim

import (
	"context"
	"reflect"
	"time"

	longhorntypes "github.com/longhorn/longhorn-manager/types"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/config"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/util"
)

const (
	pvcControllerManageSnapshotMaxCountAndSize = "PVCController.ManageSnapshotMaxCountAndSize"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	var (
		pvcs    = management.CoreFactory.Core().V1().PersistentVolumeClaim()
		volumes = management.LonghornFactory.Longhorn().V1beta2().Volume()
	)
	pvcController := PVCController{
		pvcs:    pvcs,
		volumes: volumes,
	}
	pvcs.OnChange(ctx, pvcControllerManageSnapshotMaxCountAndSize, pvcController.manageSnapshotMaxCountAndSize)
	return nil
}

type PVCController struct {
	pvcs    ctlcorev1.PersistentVolumeClaimController
	volumes ctllhv1.VolumeClient
}

func (h *PVCController) manageSnapshotMaxCountAndSize(_ string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc == nil || pvc.DeletionTimestamp != nil {
		return pvc, nil
	}

	// If the PVC is not ready, requeue it
	if pvc.Spec.VolumeName == "" {
		h.pvcs.EnqueueAfter(pvc.Namespace, pvc.Name, 5*time.Second)
		return pvc, nil
	}

	if util.GetProvisionedPVCProvisioner(pvc) != longhorntypes.LonghornDriverName {
		return pvc, nil
	}

	volume, err := h.volumes.Get(util.LonghornSystemNamespaceName, pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	snapshotMaxCount, snapshotMaxSize, err := util.GetSnapshotMaxCountAndSize(pvc)
	if err != nil {
		return nil, err
	}

	volumeCopy := volume.DeepCopy()
	volumeCopy.Spec.SnapshotMaxCount = snapshotMaxCount
	volumeCopy.Spec.SnapshotMaxSize = snapshotMaxSize

	if !reflect.DeepEqual(volume.Spec, volumeCopy.Spec) {
		_, err = h.volumes.Update(volumeCopy)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"namespace":        pvc.Namespace,
				"name":             pvc.Name,
				"volume":           pvc.Spec.VolumeName,
				"snapshotMaxCount": snapshotMaxCount,
				"snapshotMaxSize":  snapshotMaxSize,
			}).Error("failed to update volume for snapshot max count and size")
		}
	}
	return pvc, nil
}
