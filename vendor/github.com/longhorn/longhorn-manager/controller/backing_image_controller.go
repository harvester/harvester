package controller

import (
	"fmt"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	typedv1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type BackingImageController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the backing image
	controllerID   string
	serviceAccount string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced
}

func NewBackingImageController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	namespace string, controllerID, serviceAccount string) *BackingImageController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&typedv1core.EventSinkImpl{Interface: typedv1core.New(kubeClient.CoreV1().RESTClient()).Events("")})

	bic := &BackingImageController{
		baseController: newBaseController("longhorn-backing-image", logger),

		namespace:      namespace,
		controllerID:   controllerID,
		serviceAccount: serviceAccount,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "longhorn-backing-image-controller"}),

		ds: ds,
	}

	ds.BackingImageInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    bic.enqueueBackingImage,
		UpdateFunc: func(old, cur interface{}) { bic.enqueueBackingImage(cur) },
		DeleteFunc: bic.enqueueBackingImage,
	})
	bic.cacheSyncs = append(bic.cacheSyncs, ds.BackingImageInformer.HasSynced)

	ds.BackingImageManagerInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    bic.enqueueBackingImageForBackingImageManager,
		UpdateFunc: func(old, cur interface{}) { bic.enqueueBackingImageForBackingImageManager(cur) },
		DeleteFunc: bic.enqueueBackingImageForBackingImageManager,
	}, 0)
	bic.cacheSyncs = append(bic.cacheSyncs, ds.BackingImageManagerInformer.HasSynced)

	ds.BackingImageDataSourceInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    bic.enqueueBackingImageForBackingImageDataSource,
		UpdateFunc: func(old, cur interface{}) { bic.enqueueBackingImageForBackingImageDataSource(cur) },
		DeleteFunc: bic.enqueueBackingImageForBackingImageDataSource,
	}, 0)
	bic.cacheSyncs = append(bic.cacheSyncs, ds.BackingImageDataSourceInformer.HasSynced)

	ds.ReplicaInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    bic.enqueueBackingImageForReplica,
		UpdateFunc: func(old, cur interface{}) { bic.enqueueBackingImageForReplica(cur) },
		DeleteFunc: bic.enqueueBackingImageForReplica,
	}, 0)
	bic.cacheSyncs = append(bic.cacheSyncs, ds.ReplicaInformer.HasSynced)

	return bic
}

func (bic *BackingImageController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer bic.queue.ShutDown()

	logrus.Infof("Start Longhorn Backing Image controller")
	defer logrus.Infof("Shutting down Longhorn Backing Image controller")

	if !cache.WaitForNamedCacheSync("longhorn backing images", stopCh, bic.cacheSyncs...) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(bic.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (bic *BackingImageController) worker() {
	for bic.processNextWorkItem() {
	}
}

func (bic *BackingImageController) processNextWorkItem() bool {
	key, quit := bic.queue.Get()

	if quit {
		return false
	}
	defer bic.queue.Done(key)

	err := bic.syncBackingImage(key.(string))
	bic.handleErr(err, key)

	return true
}

func (bic *BackingImageController) handleErr(err error, key interface{}) {
	if err == nil {
		bic.queue.Forget(key)
		return
	}

	if bic.queue.NumRequeues(key) < maxRetries {
		logrus.Warnf("Error syncing Longhorn backing image %v: %v", key, err)
		bic.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	logrus.Warnf("Dropping Longhorn backing image %v out of the queue: %v", key, err)
	bic.queue.Forget(key)
}

func getLoggerForBackingImage(logger logrus.FieldLogger, bi *longhorn.BackingImage) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"backingImageName": bi.Name,
		},
	)
}

