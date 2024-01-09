package volumesnapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/pkg/generated/controllers/storage/v1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

type ActionHandler struct {
	pvcs              ctlcorev1.PersistentVolumeClaimClient
	pvcCache          ctlcorev1.PersistentVolumeClaimCache
	volumes           ctllonghornv1.VolumeClient
	volumeCache       ctllonghornv1.VolumeCache
	snapshotCache     ctlsnapshotv1.VolumeSnapshotCache
	storageClassCache ctlstoragev1.StorageClassCache
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
	snapshotName := vars["name"]
	snapshotNamespace := vars["namespace"]

	switch action {
	case actionRestore:
		var input RestoreSnapshotInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.Name == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `name` is required")
		}
		return h.restore(r.Context(), snapshotNamespace, snapshotName, input.Name, input.StorageClassName)
	default:
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
}

func (h *ActionHandler) restore(_ context.Context, snapshotNamespace, snapshotName, newPVCName, storageClassName string) error {
	volumeSnapshot, err := h.snapshotCache.Get(snapshotNamespace, snapshotName)
	if err != nil {
		return err
	}

	sourceStorageClassName := volumeSnapshot.Annotations[util.AnnotationStorageClassName]
	sourceStorageProvisioner := volumeSnapshot.Annotations[util.AnnotationStorageProvisioner]
	sourceImageID := volumeSnapshot.Annotations[util.AnnotationImageID]
	// default to using source storageClass
	if storageClassName == "" {
		storageClassName = sourceStorageClassName
	}
	sc, err := h.storageClassCache.Get(storageClassName)
	if err != nil {
		return apierror.NewAPIError(validation.InvalidBodyContent, fmt.Sprintf("Can not restore volume snapshot with a storageClass that does not exist: %v", err))
	}
	if sourceStorageProvisioner != "" && sc.Provisioner != sourceStorageProvisioner {
		return apierror.NewAPIError(validation.InvalidBodyContent, "Can not use different provisioner for restoring volume snapshot")
	}
	if sourceImageID != "" && storageClassName != sourceStorageClassName {
		return apierror.NewAPIError(validation.InvalidBodyContent, "Can not use different storageClass for restoring image volume snapshot")
	}

	apiGroup := snapshotv1.GroupName
	volumeMode := corev1.PersistentVolumeBlock
	accessModes := []corev1.PersistentVolumeAccessMode{
		corev1.ReadWriteMany,
	}
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceStorage: *volumeSnapshot.Status.RestoreSize,
		},
	}

	newPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        newPVCName,
			Namespace:   snapshotNamespace,
			Annotations: map[string]string{},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      accessModes,
			Resources:        resources,
			StorageClassName: &storageClassName,
			VolumeMode:       &volumeMode,
			DataSource: &corev1.TypedLocalObjectReference{
				Kind:     "VolumeSnapshot",
				Name:     snapshotName,
				APIGroup: &apiGroup,
			},
		},
	}

	if sourceImageID != "" {
		newPVC.Annotations[util.AnnotationImageID] = sourceImageID
	}

	if _, err = h.pvcs.Create(newPVC); err != nil {
		logrus.Errorf("failed to restore volume snapshot %s/%s", snapshotNamespace, snapshotName)
		return err
	}

	if sc.Provisioner == longhorntypes.LonghornDriverName {
		if err = h.mountSourcePVC(volumeSnapshot); err != nil {
			return err
		}
	}

	return nil
}

func (h *ActionHandler) mountSourcePVC(volumeSnapshot *snapshotv1.VolumeSnapshot) error {
	if volumeSnapshot.Spec.Source.PersistentVolumeClaimName == nil {
		return nil
	}

	pvc, err := h.pvcCache.Get(volumeSnapshot.Namespace, *volumeSnapshot.Spec.Source.PersistentVolumeClaimName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("can't find pvc %s/%s, err: %w", volumeSnapshot.Namespace, *volumeSnapshot.Spec.Source.PersistentVolumeClaimName, err)
	}

	volume, err := h.volumeCache.Get(util.LonghornSystemNamespaceName, pvc.Spec.VolumeName)
	if err != nil {
		return fmt.Errorf("failed to get volume %s/%s, error: %s", util.LonghornSystemNamespaceName, pvc.Spec.VolumeName, err.Error())
	}

	if volume.Status.State == lhv1beta2.VolumeStateDetached || volume.Status.State == lhv1beta2.VolumeStateDetaching {
		volCpy := volume.DeepCopy()
		volCpy.Spec.NodeID = volume.Status.OwnerID
		logrus.Infof("mount detached volume %s to the node %s", volCpy.Name, volCpy.Spec.NodeID)
		if _, err = h.volumes.Update(volCpy); err != nil {
			return err
		}
	}
	return nil
}
