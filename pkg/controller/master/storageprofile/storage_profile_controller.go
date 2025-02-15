package storageprofile

import (
	"reflect"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
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
	profileSpec := h.generateProfileSpec(sc)

	profileCpy := profile.DeepCopy()
	profileCpy.Spec = profileSpec
	if !reflect.DeepEqual(profile, profileCpy) {
		return h.storageProfileController.Update(profileCpy)
	}
	return profile, nil
}

func (h *storageProfileHandler) generateProfileSpec(sc *v1.StorageClass) cdiv1.StorageProfileSpec {
	switch sc.Provisioner {
	case util.CSIProvisionerLonghorn:
		if sc.Parameters != nil {
			if _, ok := sc.Parameters["dataEngine"]; ok {
				if sc.Parameters["dataEngine"] == string(longhornv1.DataEngineTypeV2) {
					copyStrategy := cdiv1.CloneStrategyHostAssisted
					snapshotClass := "longhorn-snapshot"
					return cdiv1.StorageProfileSpec{
						CloneStrategy: &copyStrategy,
						SnapshotClass: &snapshotClass,
					}
				}
			}
		}
		return cdiv1.StorageProfileSpec{}
	default:
		return cdiv1.StorageProfileSpec{}
	}
}
