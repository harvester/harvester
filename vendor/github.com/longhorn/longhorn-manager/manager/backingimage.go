package manager

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	DataSourceTypeExportFromVolumeParameterExportType      = "export-type"
	DataSourceTypeExportFromVolumeParameterExportTypeRAW   = "raw"
	DataSourceTypeExportFromVolumeParameterExportTypeQCOW2 = "qcow2"
)

func (m *VolumeManager) ListBackingImages() (map[string]*longhorn.BackingImage, error) {
	return m.ds.ListBackingImages()
}

func (m *VolumeManager) ListBackingImagesSorted() ([]*longhorn.BackingImage, error) {
	backingImageMap, err := m.ds.ListBackingImages()
	if err != nil {
		return []*longhorn.BackingImage{}, err
	}

	backingImages := make([]*longhorn.BackingImage, len(backingImageMap))
	backingImageNames, err := sortKeys(backingImageMap)
	if err != nil {
		return []*longhorn.BackingImage{}, err
	}
	for i, backingImageName := range backingImageNames {
		backingImages[i] = backingImageMap[backingImageName]
	}
	return backingImages, nil
}

func (m *VolumeManager) GetBackingImage(name string) (*longhorn.BackingImage, error) {
	return m.ds.GetBackingImage(name)
}

func (m *VolumeManager) ListBackingImageDataSources() (map[string]*longhorn.BackingImageDataSource, error) {
	return m.ds.ListBackingImageDataSources()
}

func (m *VolumeManager) GetBackingImageDataSource(name string) (*longhorn.BackingImageDataSource, error) {
	return m.ds.GetBackingImageDataSource(name)
}

func (m *VolumeManager) GetBackingImageDataSourcePod(name string) (*v1.Pod, error) {
	pod, err := m.ds.GetPod(types.GetBackingImageDataSourcePodName(name))
	if err != nil {
		return nil, err
	}
	if pod == nil || pod.Labels[types.GetLonghornLabelKey(types.LonghornLabelBackingImageDataSource)] != name {
		return nil, fmt.Errorf("cannot find pod for backing image data source %v", name)
	}
	return pod, nil
}

func (m *VolumeManager) GetDefaultBackingImageManagersByDiskUUID(diskUUID string) (*longhorn.BackingImageManager, error) {
	defaultImage, err := m.ds.GetSettingValueExisted(types.SettingNameDefaultBackingImageManagerImage)
	if err != nil {
		return nil, err
	}

	bims, err := m.ds.ListBackingImageManagersByDiskUUID(diskUUID)
	if err != nil {
		return nil, err
	}
	for _, bim := range bims {
		if bim.Spec.Image == defaultImage {
			return bim, nil
		}
	}
	return nil, fmt.Errorf("default backing image manager for disk %v is not found", diskUUID)
}

func (m *VolumeManager) CreateBackingImage(name, checksum, sourceType string, parameters map[string]string) (bi *longhorn.BackingImage, err error) {
	bi = &longhorn.BackingImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: types.GetBackingImageLabels(),
		},
		Spec: longhorn.BackingImageSpec{
			Disks:            map[string]string{},
			Checksum:         checksum,
			SourceType:       longhorn.BackingImageDataSourceType(sourceType),
			SourceParameters: parameters,
		},
	}
	if bi, err = m.ds.CreateBackingImage(bi); err != nil {
		return nil, err
	}

	logrus.Infof("Created backing image %v", name)
	return bi, nil
}

func (m *VolumeManager) DeleteBackingImage(name string) error {
	if err := m.ds.DeleteBackingImage(name); err != nil {
		return err
	}
	logrus.Infof("Deleting backing image %v", name)
	return nil
}

