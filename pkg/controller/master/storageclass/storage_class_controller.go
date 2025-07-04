package storageclass

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

type storageClassHandler struct {
	storageClassClient       ctlstoragev1.StorageClassClient
	storageProfileClient     ctlcdiv1.StorageProfileClient
	storageProfileCache      ctlcdiv1.StorageProfileCache
	cdiClient                ctlcdiv1.CDIClient
	cdiCache                 ctlcdiv1.CDICache
	volumeSnapshotClassCache ctlsnapshotv1.VolumeSnapshotClassCache
}

func (h *storageClassHandler) OnChanged(_ string, sc *storagev1.StorageClass) (*storagev1.StorageClass, error) {
	if sc == nil || sc.DeletionTimestamp != nil {
		return sc, nil
	}

	if err := h.syncCDI(sc); err != nil {
		return sc, err
	}

	return sc, nil
}

// syncCDI synchronizes the CDI settings and storage profile for the given StorageClass.
func (h *storageClassHandler) syncCDI(sc *storagev1.StorageClass) error {
	// If the storage class is Longhorn v1, we don't need to sync CDI settings or
	// storage profile, as we don't support CDI for Longhorn v1.
	if sc.Provisioner == util.CSIProvisionerLonghorn {
		if v, ok := sc.Parameters["dataEngine"]; ok {
			if v == string(longhornv1.DataEngineTypeV1) {
				return nil
			}
		}
	}

	if err := h.syncCDISettings(sc); err != nil {
		return err
	}
	if err := h.syncStorageProfile(sc); err != nil {
		return err
	}

	return nil
}

// syncCDISettings updates the CDI based on the annotations of the StorageClass.
func (h *storageClassHandler) syncCDISettings(sc *storagev1.StorageClass) error {
	cdi, err := h.cdiCache.Get(util.CDIObjectName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
				Warnf("CDI %s not found, skipping update", sc.Name)
			return nil
		}
		return err
	}

	toUpdate := cdi.DeepCopy()
	// Check FileSystemOverhead
	if v, ok := sc.Annotations[util.AnnotationCDIFSOverhead]; ok {
		// Skip if v is not in regex pattern `^(0(?:\.\d{1,3})?|1)$`
		if !regexp.MustCompile(util.FSOverheadRegex).MatchString(v) {
			logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
				Warnf("Invalid filesystem overhead %s, not match %s, skipping update", v, util.FSOverheadRegex)
			return nil
		}

		if toUpdate.Spec.Config == nil {
			toUpdate.Spec.Config = &cdiv1.CDIConfigSpec{}
		}
		if toUpdate.Spec.Config.FilesystemOverhead == nil {
			toUpdate.Spec.Config.FilesystemOverhead = &cdiv1.FilesystemOverhead{}
		}
		if toUpdate.Spec.Config.FilesystemOverhead.StorageClass == nil {
			toUpdate.Spec.Config.FilesystemOverhead.StorageClass = make(map[string]cdiv1.Percent, 1)
		}
		if string(toUpdate.Spec.Config.FilesystemOverhead.StorageClass[sc.Name]) != v {
			toUpdate.Spec.Config.FilesystemOverhead.StorageClass[sc.Name] = cdiv1.Percent(v)
		}
	} else if toUpdate.Spec.Config.FilesystemOverhead != nil {
		_, ok := toUpdate.Spec.Config.FilesystemOverhead.StorageClass[sc.Name]
		if ok {
			// Remove the storage class from FilesystemOverhead if the annotation is not present
			delete(toUpdate.Spec.Config.FilesystemOverhead.StorageClass, sc.Name)
		}
	}

	if !reflect.DeepEqual(toUpdate, cdi) {
		_, err := h.cdiClient.Update(toUpdate)
		if err != nil {
			logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
				WithError(err).
				Errorf("Failed to update CDI %s", util.CDIObjectName)
			return fmt.Errorf("failed to update CDIConfig: %w", err)
		}
	}

	return nil
}

// syncStorageProfile updates the StorageProfile associated with the given StorageClass
// It checks the annotations of the StorageClass and updates the StorageProfile accordingly.
// The function handles the following annotations:
// - cdi.harvesterhci.io/storageProfileCloneStrategy: updates the CloneStrategy in the StorageProfile.
// - cdi.harvesterhci.io/storageProfileVolumeSnapshotClass: updates the SnapshotClass in the StorageProfile.
// - cdi.harvesterhci.io/storageProfileVolumeModeAccessModes: updates the ClaimPropertySets in the StorageProfile.
// If any of these annotations are not present in the StorageClass, the corresponding fields in the StorageProfile are set to nil (if applicable).
func (h *storageClassHandler) syncStorageProfile(sc *storagev1.StorageClass) error {
	profileName := sc.Name
	profile, err := h.storageProfileCache.Get(profileName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
				Warnf("StorageProfile %s not found, skipping update", profileName)
			return nil
		}
		return err
	}

	toUpdate := profile.DeepCopy()

	updateCloneStrategy(sc, toUpdate)
	if err := h.updateSnapshotClass(sc, toUpdate); err != nil {
		logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
			WithError(err).
			Errorf("Failed to update StorageProfile %s", profileName)
		return err
	}
	updateClaimPropertySets(sc, toUpdate)

	if !reflect.DeepEqual(toUpdate, profile) {
		_, err := h.storageProfileClient.Update(toUpdate)
		if err != nil {
			logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
				WithError(err).
				Errorf("Failed to update StorageProfile %s", profileName)
			return err
		}
	}
	return nil
}