func (bic *BackingImageController) syncBackingImage(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "fail to sync backing image for %v", key)
	}()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != bic.namespace {
		return nil
	}

	backingImage, err := bic.ds.GetBackingImage(name)
	if err != nil {
		if !datastore.ErrorIsNotFound(err) {
			bic.logger.WithField("backingImage", name).WithError(err).Error("Failed to retrieve backing image from datastore")
			return err
		}
		bic.logger.WithField("backingImage", name).Debug("Can't find backing image, may have been deleted")
		return nil
	}

	log := getLoggerForBackingImage(bic.logger, backingImage)

	if !bic.isResponsibleFor(backingImage) {
		return nil
	}
	if backingImage.Status.OwnerID != bic.controllerID {
		backingImage.Status.OwnerID = bic.controllerID
		backingImage, err = bic.ds.UpdateBackingImageStatus(backingImage)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
		log.Infof("Backing Image got new owner %v", bic.controllerID)
	}

	if backingImage.DeletionTimestamp != nil {
		replicas, err := bic.ds.ListReplicasByBackingImage(backingImage.Name)
		if err != nil {
			return err
		}
		if len(replicas) != 0 {
			log.Info("Need to wait for all replicas stopping using this backing image before removing the finalizer")
			return nil
		}
		log.Info("No replica is using this backing image, will clean up the record for backing image managers and remove the finalizer then")
		if err := bic.cleanupBackingImageManagers(backingImage); err != nil {
			return err
		}
		return bic.ds.RemoveFinalizerForBackingImage(backingImage)
	}

	// UUID is immutable once it's set.
	// Should make sure UUID is not empty before syncing with other resources.
	if backingImage.Status.UUID == "" {
		backingImage.Status.UUID = util.RandomID()
		if backingImage, err = bic.ds.UpdateBackingImageStatus(backingImage); err != nil {
			if !apierrors.IsConflict(errors.Cause(err)) {
				return err
			}
			log.WithError(err).Debugf("Requeue %v due to conflict", key)
			bic.enqueueBackingImage(backingImage)
			return nil
		}
		bic.eventRecorder.Eventf(backingImage, corev1.EventTypeNormal, EventReasonUpdate, "Initialized UUID to %v", backingImage.Status.UUID)
	}

	existingBackingImage := backingImage.DeepCopy()
	defer func() {
		if err != nil {
			return
		}
		if reflect.DeepEqual(existingBackingImage.Status, backingImage.Status) {
			return
		}
		if _, err := bic.ds.UpdateBackingImageStatus(backingImage); err != nil && apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Debugf("Requeue %v due to conflict", key)
			bic.enqueueBackingImage(backingImage)
		}
	}()

	if backingImage.Status.DiskFileStatusMap == nil {
		backingImage.Status.DiskFileStatusMap = map[string]*longhorn.BackingImageDiskFileStatus{}
	}
	if backingImage.Status.DiskLastRefAtMap == nil {
		backingImage.Status.DiskLastRefAtMap = map[string]string{}
	}

	if err := bic.handleBackingImageDataSource(backingImage); err != nil {
		return err
	}

	// We cannot continue without `Spec.Disks`. The backing image data source controller can update it.
	if backingImage.Spec.Disks == nil {
		return nil
	}

	if err := bic.handleBackingImageManagers(backingImage); err != nil {
		return err
	}

	if err := bic.syncBackingImageFileInfo(backingImage); err != nil {
		return err
	}

	if err := bic.updateDiskLastReferenceMap(backingImage); err != nil {
		return err
	}

	return nil
}

