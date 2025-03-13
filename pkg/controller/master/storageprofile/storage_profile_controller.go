package storageprofile

import (
	"reflect"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

// storageProfileHandler dynamically manages storage profiles
type storageProfileHandler struct {
	storageProfileClient     ctlcdiv1.StorageProfileClient
	storageProfileController ctlcdiv1.StorageProfileController
	scClient                 ctlstoragev1.StorageClassClient
}

func (h *storageProfileHandler) OnChanged(_ string, profile *cdiv1.StorageProfile) (*cdiv1.StorageProfile, error) {
	if profile == nil || profile.DeletionTimestamp != nil {
		return profile, nil
	}

	// if storage class is not set, do nothing
	if profile.Status.StorageClass == nil {
		return profile, nil
	}

	targetSCName := *profile.Status.StorageClass
	sc, err := h.scClient.Get(targetSCName, metav1.GetOptions{})
	if err != nil {
		return profile, err
	}
	profileCpy := profile.DeepCopy()
	h.generateProfileSpec(sc, profileCpy)
	if !reflect.DeepEqual(profile, profileCpy) {
		return h.storageProfileController.Update(profileCpy)
	}
	return profile, nil
}

func (h *storageProfileHandler) generateProfileSpec(sc *v1.StorageClass, profile *cdiv1.StorageProfile) *cdiv1.StorageProfile {
	switch sc.Provisioner {
	case util.CSIProvisionerLonghorn:
		snapshotClass := "longhorn-snapshot"
		if sc.Parameters != nil {
			if v, ok := sc.Parameters["dataEngine"]; ok {
				if v == string(longhornv1.DataEngineTypeV2) {
					copyStrategy := cdiv1.CloneStrategyHostAssisted
					profile.Spec.CloneStrategy = &copyStrategy
					profile.Spec.SnapshotClass = &snapshotClass
					return profile
				}
			}
		}
		profile.Spec.SnapshotClass = &snapshotClass
		return profile
	case util.CSIProvisionerLVM:
		volumeMode := corev1.PersistentVolumeBlock
		claimProperty := cdiv1.ClaimPropertySet{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:  &volumeMode,
		}
		for _, property := range profile.Spec.ClaimPropertySets {
			if reflect.DeepEqual(property, claimProperty) {
				return profile
			}
		}
		profile.Spec.ClaimPropertySets = append(profile.Spec.ClaimPropertySets, claimProperty)
		return profile
	default:
		return profile
	}
}