func (m *VolumeManager) CleanUpBackingImageDiskFiles(name string, diskFileList []string) (bi *longhorn.BackingImage, err error) {
	defer logrus.Infof("Cleaning up backing image %v in diskFileList %+v", name, diskFileList)

	bi, err = m.GetBackingImage(name)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get backing image %v", name)
	}
	if bi.DeletionTimestamp != nil {
		logrus.Infof("Deleting backing image %v, there is no need to do disk cleanup for it", name)
		return bi, nil
	}
	if bi.Spec.Disks == nil {
		logrus.Infof("backing image %v has not disk required, there is no need to do cleanup then", name)
		return bi, nil
	}
	bids, err := m.GetBackingImageDataSource(name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, errors.Wrapf(err, "unable to get backing image data source %v", name)
		}
		logrus.Warnf("Cannot find backing image data source %v, will ignore it and continue clean up", name)
	}

	existingBI := bi.DeepCopy()
	defer func() {
		if err == nil {
			if !reflect.DeepEqual(bi.Spec, existingBI.Spec) {
				bi, err = m.ds.UpdateBackingImage(bi)
				return
			}
		}
	}()

	replicas, err := m.ds.ListReplicasByBackingImage(name)
	if err != nil {
		return nil, err
	}
	disksInUse := map[string]struct{}{}
	for _, r := range replicas {
		disksInUse[r.Spec.DiskID] = struct{}{}
	}
	if bids != nil && !bids.Spec.FileTransferred {
		disksInUse[bids.Spec.DiskUUID] = struct{}{}
	}
	cleanupFileMap := map[string]struct{}{}
	for _, diskUUID := range diskFileList {
		if _, exists := disksInUse[diskUUID]; exists {
			return nil, fmt.Errorf("cannot clean up backing image %v in disk %v since there is at least one replica using it", name, diskUUID)
		}
		if _, exists := bi.Spec.Disks[diskUUID]; !exists {
			continue
		}
		delete(bi.Spec.Disks, diskUUID)
		cleanupFileMap[diskUUID] = struct{}{}
	}

	var readyActiveFileCount, handlingActiveFileCount, failedActiveFileCount int
	var readyCleanupFileCount, handlingCleanupFileCount, failedCleanupFileCount int
	for diskUUID := range existingBI.Spec.Disks {
		// Consider non-existing files as pending backing image files.
		fileStatus, exists := bi.Status.DiskFileStatusMap[diskUUID]
		if !exists {
			fileStatus = &longhorn.BackingImageDiskFileStatus{}
		}
		switch fileStatus.State {
		case longhorn.BackingImageStateReadyForTransfer, longhorn.BackingImageStateReady:
			if _, exists := cleanupFileMap[diskUUID]; !exists {
				readyActiveFileCount++
			} else {
				readyCleanupFileCount++
			}
		case longhorn.BackingImageStateFailed:
			if _, exists := cleanupFileMap[diskUUID]; !exists {
				failedActiveFileCount++
			} else {
				failedCleanupFileCount++
			}
		default:
			if _, exists := cleanupFileMap[diskUUID]; !exists {
				handlingActiveFileCount++
			} else {
				handlingCleanupFileCount++
			}
		}
	}

	// TODO: Make `haBackingImageCount` configure when introducing HA backing image feature
	haBackingImageCount := 1
	if haBackingImageCount <= readyActiveFileCount {
		return bi, nil
	}
	if readyCleanupFileCount > 0 {
		return nil, fmt.Errorf("failed to do cleanup since there will be no enough ready files for HA after the deletion")
	}

	if haBackingImageCount <= readyActiveFileCount+handlingCleanupFileCount {
		return bi, nil
	}
	if handlingCleanupFileCount > 0 {
		return nil, fmt.Errorf("failed to do cleanup since there will be no enough ready/in-progress/pending files for HA after the deletion")
	}

	if haBackingImageCount <= readyActiveFileCount+handlingCleanupFileCount+failedCleanupFileCount {
		return bi, nil
	}
	if failedCleanupFileCount > 0 {
		return nil, fmt.Errorf("cannot do cleanup since there are no enough files for HA")
	}

	return bi, nil
}
