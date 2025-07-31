package storageclass

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"time"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/conversion"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

type storageClassHandler struct {
	storageClassController   ctlstoragev1.StorageClassController
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
	if isLonghornV1(sc) {
		return nil
	}

	if err := h.syncCDISettings(sc); err != nil {
		return err
	}
	if err := h.syncStorageProfile(sc); err != nil {
		return err
	}

	return nil
}

func isLonghornV1(sc *storagev1.StorageClass) bool {
	// Check if the storage class is Longhorn v1 by checking the provisioner and parameters
	return sc.Provisioner == util.CSIProvisionerLonghorn &&
		sc.Parameters["dataEngine"] == string(longhornv1.DataEngineTypeV1)
}

// syncCDISettings updates the CDI based on the annotations of the StorageClass.
func (h *storageClassHandler) syncCDISettings(sc *storagev1.StorageClass) error {
	cdi, err := h.cdiCache.Get(util.CDIObjectName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).Warnf("CDI %s not found, skipping update", sc.Name)
			return nil
		}
		return err
	}
	toUpdate := cdi.DeepCopy()
	v, hasAnno := sc.Annotations[util.AnnotationCDIFSOverhead]
	if hasAnno {
		if !regexp.MustCompile(util.FSOverheadRegex).MatchString(v) {
			logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).Warnf("Invalid filesystem overhead %s, not match %s, skipping update", v, util.FSOverheadRegex)
			return nil
		}
		setFilesystemOverhead(toUpdate, sc.Name, v)
	} else {
		removeFilesystemOverhead(toUpdate, sc.Name)
	}
	if reflect.DeepEqual(toUpdate, cdi) {
		return nil
	}
	if _, err := h.cdiClient.Update(toUpdate); err != nil {
		logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).WithError(err).Errorf("Failed to update CDI %s", util.CDIObjectName)
		return fmt.Errorf("failed to update CDIConfig: %w", err)
	}
	return nil
}

// setFilesystemOverhead sets the filesystem overhead for a storage class in the CDI config.
func setFilesystemOverhead(cdi *cdiv1.CDI, scName, value string) {
	if cdi.Spec.Config == nil {
		cdi.Spec.Config = &cdiv1.CDIConfigSpec{}
	}
	if cdi.Spec.Config.FilesystemOverhead == nil {
		cdi.Spec.Config.FilesystemOverhead = &cdiv1.FilesystemOverhead{}
	}
	if cdi.Spec.Config.FilesystemOverhead.StorageClass == nil {
		cdi.Spec.Config.FilesystemOverhead.StorageClass = make(map[string]cdiv1.Percent, 1)
	}
	if string(cdi.Spec.Config.FilesystemOverhead.StorageClass[scName]) != value {
		cdi.Spec.Config.FilesystemOverhead.StorageClass[scName] = cdiv1.Percent(value)
	}
}

// removeFilesystemOverhead removes the filesystem overhead entry for a storage class if it exists.
func removeFilesystemOverhead(cdi *cdiv1.CDI, scName string) {
	if cdi.Spec.Config != nil && cdi.Spec.Config.FilesystemOverhead != nil && cdi.Spec.Config.FilesystemOverhead.StorageClass != nil {
		delete(cdi.Spec.Config.FilesystemOverhead.StorageClass, scName)
	}
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
		// StorageProfile doesn't exist yet. The CDI controller will automatically create
		// one for each StorageClass, so we'll retry in a moment to let that happen.
		if apierrors.IsNotFound(err) {
			logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).Warnf("StorageProfile %s not found, retrying later", profileName)
			h.storageClassController.EnqueueAfter(sc.Name, 1*time.Second)
			return nil
		}
		return err
	}

	toUpdate := profile.DeepCopy()

	toUpdate, err = updateCloneStrategy(sc, toUpdate)
	if err != nil {
		logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
			WithError(err).
			Errorf("Failed to update CloneStrategy in StorageProfile %s", profileName)
		return err
	}

	toUpdate, err = h.updateSnapshotClass(sc, toUpdate)
	if err != nil {
		logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
			WithError(err).
			Errorf("Failed to update SnapshotClass in StorageProfile %s", profileName)
		return err
	}

	toUpdate, err = updateClaimPropertySets(sc, toUpdate)
	if err != nil {
		logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
			WithError(err).
			Errorf("Failed to update ClaimPropertySets in StorageProfile %s", profileName)
		return err
	}

	if reflect.DeepEqual(toUpdate, profile) {
		return nil
	}

	if _, err := h.storageProfileClient.Update(toUpdate); err != nil {
		logrus.WithFields(logrus.Fields{"storageclass": sc.Name}).
			WithError(err).
			Errorf("Failed to update StorageProfile %s", profileName)
		return err
	}
	return nil
}

