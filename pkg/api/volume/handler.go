package volume

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
)

type ActionHandler struct {
	images      v1beta1.VirtualMachineImageClient
	pvcs        ctlcorev1.PersistentVolumeClaimClient
	pvcCache    ctlcorev1.PersistentVolumeClaimCache
	pvs         ctlharvcorev1.PersistentVolumeClient
	pvCache     ctlharvcorev1.PersistentVolumeCache
	snapshots   ctlsnapshotv1.VolumeSnapshotClient
	volumes     ctllhv1.VolumeClient
	volumeCache ctllhv1.VolumeCache
	vmCache     ctlkubevirtv1.VirtualMachineCache
}

func (h ActionHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if err := h.do(rw, req); err != nil {
		status := http.StatusInternalServerError
		if e, ok := err.(*apierror.APIError); ok {
			status = e.Code.Status
		}
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

func (h *ActionHandler) do(_ http.ResponseWriter, r *http.Request) error {
	vars := util.EncodeVars(mux.Vars(r))
	action := vars["action"]
	pvcName := vars["name"]
	pvcNamespace := vars["namespace"]

	switch action {
	case actionExport:
		var input ExportVolumeInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.DisplayName == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `displayName` is required")
		}
		if input.Namespace == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `namespace` is required")
		}
		return h.exportVolume(r.Context(), input.Namespace, input.DisplayName, input.StorageClassName, pvcNamespace, pvcName)
	case actionCancelExpand:
		return h.cancelExpand(r.Context(), pvcNamespace, pvcName)
	case actionClone:
		var input CloneVolumeInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.Name == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `name` is required")
		}
		return h.clone(r.Context(), pvcNamespace, pvcName, input.Name)
	case actionSnapshot:
		var input SnapshotVolumeInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.Name == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `name` is required")
		}
		return h.snapshot(r.Context(), pvcNamespace, pvcName, input.Name)
	default:
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
}

func (h *ActionHandler) exportVolume(_ context.Context, imageNamespace, imageDisplayName, imageStorageClassName, pvcNamespace, pvcName string) error {
	vmImage := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "image-",
			Namespace:    imageNamespace,
			Annotations:  map[string]string{},
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			DisplayName:  imageDisplayName,
			SourceType:   harvesterv1.VirtualMachineImageSourceTypeExportVolume,
			PVCName:      pvcName,
			PVCNamespace: pvcNamespace,
		},
	}

	if imageStorageClassName != "" {
		vmImage.Annotations[util.AnnotationStorageClassName] = imageStorageClassName
	}

	if _, err := h.images.Create(vmImage); err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace":  pvcNamespace,
			"name":       pvcName,
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"err":        err,
		}).Error("failed to create image from PVC")
		return err
	}

	return nil
}

func (h *ActionHandler) cancelExpand(_ context.Context, pvcNamespace, pvcName string) error {
	// get pvc
	pvc, err := h.pvcCache.Get(pvcNamespace, pvcName)
	if err != nil {
		return err
	}

	vms, err := h.vmCache.GetByIndex(indexeresutil.VMByPVCIndex, ref.Construct(pvcNamespace, pvcName))
	if err != nil {
		return fmt.Errorf("failed to get VMs by index: %s, PVC: %s/%s, err: %v", indexeresutil.VMByPVCIndex, pvcNamespace, pvcName, err)
	}
	if len(vms) != 0 {
		return fmt.Errorf("can not operate the volume %s which is currently attached to VM: %s/%s", pvc.Name, vms[0].Namespace, vms[0].Name)
	}

	// get pv
	pvName := pvc.Spec.VolumeName
	pv, err := h.pvCache.Get(pvName)
	if err != nil {
		return err
	}

	// backup pv reclaim policy
	pvReclaimPolicyBackup := pv.Spec.PersistentVolumeReclaimPolicy

	// change pv reclaim policy to Retain
	if err = h.tryUpdatePV(pvName, func(pv *corev1.PersistentVolume) *corev1.PersistentVolume {
		pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimRetain
		return pv
	}); err != nil {
		logrus.WithFields(logrus.Fields{
			"name":       pvName,
			"apiVersion": "v1",
			"kind":       "PersistentVolume",
			"policy":     corev1.PersistentVolumeReclaimRetain,
			"err":        err,
		}).Error("failed to change reclaim policy of PV")
		return err
	}

	// delete pvc
	if err = h.pvcs.Delete(pvcNamespace, pvcName, &metav1.DeleteOptions{}); err != nil {
		logrus.WithFields(logrus.Fields{
			"name":       pvcName,
			"namespace":  pvcNamespace,
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"err":        err,
		}).Error("failed to delete PVC")
		return err
	}
	if err = h.waitPVCDeleted(pvcNamespace, pvcName); err != nil {
		return err
	}

	// remove claimRef from pv
	if err = h.tryUpdatePV(pvName, func(pv *corev1.PersistentVolume) *corev1.PersistentVolume {
		pv.Spec.ClaimRef = nil
		return pv
	}); err != nil {
		logrus.WithFields(logrus.Fields{
			"name":       pvName,
			"apiVersion": "v1",
			"kind":       "PersistentVolume",
			"err":        err,
		}).Error("failed to remove claimRef from PV")
		return err
	}

	// restore pvc
	restorePVC := pvc.DeepCopy()
	restorePVC.ResourceVersion = ""
	restorePVC.UID = ""
	restorePVC.Spec.Resources.Requests[corev1.ResourceStorage] = *pvc.Status.Capacity.Storage()
	if _, err = h.pvcs.Create(restorePVC); err != nil {
		logrus.WithFields(logrus.Fields{
			"name":       pvcName,
			"namespace":  pvcNamespace,
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"err":        err,
		}).Error("failed to restore PVC")
		return err
	}

	// restore pv reclaim policy
	if err = h.tryUpdatePV(pvName, func(pv *corev1.PersistentVolume) *corev1.PersistentVolume {
		pv.Spec.PersistentVolumeReclaimPolicy = pvReclaimPolicyBackup
		return pv
	}); err != nil {
		logrus.WithFields(logrus.Fields{
			"name":       pvName,
			"apiVersion": "v1",
			"kind":       "PersistentVolume",
			"policy":     pvReclaimPolicyBackup,
			"err":        err,
		}).Error("failed to restore reclaim policy of PV")
		return err
	}

	return nil
}