func (bic *BackingImageController) cleanupBackingImageManagers(bi *longhorn.BackingImage) (err error) {
	defaultImage, err := bic.ds.GetSettingValueExisted(types.SettingNameDefaultBackingImageManagerImage)
	if err != nil {
		return err
	}

	log := getLoggerForBackingImage(bic.logger, bi)

	bimMap, err := bic.ds.ListBackingImageManagers()
	if err != nil {
		return err
	}
	for _, bim := range bimMap {
		if bim.DeletionTimestamp != nil {
			continue
		}
		bimLog := log.WithField("backingImageManager", bim.Name)
		// Directly clean up old backing image managers (including incompatible managers).
		// New backing image managers can detect and reuse the existing backing image files if necessary.
		if bim.Spec.Image != defaultImage {
			bimLog.Info("Deleting old/non-default backing image manager")
			if err := bic.ds.DeleteBackingImageManager(bim.Name); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			bimLog.Info("Deleted old/non-default backing image manager")
			bic.eventRecorder.Eventf(bi, corev1.EventTypeNormal, EventReasonDelete, "deleted old/non-default backing image manager %v in disk %v on node %v", bim.Name, bim.Spec.DiskUUID, bim.Spec.NodeID)
			continue
		}

		// This sync loop cares about the backing image managers related to the current backing image only.
		if _, isRelatedToCurrentBI := bim.Spec.BackingImages[bi.Name]; !isRelatedToCurrentBI {
			continue
		}
		// The entry in the backing image manager matches the current backing image.
		if _, isStillRequiredByCurrentBI := bi.Spec.Disks[bim.Spec.DiskUUID]; isStillRequiredByCurrentBI && bi.DeletionTimestamp == nil {
			if bim.Spec.BackingImages[bi.Name] == bi.Status.UUID {
				continue
			}
		}

		// The current backing image doesn't require this manager any longer, or the entry in backing image manager doesn't match the backing image uuid.
		delete(bim.Spec.BackingImages, bi.Name)
		if bim, err = bic.ds.UpdateBackingImageManager(bim); err != nil {
			return err
		}
		if len(bim.Spec.BackingImages) == 0 {
			if err := bic.ds.DeleteBackingImageManager(bim.Name); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			bic.eventRecorder.Eventf(bi, corev1.EventTypeNormal, EventReasonDelete, "deleting unused backing image manager %v in disk %v on node %v", bim.Name, bim.Spec.DiskUUID, bim.Spec.NodeID)
			continue
		}
	}

	return nil
}

