package volume

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/server/subresource"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

var (
	subResourceMethod = map[string]string{
		actionExport:       http.MethodPut,
		actionCancelExpand: http.MethodPut,
		actionClone:        http.MethodPut,
		actionSnapshot:     http.MethodPut,
	}
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
}

func (h *ActionHandler) IsMatchedResource(resource subresource.Resource, httpMethod string) bool {
	if resource.Name != subresource.PersistentVolumeClaims.Resource {
		return false
	}

	if method, ok := subResourceMethod[resource.SubResource]; ok {
		return method == httpMethod
	}

	return false
}

func (h *ActionHandler) SubResourceHandler(_ http.ResponseWriter, r *http.Request, resource subresource.Resource) error {
	action := resource.SubResource
	pvcNamespace := resource.Namespace
	pvcName := resource.ObjectName

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
		return apierror.NewAPIError(validation.InvalidAction, fmt.Sprintf("Unsupported subresource %s", resource.SubResource))
	}
}

func (h *ActionHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
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

func (h *ActionHandler) do(rw http.ResponseWriter, r *http.Request) error {
	vars := util.EncodeVars(mux.Vars(r))

	resource := subresource.Resource{
		Name:        subresource.PersistentVolumeClaims.Resource,
		ObjectName:  vars["name"],
		Namespace:   vars["namespace"],
		SubResource: vars["action"],
	}

	return subresource.Execute(h, rw, r, resource)
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

	// make sure the volume is not attached to any VMs
	// otherwise, the harvester webhook will reject the pvc deletion below.
	annotationSchemaOwners, err := ref.GetSchemaOwnersFromAnnotation(pvc)
	if err != nil {
		return fmt.Errorf("failed to get schema owners from annotation: %v", err)
	}

	if attachedList := annotationSchemaOwners.List(kubevirtv1.Kind(kubevirtv1.VirtualMachineGroupVersionKind.Kind)); len(attachedList) != 0 {
		return fmt.Errorf("can not operate the volume %s which is currently attached to VMs: %s", pvc.Name, strings.Join(attachedList, ", "))
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