// updateCloneStrategy updates the CloneStrategy in the StorageProfile based on the StorageClass annotation.
func updateCloneStrategy(sc *storagev1.StorageClass, profile *cdiv1.StorageProfile) {
	if v, ok := sc.Annotations[util.AnnotationStorageProfileCloneStrategy]; ok {
		if profile.Spec.CloneStrategy == nil || string(*profile.Spec.CloneStrategy) != v {
			cloneStrategy, err := util.ToCloneStrategy(v)
			if err != nil {
				logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
					WithError(err).
					Warnf("Invalid clone strategy %s", v)
			}
			profile.Spec.CloneStrategy = cloneStrategy
		}
	} else if profile.Spec.CloneStrategy != nil {
		profile.Spec.CloneStrategy = nil
	}
}

// updateSnapshotClass updates the SnapshotClass in the StorageProfile based on the StorageClass annotation.
func (h *storageClassHandler) updateSnapshotClass(sc *storagev1.StorageClass, profile *cdiv1.StorageProfile) error {
	if v, ok := sc.Annotations[util.AnnotationStorageProfileSnapshotClass]; ok {
		if v == "" {
			logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
				Warnf("SnapshotClass annotation is empty, skip updating SnapshotClass in StorageProfile %s", profile.Name)
			return nil
		}
		// check if the corresponding volume snapshot class exists
		if _, err := h.volumeSnapshotClassCache.Get(v); err != nil {
			if apierrors.IsNotFound(err) {
				logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
					Warnf("VolumeSnapshotClass %s not found, skipping update of SnapshotClass in StorageProfile %s", v, profile.Name)
				// If the VolumeSnapshotClass does not exist, we do not update the SnapshotClass in the StorageProfile
				return nil
			}
			return fmt.Errorf("failed to get VolumeSnapshotClass %s: %w", v, err)
		}

		if profile.Spec.SnapshotClass == nil || string(*profile.Spec.SnapshotClass) != v {
			profile.Spec.SnapshotClass = &v
		}
	} else if profile.Spec.SnapshotClass != nil {
		profile.Spec.SnapshotClass = nil
	}
	return nil
}

// updateClaimPropertySets updates the ClaimPropertySets in the StorageProfile based on the StorageClass annotation.
func updateClaimPropertySets(sc *storagev1.StorageClass, profile *cdiv1.StorageProfile) {
	if v, ok := sc.Annotations[util.AnnotationStorageProfileVolumeModeAccessModes]; ok {
		parsed, err := util.ParseVolumeModeAccessModes(v)
		if err != nil {
			logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
				WithError(err).
				Warnf("Invalid JSON for %s", util.AnnotationStorageProfileVolumeModeAccessModes)
		}
		if !equalsClaimPropertySets(profile.Spec.ClaimPropertySets, parsed) {
			profile.Spec.ClaimPropertySets = parsed
		}
	} else if profile.Spec.ClaimPropertySets != nil {
		profile.Spec.ClaimPropertySets = nil
	}
}

// equalsClaimPropertySets compares two slices of ClaimPropertySet.
// It returns true if both slices contain the same ClaimPropertySets, regardless of order.
func equalsClaimPropertySets(a, b []cdiv1.ClaimPropertySet) bool {
	normalize := func(list []cdiv1.ClaimPropertySet) []cdiv1.ClaimPropertySet {
		normalized := make([]cdiv1.ClaimPropertySet, len(list))
		for i, item := range list {
			accessModes := append([]corev1.PersistentVolumeAccessMode(nil), item.AccessModes...)
			// Sort access modes and add to normalized
			slices.Sort(accessModes)
			normalized[i] = cdiv1.ClaimPropertySet{
				AccessModes: accessModes,
				VolumeMode:  item.VolumeMode,
			}
		}
		// Sort volume modes
		sort.Slice(normalized, func(i, j int) bool {
			return *normalized[i].VolumeMode < *normalized[j].VolumeMode
		})
		return normalized
	}

	normA := normalize(a)
	normB := normalize(b)
	return reflect.DeepEqual(normA, normB)
}