func (h *ActionHandler) waitPVCDeleted(pvcNamespace, pvcName string) error {
	backoff := wait.Backoff{
		Steps:    30,
		Duration: 1 * time.Second,
		Factor:   1.0,
		Jitter:   0.1,
	}
	return retry.OnError(backoff, util.IsStillExists, func() error {
		if _, err := h.pvcs.Get(pvcNamespace, pvcName, metav1.GetOptions{}); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}
		return util.NewStillExists(corev1.Resource("persistentvolumeclaim"), pvcName)
	})
}

func (h *ActionHandler) tryUpdatePV(pvName string, update func(pv *corev1.PersistentVolume) *corev1.PersistentVolume) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pv, err := h.pvs.Get(pvName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		newPV := update(pv.DeepCopy())
		_, err = h.pvs.Update(newPV)
		return err
	})
}

func (h *ActionHandler) clone(_ context.Context, pvcNamespace, pvcName, newPVCName string) error {
	pvc, err := h.pvcCache.Get(pvcNamespace, pvcName)
	if err != nil {
		return err
	}

	newPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        newPVCName,
			Namespace:   pvcNamespace,
			Annotations: map[string]string{},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      pvc.Spec.AccessModes,
			Resources:        pvc.Spec.Resources,
			StorageClassName: pvc.Spec.StorageClassName,
			VolumeMode:       pvc.Spec.VolumeMode,
			DataSource: &corev1.TypedLocalObjectReference{
				Kind: "PersistentVolumeClaim",
				Name: pvcName,
			},
		},
	}

	if imageID := pvc.Annotations[util.AnnotationImageID]; imageID != "" {
		newPVC.Annotations[util.AnnotationImageID] = imageID
	}

	if _, err = h.pvcs.Create(newPVC); err != nil {
		logrus.WithFields(logrus.Fields{
			"name":       pvcName,
			"namespace":  pvcNamespace,
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"err":        err,
		}).Error("failed to clone volume")
		return err
	}

	return nil
}

func (h *ActionHandler) snapshot(_ context.Context, pvcNamespace, pvcName, snapshotName string) error {
	pvc, err := h.pvcCache.Get(pvcNamespace, pvcName)
	if err != nil {
		return err
	}

	provisioner := util.GetProvisionedPVCProvisioner(pvc)
	csiDriverInfo, err := settings.GetCSIDriverInfo(provisioner)
	if err != nil {
		return err
	}

	volumeSnapshotClassName := csiDriverInfo.VolumeSnapshotClassName
	pvcAPIVersion, pvcKind := util.PersistentVolumeClaimsKind.ToAPIVersionAndKind()
	snapshot := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotName,
			Namespace: pvcNamespace,
			Annotations: map[string]string{
				util.AnnotationStorageClassName:   *pvc.Spec.StorageClassName,
				util.AnnotationStorageProvisioner: provisioner,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: pvcAPIVersion,
					Kind:       pvcKind,
					Name:       pvc.Name,
					UID:        pvc.UID,
				},
			},
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: &pvcName,
			},
			VolumeSnapshotClassName: &volumeSnapshotClassName,
		},
	}

	if imageID := pvc.Annotations[util.AnnotationImageID]; imageID != "" {
		snapshot.Annotations[util.AnnotationImageID] = imageID
	}

	if _, err = h.snapshots.Create(snapshot); err != nil {
		logrus.WithFields(logrus.Fields{
			"name":       pvcName,
			"namespace":  pvcNamespace,
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"err":        err,
		}).Error("failed to create volume snapshot from PVC")
		return err
	}

	return nil
}
