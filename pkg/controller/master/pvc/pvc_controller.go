package pvc

import (
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"

	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
)

// pvcHandler used to separate the PVC from the DataVolume
type pvcHandler struct {
	dataVolumeClient ctlcdiv1.DataVolumeClient
	pvcClient        ctlcorev1.PersistentVolumeClaimClient
	pvcController    ctlcorev1.PersistentVolumeClaimController
}

func (h *pvcHandler) cleanupDataVolume(_ string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc == nil {
		return pvc, nil
	}

	if _, find := pvc.Annotations[cdicommon.AnnCreatedForDataVolume]; !find {
		return pvc, nil
	}

	// we need to remove dataVolume (if we have) before we remove the PVC
	if err := h.dataVolumeClient.Delete(pvc.Namespace, pvc.Name, &metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return pvc, err
		}
	}

	logrus.Infof("Deleted DataVolume %s/%s with PVC removing", pvc.Namespace, pvc.Name)
	return pvc, nil
}