func (bic *BackingImageController) handleBackingImageDataSource(bi *longhorn.BackingImage) (err error) {
	log := getLoggerForBackingImage(bic.logger, bi)

	bids, err := bic.ds.GetBackingImageDataSource(bi.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if bids == nil {
		log.Debug("Cannot find backing image data source, then controller will create it first")
		var readyDiskUUID, readyDiskPath, readyNodeID string
		isReadyFile := false
		foundReadyDisk := false
		if bi.Spec.Disks != nil {
			for diskUUID := range bi.Spec.Disks {
				node, diskName, err := bic.ds.GetReadyDiskNode(diskUUID)
				if err != nil {
					if !types.ErrorIsNotFound(err) {
						return err
					}
					continue
				}
				foundReadyDisk = true
				readyNodeID = node.Name
				readyDiskUUID = diskUUID
				readyDiskPath = node.Spec.Disks[diskName].Path
				// Prefer to pick up a disk contains the ready file if possible.
				if fileStatus, ok := bi.Status.DiskFileStatusMap[diskUUID]; ok && fileStatus.State == longhorn.BackingImageStateReady {
					isReadyFile = true
					break
				}
			}
		}
		if !foundReadyDisk {
			nodes, err := bic.ds.ListNodes()
			if err != nil {
				return err
			}
			for _, node := range nodes {
				if types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeSchedulable).Status != longhorn.ConditionStatusTrue {
					continue
				}
				for diskName, diskStatus := range node.Status.DiskStatus {
					if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Status != longhorn.ConditionStatusTrue {
						continue
					}
					diskSpec, exists := node.Spec.Disks[diskName]
					if !exists {
						continue
					}
					foundReadyDisk = true
					readyNodeID = node.Name
					readyDiskUUID = diskStatus.DiskUUID
					readyDiskPath = diskSpec.Path
					break
				}
				if foundReadyDisk {
					break
				}
			}
		}
		if !foundReadyDisk {
			return fmt.Errorf("cannot find a ready disk for backing image data source creation")
		}

		bids = &longhorn.BackingImageDataSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:            bi.Name,
				OwnerReferences: datastore.GetOwnerReferencesForBackingImage(bi),
			},
			Spec: longhorn.BackingImageDataSourceSpec{
				NodeID:     readyNodeID,
				UUID:       bi.Status.UUID,
				DiskUUID:   readyDiskUUID,
				DiskPath:   readyDiskPath,
				Checksum:   bi.Spec.Checksum,
				SourceType: bi.Spec.SourceType,
				Parameters: bi.Spec.SourceParameters,
			},
		}
		if isReadyFile {
			bids.Spec.FileTransferred = true
		}
		if bids.Spec.Parameters == nil {
			bids.Spec.Parameters = map[string]string{}
		}
		if bids.Spec.SourceType == longhorn.BackingImageDataSourceTypeExportFromVolume {
			bids.Labels = map[string]string{types.GetLonghornLabelKey(types.LonghornLabelExportFromVolume): bids.Spec.Parameters[DataSourceTypeExportFromVolumeParameterVolumeName]}
		}
		if bids, err = bic.ds.CreateBackingImageDataSource(bids); err != nil {
			return err
		}
	}
	existingBIDS := bids.DeepCopy()

	if bids.Spec.UUID == "" {
		bids.Spec.UUID = bi.Status.UUID
	}

	recoveryWaitIntervalSettingValue, err := bic.ds.GetSettingAsInt(types.SettingNameBackingImageRecoveryWaitInterval)
	if err != nil {
		return err
	}
	recoveryWaitInterval := time.Duration(recoveryWaitIntervalSettingValue) * time.Second

	// If all files in Spec.Disk becomes unavailable and there is no extra ready files.
	allFilesUnavailable := true
	if bi.Spec.Disks != nil {
		for diskUUID := range bi.Spec.Disks {
			fileStatus, ok := bi.Status.DiskFileStatusMap[diskUUID]
			if !ok {
				allFilesUnavailable = false
				break
			}
			if fileStatus.State != longhorn.BackingImageStateFailed {
				allFilesUnavailable = false
				break
			}
			if fileStatus.LastStateTransitionTime == "" {
				allFilesUnavailable = false
				break
			}
			lastStateTransitionTime, err := util.ParseTime(fileStatus.LastStateTransitionTime)
			if err != nil {
				return err
			}
			if lastStateTransitionTime.Add(recoveryWaitInterval).After(time.Now()) {
				allFilesUnavailable = false
				break
			}
		}
	}
	if allFilesUnavailable {
		// Check if there are extra available files outside of Spec.Disks
		for diskUUID, fileStatus := range bi.Status.DiskFileStatusMap {
			if _, exists := bi.Spec.Disks[diskUUID]; exists {
				continue
			}
			if fileStatus.State != longhorn.BackingImageStateFailed {
				allFilesUnavailable = false
				break
			}
			if fileStatus.LastStateTransitionTime == "" {
				allFilesUnavailable = false
				break
			}
			lastStateTransitionTime, err := util.ParseTime(fileStatus.LastStateTransitionTime)
			if err != nil {
				return err
			}
			if lastStateTransitionTime.Add(recoveryWaitInterval).After(time.Now()) {
				allFilesUnavailable = false
				break
			}
		}
	}

	// Check if the data source already finished the 1st file preparing.
	if !bids.Spec.FileTransferred && bids.Status.CurrentState == longhorn.BackingImageStateReady {
		fileStatus, exists := bi.Status.DiskFileStatusMap[bids.Spec.DiskUUID]
		if exists && fileStatus.State == longhorn.BackingImageStateReady {
			bids.Spec.FileTransferred = true
			log.Infof("Default backing image manager successfully took over the file prepared by the backing image data source, will mark the data source as file transferred")
		}
	} else if bids.Spec.FileTransferred && allFilesUnavailable {
		switch bids.Spec.SourceType {
		case longhorn.BackingImageDataSourceTypeDownload:
			log.Info("Prepare to re-download backing image via backing image data source since all existing files become unavailable")
			bids.Spec.FileTransferred = false
		default:
			log.Warnf("Cannot recover backing image after all existing files becoming unavailable, since the backing image data source with type %v doesn't support restarting", bids.Spec.SourceType)
		}
	}

	if !bids.Spec.FileTransferred {
		if _, _, err := bic.ds.GetReadyDiskNode(bids.Spec.DiskUUID); err != nil && types.ErrorIsNotFound(err) {
			nodes, err := bic.ds.ListReadyNodes()
			if err != nil {
				return err
			}
			for _, node := range nodes {
				updated := false
				for diskName, diskSpec := range node.Spec.Disks {
					diskStatus, exists := node.Status.DiskStatus[diskName]
					if !exists {
						continue
					}
					if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Status != longhorn.ConditionStatusTrue {
						continue
					}
					bids.Spec.DiskUUID = diskStatus.DiskUUID
					bids.Spec.DiskPath = diskSpec.Path
					bids.Spec.NodeID = node.Name
					updated = true
				}
				if updated {
					break
				}
			}
		}
	}

	if !reflect.DeepEqual(bids, existingBIDS) {
		if _, err := bic.ds.UpdateBackingImageDataSource(bids); err != nil {
			return err
		}
	}
	return nil
}

