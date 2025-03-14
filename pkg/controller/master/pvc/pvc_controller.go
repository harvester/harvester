package pvc

import (
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"

	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

// pvcHandler used to separate the PVC from the DataVolume
type pvcHandler struct {
	volImportSourceClient ctlcdiv1.VolumeImportSourceClient
	dataVolumeClient      ctlcdiv1.DataVolumeClient
	pvcClient             ctlcorev1.PersistentVolumeClaimClient
	pvcController         ctlcorev1.PersistentVolumeClaimController
}

func (h *pvcHandler) createFilesystemBlankSource(_ string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc == nil || pvc.Spec.VolumeMode == nil || *pvc.Spec.VolumeMode != corev1.PersistentVolumeFilesystem {
		return pvc, nil
	}

	annotations := pvc.GetAnnotations()
	if v, find := annotations[util.AnnotationVolForVM]; !find || v != "true" {
		logrus.Debugf("PVC %s/%s is not for VM, skip checking filesystem blank", pvc.Namespace, pvc.Name)
		return pvc, nil
	}

	if _, err := h.volImportSourceClient.Get(pvc.Namespace, util.ImportSourceFSBlank, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			blankSource := cdiv1.VolumeImportSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.ImportSourceFSBlank,
					Namespace: pvc.Namespace,
				},
				Spec: cdiv1.VolumeImportSourceSpec{
					Source: &cdiv1.ImportSourceType{
						Blank: &cdiv1.DataVolumeBlankImage{},
					},
				},
			}
			if _, err := h.volImportSourceClient.Create(&blankSource); err != nil {
				return pvc, err
			}
			logrus.Infof("Created filesystem blank source %s/%s for PVC %s/%s", pvc.Namespace, util.ImportSourceFSBlank, pvc.Namespace, pvc.Name)
		} else {
			return pvc, err
		}
	}
	return pvc, nil
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
