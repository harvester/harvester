package volume

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/mux"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
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
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
)

type ActionHandler struct {
	images      v1beta1.VirtualMachineImageClient
	pods        ctlcorev1.PodCache
	pvcs        ctlcorev1.PersistentVolumeClaimClient
	pvcCache    ctlcorev1.PersistentVolumeClaimCache
	pvs         ctlharvcorev1.PersistentVolumeClient
	pvCache     ctlharvcorev1.PersistentVolumeCache
	snapshots   ctlsnapshotv1.VolumeSnapshotClient
	scCache     ctlstoragev1.StorageClassCache
	volumes     ctllhv1.VolumeClient
	volumeCache ctllhv1.VolumeCache
	vmCache     ctlkubevirtv1.VirtualMachineCache
}

func (h *ActionHandler) Do(ctx *harvesterServer.Ctx) (interface{}, error) {
	req := ctx.Req()
	vars := util.EncodeVars(mux.Vars(req))
	action := vars["action"]
	pvcName := vars["name"]
	pvcNamespace := vars["namespace"]

	switch action {
	case actionExport:
		var input ExportVolumeInput
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			return nil, apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.DisplayName == "" {
			return nil, apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `displayName` is required")
		}
		if input.Namespace == "" {
			return nil, apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `namespace` is required")
		}
		if err := h.validateExportVolume(input.StorageClassName, pvcNamespace, pvcName); err != nil {
			return nil, apierror.NewAPIError(validation.InvalidBodyContent, err.Error())
		}
		return h.exportVolume(req.Context(), input.Namespace, input.DisplayName, input.StorageClassName, pvcNamespace, pvcName)
	case actionCancelExpand:
		return nil, h.cancelExpand(req.Context(), pvcNamespace, pvcName)
	case actionClone:
		var input CloneVolumeInput
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			return nil, apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.Name == "" {
			return nil, apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `name` is required")
		}
		return nil, h.clone(req.Context(), pvcNamespace, pvcName, input.Name)
	case actionSnapshot:
		var input SnapshotVolumeInput
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			return nil, apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.Name == "" {
			return nil, apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `name` is required")
		}
		return nil, h.snapshot(req.Context(), pvcNamespace, pvcName, input.Name)
	default:
		return nil, apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
}

func (h *ActionHandler) assertPVCNotInUse(pvcNamespace, pvcName string) error {
	// find any pod use this PVC (same validation on CDI)
	index := fmt.Sprintf("%s-%s", pvcNamespace, pvcName)
	if pods, err := h.pods.GetByIndex(util.IndexPodByPVC, index); err == nil && len(pods) > 0 {
		podList := []string{}
		for _, pod := range pods {
			indexedPod := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
			podList = append(podList, indexedPod)
		}
		return fmt.Errorf("PVC %s is used by Pods %v, cannot export volume when it's running", pvcName, podList)
	}
	return nil
}

func (h *ActionHandler) validateExportVolume(storageClassName, pvcNamespace, pvcName string) error {

	// check target info
	if storageClassName == "" {
		return fmt.Errorf("parameter `storageClassName` is required")
	}
	sc, err := h.scCache.Get(storageClassName)
	if err != nil {
		return fmt.Errorf("get target StorageClass with Err: %v", err)
	}

	// check source info
	pvc, err := h.pvcCache.Get(pvcNamespace, pvcName)
	if err != nil {
		return fmt.Errorf("get PVC with Err: %v", err)
	}
	pvcStorageClassName := pvc.Spec.StorageClassName
	if pvcStorageClassName == nil {
		return fmt.Errorf("PVC should have storageClassName")
	}
	pvcSC, err := h.scCache.Get(*pvcStorageClassName)
	if err != nil {
		return fmt.Errorf("get PVC StorageClass with Err: %v", err)
	}

	// both source/target SC are Longhorn v1, skip the validation
	if h.isLonghornV1Engine(pvcSC) && h.isLonghornV1Engine(sc) {
		return nil
	}

	// we need to ensure the source PVC is not in use since we need to create source pod to export the volume
	if err := h.assertPVCNotInUse(pvcNamespace, pvcName); err != nil {
		return err
	}

	// export to the Same StorageClass is allowed
	if *pvcStorageClassName == storageClassName {
		return nil
	}

	// The validation rules as below:
	// CDI volume cannot be exported to a Longhorn v1 StorageClass
	// Longhorn v1 volume can be exported to any StorageClass (both CDI/BackingImage work well)

	if isCDIVolume(pvcSC) {
		// means CDI volume
		if sc.Provisioner == util.CSIProvisionerLonghorn {
			v, found := sc.Parameters["dataEngine"]
			if found {
				// check engine type
				if v == string(longhornv1.DataEngineTypeV2) {
					return nil
				}
			}
			return fmt.Errorf("CDI volume cannot be exported to a Longhorn v1 StorageClass")
		}
	}

	return nil
}

func (h *ActionHandler) isLonghornV1Engine(sc *storagev1.StorageClass) bool {
	if sc.Provisioner != util.CSIProvisionerLonghorn {
		return false
	}
	if v, found := sc.Parameters["dataEngine"]; found {
		return v == string(longhornv1.DataEngineTypeV1)
	}
	return true
}

func (h *ActionHandler) exportVolume(_ context.Context, imageNamespace, imageDisplayName, imageStorageClassName, pvcNamespace, pvcName string) (interface{}, error) {
	vmImageBackend := harvesterv1.VMIBackendBackingImage
	targetSCName := imageStorageClassName
	targetSC, err := h.scCache.Get(targetSCName)
	if err != nil {
		return nil, err
	}
	// check the target Image is the CDI Image
	if isCDIVolume(targetSC) {
		vmImageBackend = harvesterv1.VMIBackendCDI
	}

	vmImage := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "image-",
			Namespace:    imageNamespace,
			Annotations:  map[string]string{},
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			Backend:                vmImageBackend,
			DisplayName:            imageDisplayName,
			SourceType:             harvesterv1.VirtualMachineImageSourceTypeExportVolume,
			PVCName:                pvcName,
			PVCNamespace:           pvcNamespace,
			TargetStorageClassName: targetSCName,
		},
	}

	if targetSCName != "" {
		vmImage.Annotations[util.AnnotationStorageClassName] = targetSCName
	}

	image, err := h.images.Create(vmImage)

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace":  pvcNamespace,
			"name":       pvcName,
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"err":        err,
		}).Error("failed to create image from PVC")
		return nil, err
	}

	return image, nil
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

	provisioner := util.GetProvisionedPVCProvisioner(pvc, h.scCache)
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

func isCDIVolume(sc *storagev1.StorageClass) bool {
	if sc.Provisioner != util.CSIProvisionerLonghorn {
		return true
	}
	if v, found := sc.Parameters["dataEngine"]; found {
		if v == string(longhornv1.DataEngineTypeV2) {
			return true
		}
	}
	return false
}