// updateCloneStrategy updates the CloneStrategy in the StorageProfile based on the StorageClass annotation.
func updateCloneStrategy(sc *storagev1.StorageClass, profile *cdiv1.StorageProfile) (*cdiv1.StorageProfile, error) {
	if v, ok := sc.Annotations[util.AnnotationStorageProfileCloneStrategy]; ok {
		if profile.Spec.CloneStrategy == nil || string(*profile.Spec.CloneStrategy) != v {
			cloneStrategy, err := util.ToCloneStrategy(v)
			if err != nil {
				return nil, fmt.Errorf("invalid clone strategy %s: %w", v, err)
			}
			profile.Spec.CloneStrategy = cloneStrategy
		}
	} else if profile.Spec.CloneStrategy != nil {
		profile.Spec.CloneStrategy = nil
	}
	return profile, nil
}

// updateSnapshotClass updates the SnapshotClass in the StorageProfile based on the StorageClass annotation.
func (h *storageClassHandler) updateSnapshotClass(sc *storagev1.StorageClass, profile *cdiv1.StorageProfile) (*cdiv1.StorageProfile, error) {
	if v, ok := sc.Annotations[util.AnnotationStorageProfileSnapshotClass]; ok {
		if v == "" {
			return nil, fmt.Errorf("SnapshotClass annotation is empty")
		}
		// check if the corresponding volume snapshot class exists
		if _, err := h.volumeSnapshotClassCache.Get(v); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("VolumeSnapshotClass %s not found", v)
			}
			return nil, fmt.Errorf("failed to get VolumeSnapshotClass %s: %w", v, err)
		}

		if profile.Spec.SnapshotClass == nil || string(*profile.Spec.SnapshotClass) != v {
			profile.Spec.SnapshotClass = &v
		}
	} else if profile.Spec.SnapshotClass != nil {
		profile.Spec.SnapshotClass = nil
	}
	return profile, nil
}

// updateClaimPropertySets updates the ClaimPropertySets in the StorageProfile based on the StorageClass annotation.
func updateClaimPropertySets(sc *storagev1.StorageClass, profile *cdiv1.StorageProfile) (*cdiv1.StorageProfile, error) {
	if v, ok := sc.Annotations[util.AnnotationStorageProfileVolumeModeAccessModes]; ok {
		parsed, err := util.ParseVolumeModeAccessModes(v)
		if err != nil {
			return nil, fmt.Errorf("invalid JSON for %s: %w", util.AnnotationStorageProfileVolumeModeAccessModes, err)
		}
		if !equalsClaimPropertySets(profile.Spec.ClaimPropertySets, parsed) {
			profile.Spec.ClaimPropertySets = parsed
		}
	} else if profile.Spec.ClaimPropertySets != nil {
		profile.Spec.ClaimPropertySets = nil
	}
	return profile, nil
}

func claimPropertySetSemanticallyEqual(a, b cdiv1.ClaimPropertySet) bool {
	equalities := conversion.EqualitiesOrDie(
		func(a, b *corev1.PersistentVolumeMode) bool {
			return a == b
		},
		func(a, b []corev1.PersistentVolumeAccessMode) bool {
			slices.Sort(a)
			slices.Sort(b)
			return slices.Equal(a, b)
		},
	)

	return equalities.DeepEqual(a, b)
}

// equalsClaimPropertySets compares two slices of ClaimPropertySet.
// It returns true if both slices contain the same ClaimPropertySets, regardless of order.
func equalsClaimPropertySets(a, b []cdiv1.ClaimPropertySet) bool {
	if len(a) != len(b) {
		return false
	}

	sort.Slice(a, func(i, j int) bool {
		return *(a[i].VolumeMode) < *(a[j].VolumeMode)
	})
	sort.Slice(b, func(i, j int) bool {
		return *(b[i].VolumeMode) < *(b[j].VolumeMode)
	})

	for i := range a {
		if !claimPropertySetSemanticallyEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}