func (bic *BackingImageController) handleBackingImageManagers(bi *longhorn.BackingImage) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to handle backing image managers")
	}()

	log := getLoggerForBackingImage(bic.logger, bi)

	if err := bic.cleanupBackingImageManagers(bi); err != nil {
		return err
	}

	defaultImage, err := bic.ds.GetSettingValueExisted(types.SettingNameDefaultBackingImageManagerImage)
	if err != nil {
		return err
	}

	for diskUUID := range bi.Spec.Disks {
		noDefaultBIM := true
		requiredBIs := map[string]string{}

		bimMap, err := bic.ds.ListBackingImageManagersByDiskUUID(diskUUID)
		if err != nil {
			return err
		}
		for _, bim := range bimMap {
			// Add current backing image record to the default manager
			if bim.DeletionTimestamp == nil && bim.Spec.Image == defaultImage {
				if uuidInManager, exists := bim.Spec.BackingImages[bi.Name]; !exists || uuidInManager != bi.Status.UUID {
					bim.Spec.BackingImages[bi.Name] = bi.Status.UUID
					if bim, err = bic.ds.UpdateBackingImageManager(bim); err != nil {
						return err
					}
				}
				noDefaultBIM = false
				break
			}
			// Otherwise, migrate records from non-default managers
			for biName, uuid := range bim.Spec.BackingImages {
				requiredBIs[biName] = uuid
			}
		}

		if noDefaultBIM {
			log.Infof("Cannot find default backing image manager for disk %v, will create it", diskUUID)

			node, diskName, err := bic.ds.GetReadyDiskNode(diskUUID)
			if err != nil {
				if !types.ErrorIsNotFound(err) {
					return err
				}
				log.WithField("diskUUID", diskUUID).WithError(err).Warnf("Disk is not ready hence there is no way to create backing image manager then")
				continue
			}
			requiredBIs[bi.Name] = bi.Status.UUID
			manifest := bic.generateBackingImageManagerManifest(node, diskName, defaultImage, requiredBIs)
			bim, err := bic.ds.CreateBackingImageManager(manifest)
			if err != nil {
				return err
			}

			log.WithFields(logrus.Fields{"backingImageManager": bim.Name, "diskUUID": diskUUID}).Infof("Created default backing image manager")
			bic.eventRecorder.Eventf(bi, corev1.EventTypeNormal, EventReasonCreate, "created default backing image manager %v in disk %v on node %v", bim.Name, bim.Spec.DiskUUID, bim.Spec.NodeID)
		}
	}

	return nil
}

// syncBackingImageFileInfo blindly updates the disk file info based on the results of backing image managers.
func (bic *BackingImageController) syncBackingImageFileInfo(bi *longhorn.BackingImage) (err error) {
	log := getLoggerForBackingImage(bic.logger, bi)
	defer func() {
		err = errors.Wrapf(err, "failed to sync backing image file state")
	}()

	currentDiskFiles := map[string]struct{}{}

	// Sync with backing image data source when the first file is not ready and not taken over by the backing image manager.
	bids, err := bic.ds.GetBackingImageDataSource(bi.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		log.Warn("Cannot find backing image data source, but controller will continue syncing backing image")
	}
	if bids != nil && bids.Status.CurrentState != longhorn.BackingImageStateReady {
		currentDiskFiles[bids.Spec.DiskUUID] = struct{}{}
		// Due to the possible file type conversion (from raw to qcow2), the size of a backing image data source may changed when the file becomes `ready-for-transfer`.
		// To avoid mismatching, the controller will blindly update bi.Status.Size based on bids.Status.Size here.
		if bids.Status.Size != 0 && bids.Status.Size != bi.Status.Size {
			bi.Status.Size = bids.Status.Size
		}
		if err := bic.updateStatusWithFileInfo(bi,
			bids.Spec.DiskUUID, bids.Status.Message, bids.Status.Checksum, bids.Status.CurrentState, bids.Status.Progress); err != nil {
			return err
		}
	}

	// The file info may temporarily become empty when the data source just transfers the file
	// but the backing image manager has not updated the map.

	bimMap, err := bic.ds.ListBackingImageManagers()
	if err != nil {
		return err
	}
	for _, bim := range bimMap {
		if bim.DeletionTimestamp != nil {
			continue
		}
		info, exists := bim.Status.BackingImageFileMap[bi.Name]
		if !exists {
			continue
		}
		if info.UUID != bi.Status.UUID {
			continue
		}
		// If the backing image data source is up and preparing the 1st file,
		// backing image should ignore the file info of the related backing image manager.
		if bids != nil && bids.Spec.DiskUUID == bim.Spec.DiskUUID && bids.Status.CurrentState != longhorn.BackingImageStateReady {
			continue
		}
		currentDiskFiles[bim.Spec.DiskUUID] = struct{}{}
		if err := bic.updateStatusWithFileInfo(bi,
			bim.Spec.DiskUUID, info.Message, info.CurrentChecksum, info.State, info.Progress); err != nil {
			return err
		}
		if info.Size > 0 {
			if bi.Status.Size == 0 {
				bi.Status.Size = info.Size
				bic.eventRecorder.Eventf(bi, corev1.EventTypeNormal, EventReasonUpdate, "Set size to %v", bi.Status.Size)
			}
			if bi.Status.Size != info.Size {
				return fmt.Errorf("BUG: found mismatching size %v reported by backing image manager %v in disk %v, the size recorded in status is %v", info.Size, bim.Name, bim.Spec.DiskUUID, bi.Status.Size)
			}
		}
	}

	for diskUUID := range bi.Status.DiskFileStatusMap {
		if _, exists := currentDiskFiles[diskUUID]; !exists {
			delete(bi.Status.DiskFileStatusMap, diskUUID)
		}
	}

	for diskUUID := range bi.Status.DiskFileStatusMap {
		if bi.Status.DiskFileStatusMap[diskUUID].LastStateTransitionTime == "" {
			bi.Status.DiskFileStatusMap[diskUUID].LastStateTransitionTime = util.Now()
		}
	}

	return nil
}

func (bic *BackingImageController) updateStatusWithFileInfo(bi *longhorn.BackingImage,
	diskUUID, message, checksum string, state longhorn.BackingImageState, progress int) error {
	log := getLoggerForBackingImage(bic.logger, bi)

	if _, exists := bi.Status.DiskFileStatusMap[diskUUID]; !exists {
		bi.Status.DiskFileStatusMap[diskUUID] = &longhorn.BackingImageDiskFileStatus{}
	}
	if bi.Status.DiskFileStatusMap[diskUUID].State != state {
		bi.Status.DiskFileStatusMap[diskUUID].LastStateTransitionTime = util.Now()
		bi.Status.DiskFileStatusMap[diskUUID].State = state
	}
	bi.Status.DiskFileStatusMap[diskUUID].Progress = progress
	bi.Status.DiskFileStatusMap[diskUUID].Message = message

	if checksum != "" {
		if bi.Status.Checksum == "" {
			// This is field is immutable once it is set.
			bi.Status.Checksum = checksum
		}
		if bi.Status.Checksum != checksum {
			if bi.Status.DiskFileStatusMap[diskUUID].State != longhorn.BackingImageStateFailed {
				msg := fmt.Sprintf("Somehow backing image recorded checksum %v doesn't match the file checksum %v in disk %v", bi.Status.Checksum, checksum, diskUUID)
				log.Warn(msg)
				bi.Status.DiskFileStatusMap[diskUUID].State = longhorn.BackingImageStateFailed
				bi.Status.DiskFileStatusMap[diskUUID].Message = msg
			}
		}
	}

	return nil
}

func (bic *BackingImageController) updateDiskLastReferenceMap(bi *longhorn.BackingImage) error {
	replicas, err := bic.ds.ListReplicasByBackingImage(bi.Name)
	if err != nil {
		return err
	}
	filesInUse := map[string]struct{}{}
	for _, replica := range replicas {
		filesInUse[replica.Spec.DiskID] = struct{}{}
		delete(bi.Status.DiskLastRefAtMap, replica.Spec.DiskID)
	}
	for diskUUID := range bi.Status.DiskLastRefAtMap {
		if _, exists := bi.Spec.Disks[diskUUID]; !exists {
			delete(bi.Status.DiskLastRefAtMap, diskUUID)
		}
	}
	for diskUUID := range bi.Spec.Disks {
		_, isActiveFile := filesInUse[diskUUID]
		_, isRecordedHistoricFile := bi.Status.DiskLastRefAtMap[diskUUID]
		if !isActiveFile && !isRecordedHistoricFile {
			bi.Status.DiskLastRefAtMap[diskUUID] = util.Now()
		}
	}

	return nil
}

func (bic *BackingImageController) generateBackingImageManagerManifest(node *longhorn.Node, diskName, defaultImage string, requiredBackingImages map[string]string) *longhorn.BackingImageManager {
	return &longhorn.BackingImageManager{
		ObjectMeta: metav1.ObjectMeta{
			Labels: types.GetBackingImageManagerLabels(node.Name, node.Status.DiskStatus[diskName].DiskUUID),
			Name:   types.GetBackingImageManagerName(defaultImage, node.Status.DiskStatus[diskName].DiskUUID),
		},
		Spec: longhorn.BackingImageManagerSpec{
			Image:         defaultImage,
			NodeID:        node.Name,
			DiskUUID:      node.Status.DiskStatus[diskName].DiskUUID,
			DiskPath:      node.Spec.Disks[diskName].Path,
			BackingImages: requiredBackingImages,
		},
	}
}

func (bic *BackingImageController) enqueueBackingImage(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	bic.queue.Add(key)
}

func (bic *BackingImageController) enqueueBackingImageForBackingImageManager(obj interface{}) {
	bim, isBIM := obj.(*longhorn.BackingImageManager)
	if !isBIM {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		bim, ok = deletedState.Obj.(*longhorn.BackingImageManager)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained non BackingImageManager object: %#v", deletedState.Obj))
			return
		}
	}

	for biName := range bim.Spec.BackingImages {
		key := bim.Namespace + "/" + biName
		bic.queue.Add(key)
	}
	for biName := range bim.Status.BackingImageFileMap {
		key := bim.Namespace + "/" + biName
		bic.queue.Add(key)
	}
}

func (bic *BackingImageController) enqueueBackingImageForBackingImageDataSource(obj interface{}) {
	bic.enqueueBackingImage(obj)
}

func (bic *BackingImageController) enqueueBackingImageForReplica(obj interface{}) {
	replica, isReplica := obj.(*longhorn.Replica)
	if !isReplica {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		replica, ok = deletedState.Obj.(*longhorn.Replica)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained non Replica object: %#v", deletedState.Obj))
			return
		}
	}

	if replica.Spec.BackingImage != "" {
		bic.logger.WithField("replica", replica.Name).WithField("backingImage", replica.Spec.BackingImage).Trace("Enqueuing backing image for replica")
		key := replica.Namespace + "/" + replica.Spec.BackingImage
		bic.queue.Add(key)
		return
	}
}

func (bic *BackingImageController) isResponsibleFor(bi *longhorn.BackingImage) bool {
	return isControllerResponsibleFor(bic.controllerID, bic.ds, bi.Name, "", bi.Status.OwnerID)
}
