package controller

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/kubernetes/pkg/controller"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/longhorn/backupstore"

	imutil "github.com/longhorn/longhorn-instance-manager/pkg/util"

	"github.com/longhorn/longhorn-manager/constant"
	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/scheduler"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

var (
	ownerKindVolume = longhorn.SchemeGroupVersion.WithKind("Volume").String()

	RetryInterval = 100 * time.Millisecond
	RetryCounts   = 20

	AutoSalvageTimeLimit = 1 * time.Minute
)

const (
	CronJobBackoffLimit             = 3
	VolumeSnapshotsWarningThreshold = 100

	LastAppliedCronJobSpecAnnotationKeySuffix = "last-applied-cronjob-spec"
)

type VolumeController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string
	// use as the default image of the share manager
	smImage string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced

	scheduler *scheduler.ReplicaScheduler

	backoff *flowcontrol.Backoff

	// for unit test
	nowHandler func() string

	proxyConnCounter util.Counter
}

func NewVolumeController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	namespace,
	controllerID, shareManagerImage string,
	proxyConnCounter util.Counter,
) *VolumeController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events("")})

	c := &VolumeController{
		baseController: newBaseController("longhorn-volume", logger),

		ds:           ds,
		namespace:    namespace,
		controllerID: controllerID,
		smImage:      shareManagerImage,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "longhorn-volume-controller"}),

		backoff: flowcontrol.NewBackOff(time.Minute, time.Minute*3),

		nowHandler: util.Now,

		proxyConnCounter: proxyConnCounter,
	}

	c.scheduler = scheduler.NewReplicaScheduler(ds)

	ds.VolumeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueVolume,
		UpdateFunc: func(old, cur interface{}) { c.enqueueVolume(cur) },
		DeleteFunc: c.enqueueVolume,
	})
	c.cacheSyncs = append(c.cacheSyncs, ds.VolumeInformer.HasSynced)

	ds.EngineInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueControlleeChange,
		UpdateFunc: func(old, cur interface{}) { c.enqueueControlleeChange(cur) },
		DeleteFunc: c.enqueueControlleeChange,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.EngineInformer.HasSynced)

	ds.ReplicaInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueControlleeChange,
		UpdateFunc: func(old, cur interface{}) { c.enqueueControlleeChange(cur) },
		DeleteFunc: c.enqueueControlleeChange,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.ReplicaInformer.HasSynced)

	ds.ShareManagerInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueVolumesForShareManager,
		UpdateFunc: func(old, cur interface{}) { c.enqueueVolumesForShareManager(cur) },
		DeleteFunc: c.enqueueVolumesForShareManager,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.ShareManagerInformer.HasSynced)

	ds.BackupVolumeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, cur interface{}) { c.enqueueVolumesForBackupVolume(cur) },
		DeleteFunc: c.enqueueVolumesForBackupVolume,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.BackupVolumeInformer.HasSynced)

	ds.BackingImageDataSourceInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueVolumesForBackingImageDataSource,
		UpdateFunc: func(old, cur interface{}) { c.enqueueVolumesForBackingImageDataSource(cur) },
		DeleteFunc: c.enqueueVolumesForBackingImageDataSource,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.BackingImageDataSourceInformer.HasSynced)

	ds.NodeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueNodeChange,
		UpdateFunc: func(old, cur interface{}) { c.enqueueNodeChange(cur) },
		DeleteFunc: c.enqueueNodeChange,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.NodeInformer.HasSynced)

	ds.SettingInformer.AddEventHandlerWithResyncPeriod(cache.FilteringResourceEventHandler{
		FilterFunc: isSettingRelatedToVolume,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueueSettingChange,
			UpdateFunc: func(old, cur interface{}) { c.enqueueSettingChange(cur) },
		},
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.SettingInformer.HasSynced)

	return c
}

func (c *VolumeController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting Longhorn volume controller")
	defer c.logger.Info("Shut down Longhorn volume controller")

	if !cache.WaitForNamedCacheSync("longhorn engines", stopCh, c.cacheSyncs...) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(c.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (c *VolumeController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *VolumeController) processNextWorkItem() bool {
	key, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.syncVolume(key.(string))
	c.handleErr(err, key)

	return true
}

func (c *VolumeController) handleErr(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}

	log := c.logger.WithField("Volume", key)
	if c.queue.NumRequeues(key) < maxRetries {
		handleReconcileErrorLogging(log, err, "Failed to sync Longhorn volume")
		c.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	handleReconcileErrorLogging(log, err, "Dropping Longhorn volume out of the queue")
	c.queue.Forget(key)
}

func getLoggerForVolume(logger logrus.FieldLogger, v *longhorn.Volume) *logrus.Entry {
	log := logger.WithFields(
		logrus.Fields{
			"volume":     v.Name,
			"frontend":   v.Spec.Frontend,
			"state":      v.Status.State,
			"owner":      v.Status.OwnerID,
			"accessMode": v.Spec.AccessMode,
			"migratable": v.Spec.Migratable,
		},
	)

	if v.Spec.AccessMode == longhorn.AccessModeReadWriteMany {
		log = log.WithFields(
			logrus.Fields{
				"shareState":    v.Status.ShareState,
				"shareEndpoint": v.Status.ShareEndpoint,
			})
	}

	return log
}

func (c *VolumeController) syncVolume(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to sync %v", key)
	}()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != c.namespace {
		// Not ours, don't do anything
		return nil
	}

	volume, err := c.ds.GetVolume(name)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			return nil
		}
		return err
	}

	log := getLoggerForVolume(c.logger, volume)

	defaultEngineImage, err := c.ds.GetSettingValueExisted(types.SettingNameDefaultEngineImage)
	if err != nil {
		return err
	}
	isResponsible, err := c.isResponsibleFor(volume, defaultEngineImage)
	if err != nil {
		return err
	}
	if !isResponsible {
		return nil
	}

	if volume.Status.OwnerID != c.controllerID {
		volume.Status.OwnerID = c.controllerID
		volume, err = c.ds.UpdateVolumeStatus(volume)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
		log.Infof("Volume got new owner %v", c.controllerID)
	}

	engines, err := c.ds.ListVolumeEngines(volume.Name)
	if err != nil {
		return err
	}
	replicas, err := c.ds.ListVolumeReplicas(volume.Name)
	if err != nil {
		return err
	}
	snapshots, err := c.ds.ListVolumeSnapshotsRO(volume.Name)
	if err != nil {
		return err
	}

	if volume.DeletionTimestamp != nil {
		if volume.Status.State != longhorn.VolumeStateDeleting {
			volume.Status.State = longhorn.VolumeStateDeleting
			volume, err = c.ds.UpdateVolumeStatus(volume)
			if err != nil {
				return err
			}
			c.eventRecorder.Eventf(volume, corev1.EventTypeNormal, constant.EventReasonDelete, "Deleting volume %v", volume.Name)
		}

		if volume.Spec.AccessMode == longhorn.AccessModeReadWriteMany {
			log.Info("Removing share manager for deleted volume")
			if err := c.ds.DeleteShareManager(volume.Name); err != nil && !datastore.ErrorIsNotFound(err) {
				return err
			}
		}

		for _, snap := range snapshots {
			if snap.DeletionTimestamp == nil {
				if err := c.ds.DeleteSnapshot(snap.Name); err != nil {
					return err
				}
			}
		}
		for _, e := range engines {
			if e.DeletionTimestamp == nil {
				if err := c.ds.DeleteEngine(e.Name); err != nil {
					return err
				}
			}
		}
		for _, r := range replicas {
			if r.DeletionTimestamp == nil {
				if err := c.ds.DeleteReplica(r.Name); err != nil {
					return err
				}
			}
		}

		kubeStatus := volume.Status.KubernetesStatus

		if kubeStatus.PVName != "" {
			if err := c.ds.DeletePersistentVolume(kubeStatus.PVName); err != nil {
				if !datastore.ErrorIsNotFound(err) {
					return err
				}
			}
		}

		if kubeStatus.PVCName != "" && kubeStatus.LastPVCRefAt == "" {
			if err := c.ds.DeletePersistentVolumeClaim(kubeStatus.Namespace, kubeStatus.PVCName); err != nil {
				if !datastore.ErrorIsNotFound(err) {
					return err
				}
			}
		}
		vaName := types.GetLHVolumeAttachmentNameFromVolumeName(volume.Name)
		if err := c.ds.DeleteLHVolumeAttachment(vaName); err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		// now volumeattachment, snapshots, replicas, and engines have been marked for deletion
		if engines, err := c.ds.ListVolumeEngines(volume.Name); err != nil {
			return err
		} else if len(engines) > 0 {
			return nil
		}
		if replicas, err := c.ds.ListVolumeReplicas(volume.Name); err != nil {
			return err
		} else if len(replicas) > 0 {
			return nil
		}

		// now snapshots, replicas, and engines are deleted
		return c.ds.RemoveFinalizerForVolume(volume)
	}

	existingVolume := volume.DeepCopy()
	existingEngines := map[string]*longhorn.Engine{}
	for k, e := range engines {
		existingEngines[k] = e.DeepCopy()
	}
	existingReplicas := map[string]*longhorn.Replica{}
	for k, r := range replicas {
		existingReplicas[k] = r.DeepCopy()
	}
	defer func() {
		var lastErr error
		// create/delete engine/replica has been handled already
		// so we only need to worry about entries in the current list
		for k, r := range replicas {
			if existingReplicas[k] == nil ||
				!reflect.DeepEqual(existingReplicas[k].Spec, r.Spec) {
				if _, err := c.ds.UpdateReplica(r); err != nil {
					lastErr = err
				}
			}
		}
		// stop updating if replicas weren't fully updated
		if lastErr == nil {
			for k, e := range engines {
				if existingEngines[k] == nil ||
					!reflect.DeepEqual(existingEngines[k].Spec, e.Spec) {
					if _, err := c.ds.UpdateEngine(e); err != nil {
						lastErr = err
					}
				}
			}
		}
		// stop updating if engines and replicas weren't fully updated
		if lastErr == nil {
			// Make sure that we don't update condition's LastTransitionTime if the condition's values hasn't changed
			handleConditionLastTransitionTime(&existingVolume.Status, &volume.Status)
			if !reflect.DeepEqual(existingVolume.Status, volume.Status) {
				_, lastErr = c.ds.UpdateVolumeStatus(volume)
			}
		}
		if err == nil {
			err = lastErr
		}
		// requeue if it's conflict
		if apierrors.IsConflict(errors.Cause(err)) {
			log.Debugf("Requeue volume due to error %v", err)
			c.enqueueVolume(volume)
			err = nil
		}
	}()

	if err := c.handleVolumeAttachmentCreation(volume); err != nil {
		return err
	}

	if err := c.ReconcileEngineReplicaState(volume, engines, replicas); err != nil {
		return err
	}

	if err := c.syncVolumeUnmapMarkSnapChainRemovedSetting(volume, engines, replicas); err != nil {
		return err
	}

	if err := c.updateRecurringJobs(volume); err != nil {
		return err
	}

	if err := c.upgradeEngineForVolume(volume, engines, replicas); err != nil {
		return err
	}

	if err := c.processMigration(volume, engines, replicas); err != nil {
		return err
	}

	if err := c.ReconcilePersistentVolume(volume); err != nil {
		return err
	}

	if err := c.ReconcileShareManagerState(volume); err != nil {
		return err
	}

	if err := c.ReconcileBackupVolumeState(volume); err != nil {
		return nil
	}

	if err := c.ReconcileVolumeState(volume, engines, replicas); err != nil {
		return err
	}

	if err := c.cleanupReplicas(volume, engines, replicas); err != nil {
		return err
	}

	return nil
}

// handleConditionLastTransitionTime rollback to the existing condition object if condition's values hasn't changed
func handleConditionLastTransitionTime(existingStatus, newStatus *longhorn.VolumeStatus) {
	for i, newCondition := range newStatus.Conditions {
		for _, existingCondition := range existingStatus.Conditions {
			if existingCondition.Type == newCondition.Type &&
				existingCondition.Status == newCondition.Status &&
				existingCondition.Reason == newCondition.Reason &&
				existingCondition.Message == newCondition.Message {
				newStatus.Conditions[i] = existingCondition
			}
		}
	}
}

// EvictReplicas do creating one more replica for eviction, if requested
func (c *VolumeController) EvictReplicas(v *longhorn.Volume,
	e *longhorn.Engine, rs map[string]*longhorn.Replica, healthyCount int) (err error) {
	log := getLoggerForVolume(c.logger, v)

	hasNewReplica := false
	healthyNonEvictingCount := healthyCount
	for _, replica := range rs {
		if replica.Status.EvictionRequested &&
			e.Status.ReplicaModeMap[replica.Name] == longhorn.ReplicaModeRW {
			healthyNonEvictingCount--
		}
		if replica.Spec.HealthyAt == "" && replica.Spec.FailedAt == "" {
			hasNewReplica = true
			break
		}
	}

	if healthyNonEvictingCount < v.Spec.NumberOfReplicas && !hasNewReplica {
		log.Info("Creating one more replica for eviction")
		if err := c.replenishReplicas(v, e, rs, ""); err != nil {
			c.eventRecorder.Eventf(v, corev1.EventTypeWarning,
				constant.EventReasonFailedEviction,
				"volume %v failed to create one more replica", v.Name)
			return errors.Wrap(err, "failed to create new replica for replica eviction")
		}
	}

	return nil
}

func (c *VolumeController) handleVolumeAttachmentCreation(v *longhorn.Volume) error {
	vaName := types.GetLHVolumeAttachmentNameFromVolumeName(v.Name)
	_, err := c.ds.GetLHVolumeAttachment(vaName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		va := longhorn.VolumeAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:   vaName,
				Labels: types.GetVolumeLabels(v.Name),
			},
			Spec: longhorn.VolumeAttachmentSpec{
				AttachmentTickets: make(map[string]*longhorn.AttachmentTicket),
				Volume:            v.Name,
			},
		}
		if _, err := c.ds.CreateLHVolumeAttachment(&va); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileEngineReplicaState will get the current main engine e.Status.ReplicaModeMap, e.Status.RestoreStatus,
// e.Status.purgeStatus, and e.Status.SnapshotCloneStatus then update v and rs accordingly.
func (c *VolumeController) ReconcileEngineReplicaState(v *longhorn.Volume, es map[string]*longhorn.Engine, rs map[string]*longhorn.Replica) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to reconcile engine/replica state for %v", v.Name)
		if v.Status.Robustness != longhorn.VolumeRobustnessDegraded {
			v.Status.LastDegradedAt = ""
		}
	}()

	// Aggregate replica wait for backing image condition
	aggregatedReplicaWaitForBackingImageError := util.NewMultiError()
	waitForBackingImage := false
	for _, r := range rs {
		waitForBackingImageCondition := types.GetCondition(r.Status.Conditions, longhorn.ReplicaConditionTypeWaitForBackingImage)
		if waitForBackingImageCondition.Status == longhorn.ConditionStatusTrue {
			waitForBackingImage = true
			if waitForBackingImageCondition.Reason == longhorn.ReplicaConditionReasonWaitForBackingImageFailed {
				aggregatedReplicaWaitForBackingImageError.Append(util.NewMultiError(waitForBackingImageCondition.Message))
			}
		}
	}
	if waitForBackingImage {
		if len(aggregatedReplicaWaitForBackingImageError) > 0 {
			failureMessage := aggregatedReplicaWaitForBackingImageError.Join()
			v.Status.Conditions = types.SetCondition(v.Status.Conditions, longhorn.VolumeConditionTypeWaitForBackingImage,
				longhorn.ConditionStatusTrue, longhorn.VolumeConditionReasonWaitForBackingImageFailed, failureMessage)
		} else {
			v.Status.Conditions = types.SetCondition(v.Status.Conditions, longhorn.VolumeConditionTypeWaitForBackingImage,
				longhorn.ConditionStatusTrue, longhorn.VolumeConditionReasonWaitForBackingImageWaiting, "")
		}
	} else {
		v.Status.Conditions = types.SetCondition(v.Status.Conditions, longhorn.VolumeConditionTypeWaitForBackingImage,
			longhorn.ConditionStatusFalse, "", "")
	}

	e, err := c.ds.PickVolumeCurrentEngine(v, es)
	if err != nil {
		return err
	}
	if e == nil {
		return nil
	}

	log := getLoggerForVolume(c.logger, v).WithField("currentEngine", e.Name)

	if e.Status.CurrentState == longhorn.InstanceStateUnknown {
		if v.Status.Robustness != longhorn.VolumeRobustnessUnknown {
			v.Status.Robustness = longhorn.VolumeRobustnessUnknown
			c.eventRecorder.Eventf(v, corev1.EventTypeWarning, constant.EventReasonUnknown, "volume %v robustness is unknown", v.Name)
		}
		return nil
	}
	if e.Status.CurrentState != longhorn.InstanceStateRunning {
		// If a replica failed at attaching stage before engine become running,
		// there is no record in e.Status.ReplicaModeMap
		engineInstanceCreationCondition := types.GetCondition(e.Status.Conditions, longhorn.InstanceConditionTypeInstanceCreation)
		isNoAvailableBackend := strings.Contains(engineInstanceCreationCondition.Message, fmt.Sprintf("exit status %v", int(syscall.ENODATA)))
		for _, r := range rs {
			if isNoAvailableBackend || (r.Spec.FailedAt == "" && r.Status.CurrentState == longhorn.InstanceStateError) {
				log.Warnf("Replica %v that not in the engine mode map is marked as failed, current state %v, engine name %v, active %v, no available backend %v",
					r.Name, r.Status.CurrentState, r.Spec.EngineName, r.Spec.Active, isNoAvailableBackend)
				e.Spec.LogRequested = true
				r.Spec.LogRequested = true
				r.Spec.FailedAt = c.nowHandler()
				r.Spec.DesireState = longhorn.InstanceStateStopped
			}
		}
		return nil
	}

	// wait for monitoring to start
	if e.Status.ReplicaModeMap == nil {
		return nil
	}

	replicaList := []*longhorn.Replica{}
	for _, r := range rs {
		replicaList = append(replicaList, r)
	}

	restoreStatusMap := map[string]*longhorn.RestoreStatus{}
	for addr, status := range e.Status.RestoreStatus {
		rName := datastore.ReplicaAddressToReplicaName(addr, replicaList)
		if _, exists := rs[rName]; exists {
			restoreStatusMap[rName] = status
		}
	}

	purgeStatusMap := map[string]*longhorn.PurgeStatus{}
	for addr, status := range e.Status.PurgeStatus {
		rName := datastore.ReplicaAddressToReplicaName(addr, replicaList)
		if _, exists := rs[rName]; exists {
			purgeStatusMap[rName] = status
		}
	}

	// 1. remove ERR replicas
	// 2. count RW replicas
	healthyCount := 0
	for rName, mode := range e.Status.ReplicaModeMap {
		r := rs[rName]
		if r == nil {
			continue
		}
		restoreStatus := restoreStatusMap[rName]
		purgeStatus := purgeStatusMap[rName]
		if mode == longhorn.ReplicaModeERR ||
			(restoreStatus != nil && restoreStatus.Error != "") ||
			(purgeStatus != nil && purgeStatus.Error != "") {
			if restoreStatus != nil && restoreStatus.Error != "" {
				c.eventRecorder.Eventf(v, corev1.EventTypeWarning, constant.EventReasonFailedRestore, "replica %v failed the restore: %s", r.Name, restoreStatus.Error)
			}
			if purgeStatus != nil && purgeStatus.Error != "" {
				c.eventRecorder.Eventf(v, corev1.EventTypeWarning, constant.EventReasonFailedSnapshotPurge, "replica %v failed the snapshot purge: %s", r.Name, purgeStatus.Error)
			}
			if r.Spec.FailedAt == "" {
				log.Warnf("Replica %v is marked as failed, current state %v, mode %v, engine name %v, active %v", r.Name, r.Status.CurrentState, mode, r.Spec.EngineName, r.Spec.Active)
				r.Spec.FailedAt = c.nowHandler()
				e.Spec.LogRequested = true
				r.Spec.LogRequested = true
			}
			r.Spec.DesireState = longhorn.InstanceStateStopped
		} else if mode == longhorn.ReplicaModeRW {
			// record once replica became healthy, so if it
			// failed in the future, we can tell it apart
			// from replica failed during rebuilding
			if r.Spec.HealthyAt == "" {
				c.backoff.DeleteEntry(r.Name)
				r.Spec.HealthyAt = c.nowHandler()
				r.Spec.RebuildRetryCount = 0
			}
			healthyCount++
		}
	}
	// If a replica failed at attaching/migrating stage,
	// there is no record in e.Status.ReplicaModeMap
	for _, r := range rs {
		if r.Spec.FailedAt == "" && r.Status.CurrentState == longhorn.InstanceStateError {
			log.Warnf("Replica %v that not in the engine mode map is marked as failed, current state %v, engine name %v, active %v",
				r.Name, r.Status.CurrentState, r.Spec.EngineName, r.Spec.Active)
			e.Spec.LogRequested = true
			r.Spec.LogRequested = true
			r.Spec.FailedAt = c.nowHandler()
			r.Spec.DesireState = longhorn.InstanceStateStopped
		}
	}

	shouldStop, err := c.shouldStopOfflineReplicaRebuilding(v, healthyCount)
	if err != nil {
		log.WithError(err).Errorf("Failed to check if offline replica rebuilding should be stopped")
		return err
	}
	if shouldStop {
		v.Status.OfflineReplicaRebuildingRequired = false
	}

	// Cannot continue evicting or replenishing replicas during engine migration.
	isMigratingDone := !isVolumeMigrating(v) && len(es) == 1

	oldRobustness := v.Status.Robustness
	if healthyCount == 0 { // no healthy replica exists, going to faulted
		// ReconcileVolumeState() will deal with the faulted case
		return nil
	} else if healthyCount >= v.Spec.NumberOfReplicas {
		v.Status.Robustness = longhorn.VolumeRobustnessHealthy
		if oldRobustness == longhorn.VolumeRobustnessDegraded {
			c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonHealthy, "volume %v became healthy", v.Name)
		}

		if isMigratingDone {
			// Evict replicas for the volume
			if err := c.EvictReplicas(v, e, rs, healthyCount); err != nil {
				return err
			}

			// Migrate local replica when Data Locality is on
			// We turn off data locality while doing auto-attaching or restoring (e.g. frontend is disabled)
			if v.Status.State == longhorn.VolumeStateAttached && !v.Status.FrontendDisabled &&
				isDataLocalityBestEffort(v) && !hasLocalReplicaOnSameNodeAsEngine(e, rs) {
				if err := c.replenishReplicas(v, e, rs, e.Spec.NodeID); err != nil {
					return err
				}
			}

			setting, err := c.getAutoBalancedReplicasSetting(v)
			if err != nil {
				c.logger.Warnf(err.Error())
			}
			if setting != longhorn.ReplicaAutoBalanceDisabled {
				if err := c.replenishReplicas(v, e, rs, ""); err != nil {
					return err
				}
			}
		}

		v.Status.OfflineReplicaRebuildingRequired = false
	} else { // healthyCount < v.Spec.NumberOfReplicas
		v.Status.Robustness = longhorn.VolumeRobustnessDegraded
		if oldRobustness != longhorn.VolumeRobustnessDegraded {
			v.Status.LastDegradedAt = c.nowHandler()
			c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonDegraded, "volume %v became degraded", v.Name)
		}

		cliAPIVersion, err := c.ds.GetEngineImageCLIAPIVersion(e.Status.CurrentImage)
		if err != nil {
			return err
		}
		// Rebuild is not supported when:
		//   1. the volume is being migrating to another node.
		//   2. the volume is old restore/DR volumes.
		//   3. the volume is expanding size.
		isOldRestoreVolume := (v.Status.IsStandby || v.Status.RestoreRequired) && cliAPIVersion < engineapi.CLIVersionFour
		isInExpansion := v.Spec.Size != e.Status.CurrentSize
		if isMigratingDone && !isOldRestoreVolume && !isInExpansion {
			if err := c.replenishReplicas(v, e, rs, ""); err != nil {
				return err
			}
		}
		// replicas will be started by ReconcileVolumeState() later
	}

	for _, status := range e.Status.CloneStatus {
		if status == nil {
			continue
		}

		if status.State == engineapi.ProcessStateComplete && v.Status.CloneStatus.State != longhorn.VolumeCloneStateCompleted {
			v.Status.CloneStatus.State = longhorn.VolumeCloneStateCompleted
			c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonVolumeCloneCompleted,
				"finished cloning snapshot %v from source volume %v",
				v.Status.CloneStatus.Snapshot, v.Status.CloneStatus.SourceVolume)
		} else if status.State == engineapi.ProcessStateError && v.Status.CloneStatus.State != longhorn.VolumeCloneStateFailed {
			v.Status.CloneStatus.State = longhorn.VolumeCloneStateFailed
			c.eventRecorder.Eventf(v, corev1.EventTypeWarning, constant.EventReasonVolumeCloneFailed,
				"failed to clone snapshot %v from source volume %v: %v",
				v.Status.CloneStatus.Snapshot, v.Status.CloneStatus.SourceVolume, status.Error)
		}
	}

	return nil
}

func isAutoSalvageNeeded(rs map[string]*longhorn.Replica) bool {
	if isFirstAttachment(rs) {
		return areAllReplicasFailed(rs)
	}
	return getHealthyAndActiveReplicaCount(rs) == 0 && getFailedReplicaCount(rs) > 0
}

func areAllReplicasFailed(rs map[string]*longhorn.Replica) bool {
	for _, r := range rs {
		if r.Spec.FailedAt == "" {
			return false
		}
	}
	return true
}

func (c *VolumeController) shouldStopOfflineReplicaRebuilding(v *longhorn.Volume, healthyCount int) (bool, error) {
	if v.Spec.BackendStoreDriver != longhorn.BackendStoreDriverTypeV2 {
		return true, nil
	}

	if healthyCount == v.Spec.NumberOfReplicas {
		return true, nil
	}

	if v.Spec.OfflineReplicaRebuilding == longhorn.OfflineReplicaRebuildingDisabled {
		return true, nil
	}

	if v.Spec.OfflineReplicaRebuilding == longhorn.OfflineReplicaRebuildingIgnored {
		offlineReplicaRebuilding, err := c.ds.GetSettingValueExisted(types.SettingNameOfflineReplicaRebuilding)
		if err != nil {
			return false, err
		}
		if offlineReplicaRebuilding == string(longhorn.OfflineReplicaRebuildingDisabled) {
			return true, nil
		}
	}

	return false, nil
}

// isFirstAttachment returns true if this is the first time the volume is attached.
// I.e., all replicas have empty Spec.HealthyAt
func isFirstAttachment(rs map[string]*longhorn.Replica) bool {
	for _, r := range rs {
		if r.Spec.HealthyAt != "" {
			return false
		}
	}
	return true
}

func getHealthyAndActiveReplicaCount(rs map[string]*longhorn.Replica) int {
	count := 0
	for _, r := range rs {
		if r.Spec.FailedAt == "" && r.Spec.HealthyAt != "" && r.Spec.Active {
			count++
		}
	}
	return count
}

func getFailedReplicaCount(rs map[string]*longhorn.Replica) int {
	count := 0
	for _, r := range rs {
		if r.Spec.FailedAt != "" {
			count++
		}
	}
	return count
}

func (c *VolumeController) cleanupReplicas(v *longhorn.Volume, es map[string]*longhorn.Engine, rs map[string]*longhorn.Replica) error {
	// TODO: I don't think it's a good idea to cleanup replicas during a migration or engine image update
	// 	since the getHealthyReplicaCount function doesn't differentiate between replicas of different engines
	// 	then during cleanupExtraHealthyReplicas the condition `healthyCount > v.Spec.NumberOfReplicas` will be true
	//  which can lead to incorrect deletion of replicas.
	//  Allow to delete replicas in `cleanupCorruptedOrStaleReplicas` marked as failed before IM-r started during engine image update.
	if isVolumeMigrating(v) {
		return nil
	}

	e, err := c.ds.PickVolumeCurrentEngine(v, es)
	if err != nil {
		return err
	}

	if err := c.cleanupCorruptedOrStaleReplicas(v, rs); err != nil {
		return err
	}

	// give a chance to delete new replicas failed when upgrading volume and waiting for IM-r starting
	if c.isVolumeUpgrading(v) {
		return nil
	}

	if err := c.cleanupFailedToScheduledReplicas(v, rs); err != nil {
		return err
	}

	if err := c.cleanupExtraHealthyReplicas(v, e, rs); err != nil {
		return err
	}

	return nil
}

func (c *VolumeController) cleanupCorruptedOrStaleReplicas(v *longhorn.Volume, rs map[string]*longhorn.Replica) error {
	healthyCount := getHealthyAndActiveReplicaCount(rs)
	cleanupLeftoverReplicas := !c.isVolumeUpgrading(v) && !isVolumeMigrating(v)
	log := getLoggerForVolume(c.logger, v)

	for _, r := range rs {
		if cleanupLeftoverReplicas {
			if !r.Spec.Active {
				// Leftover by live upgrade or migration. Successful or not, there are replicas left to clean up
				log.Infof("Removing inactive replica %v", r.Name)
				if err := c.deleteReplica(r, rs); err != nil {
					return err
				}
				continue
			} else if r.Spec.EngineImage != v.Spec.EngineImage {
				// r.Spec.Active shouldn't be set for the leftover replicas, something must wrong
				log.WithField("replica", r.Name).Warnf("Replica engine image %v is different from volume engine image %v, "+
					"but replica spec.Active has been set", r.Spec.EngineImage, v.Spec.EngineImage)
			}
		}

		if r.Spec.FailedAt == "" {
			continue
		}

		if r.DeletionTimestamp != nil {
			continue
		}

		if v.Spec.BackendStoreDriver == longhorn.BackendStoreDriverTypeV1 {
			staled := false
			if v.Spec.StaleReplicaTimeout > 0 && util.TimestampAfterTimeout(r.Spec.FailedAt,
				time.Duration(int64(v.Spec.StaleReplicaTimeout*60))*time.Second) {

				staled = true
			}

			// 1. failed for multiple times or failed at rebuilding (`Spec.RebuildRetryCount` of a newly created rebuilding replica
			//    is `FailedReplicaMaxRetryCount`) before ever became healthy/ mode RW,
			// 2. failed too long ago, became stale and unnecessary to keep around, unless we don't have any healthy replicas
			// 3. failed for race condition at upgrading when waiting IM-r to start and it would never became healty
			if (r.Spec.RebuildRetryCount >= scheduler.FailedReplicaMaxRetryCount) || (healthyCount != 0 && staled) || (r.Spec.EngineImage != v.Status.CurrentImage) {
				log.WithField("replica", r.Name).Info("Cleaning up corrupted, staled replica")
				if err := c.deleteReplica(r, rs); err != nil {
					return errors.Wrapf(err, "cannot cleanup staled replica %v", r.Name)
				}
			}
		} else {
			if err := c.deleteReplica(r, rs); err != nil {
				return errors.Wrapf(err, "failed to cleanup staled replica %v", r.Name)
			}
		}
	}

	return nil
}

func (c *VolumeController) cleanupFailedToScheduledReplicas(v *longhorn.Volume, rs map[string]*longhorn.Replica) (err error) {
	healthyCount := getHealthyAndActiveReplicaCount(rs)
	hasEvictionRequestedReplicas := hasReplicaEvictionRequested(rs)

	if healthyCount >= v.Spec.NumberOfReplicas {
		for _, r := range rs {
			if !hasEvictionRequestedReplicas {
				if r.Spec.HealthyAt == "" && r.Spec.NodeID == "" &&
					(isDataLocalityDisabled(v) || r.Spec.HardNodeAffinity != v.Status.CurrentNodeID) {
					logrus.Infof("Cleaning up failed to scheduled replica %v", r.Name)
					if err := c.deleteReplica(r, rs); err != nil {
						return errors.Wrapf(err, "failed to cleanup failed to scheduled replica %v", r.Name)
					}
				}
			}
		}
	}

	return nil
}

func (c *VolumeController) cleanupExtraHealthyReplicas(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica) (err error) {
	healthyCount := getHealthyAndActiveReplicaCount(rs)
	if healthyCount <= v.Spec.NumberOfReplicas {
		return nil
	}

	var cleaned bool
	if cleaned, err = c.cleanupEvictionRequestedReplicas(v, rs); err != nil || cleaned {
		return err
	}

	if cleaned, err = c.cleanupDataLocalityReplicas(v, e, rs); err != nil || cleaned {
		return err
	}

	// Auto-balanced replicas get cleaned based on the sorted order. Hence,
	// cleaning up for data locality replica needs to go first. Or else, a
	// new replica could have a name alphabetically smaller than all the
	// existing replicas. And this causes a rebuilding loop if this new
	// replica is for data locality.
	// Ref: https://github.com/longhorn/longhorn/issues/4761
	if cleaned, err = c.cleanupAutoBalancedReplicas(v, e, rs); err != nil || cleaned {
		return err
	}

	return nil
}

func (c *VolumeController) cleanupEvictionRequestedReplicas(v *longhorn.Volume, rs map[string]*longhorn.Replica) (bool, error) {
	log := getLoggerForVolume(c.logger, v)

	// If there is no non-evicting healthy replica,
	// Longhorn should retain one evicting healthy replica.
	hasNonEvictingHealthyReplica := false
	evictingHealthyReplica := ""
	for _, r := range rs {
		if !datastore.IsAvailableHealthyReplica(r) {
			continue
		}
		if !r.Status.EvictionRequested {
			hasNonEvictingHealthyReplica = true
			break
		}
		evictingHealthyReplica = r.Name
	}

	for _, r := range rs {
		if !r.Status.EvictionRequested {
			continue
		}
		if !hasNonEvictingHealthyReplica && r.Name == evictingHealthyReplica {
			log.Warnf("Failed to evict replica %v for now since there is no other healthy replica", r.Name)
			continue
		}
		if err := c.deleteReplica(r, rs); err != nil {
			c.eventRecorder.Eventf(v, corev1.EventTypeWarning,
				constant.EventReasonFailedEviction,
				"volume %v failed to evict replica %v",
				v.Name, r.Name)
			return false, err
		}
		log.Infof("Evicted replica %v in disk %v of node %v ", r.Name, r.Spec.DiskID, r.Spec.NodeID)
		return true, nil
	}
	return false, nil
}

func (c *VolumeController) cleanupAutoBalancedReplicas(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica) (bool, error) {
	log := getLoggerForVolume(c.logger, v).WithField("replicaAutoBalanceType", "delete")

	setting, err := c.getAutoBalancedReplicasSetting(v)
	if err != nil {
		log.Warnf(err.Error())
	}
	if setting == longhorn.ReplicaAutoBalanceDisabled {
		return false, nil
	}

	var rNames []string
	if setting == longhorn.ReplicaAutoBalanceBestEffort {
		_, rNames, _ = c.getReplicaCountForAutoBalanceBestEffort(v, e, rs, c.getReplicaCountForAutoBalanceNode)
		if len(rNames) == 0 {
			_, rNames, _ = c.getReplicaCountForAutoBalanceBestEffort(v, e, rs, c.getReplicaCountForAutoBalanceZone)
		}
	}
	if len(rNames) == 0 {
		rNames, err = c.getPreferredReplicaCandidatesForDeletion(rs)
		if err != nil {
			return false, err
		}
		log.Infof("Found replica deletion candidates %v", rNames)
	} else {
		log.Infof("Found replica deletion candidates %v with best-effort", rNames)
	}

	// Randomly delete extra non-local healthy replicas in the preferred candidate list rNames
	// Sometime cleanupExtraHealthyReplicas() is called more than once with the same input (v,e,rs).
	// To make the deleting operation idempotent and prevent deleting more replica than needed,
	// we always delete the replica with the smallest name.
	sort.Strings(rNames)
	r := rs[rNames[0]]
	log.Infof("Deleting replica %v", r.Name)
	if err := c.deleteReplica(r, rs); err != nil {
		return false, err
	}
	return true, nil
}

func (c *VolumeController) cleanupDataLocalityReplicas(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica) (bool, error) {
	if !isDataLocalityDisabled(v) &&
		hasLocalReplicaOnSameNodeAsEngine(e, rs) {
		rNames, err := c.getPreferredReplicaCandidatesForDeletion(rs)
		if err != nil {
			return false, err
		}

		// Randomly delete extra non-local healthy replicas in the preferred candidate list rNames
		// Sometime cleanupExtraHealthyReplicas() is called more than once with the same input (v,e,rs).
		// To make the deleting operation idempotent and prevent deleting more replica than needed,
		// we always delete the replica with the smallest name.
		sort.Strings(rNames)
		for _, rName := range rNames {
			r := rs[rName]
			if r.Spec.NodeID != e.Spec.NodeID {
				if err := c.deleteReplica(r, rs); err != nil {
					return false, err
				}
				return true, nil
			}
		}
	}
	return false, nil
}

func (c *VolumeController) getAutoBalancedReplicasSetting(v *longhorn.Volume) (longhorn.ReplicaAutoBalance, error) {
	var setting longhorn.ReplicaAutoBalance

	volumeSetting := v.Spec.ReplicaAutoBalance
	if volumeSetting != longhorn.ReplicaAutoBalanceIgnored {
		setting = volumeSetting
	}

	var err error
	if setting == "" {
		globalSetting, _ := c.ds.GetSettingValueExisted(types.SettingNameReplicaAutoBalance)

		if globalSetting == string(longhorn.ReplicaAutoBalanceIgnored) {
			globalSetting = string(longhorn.ReplicaAutoBalanceDisabled)
		}

		setting = longhorn.ReplicaAutoBalance(globalSetting)
	}

	err = types.ValidateReplicaAutoBalance(longhorn.ReplicaAutoBalance(setting))
	if err != nil {
		setting = longhorn.ReplicaAutoBalanceDisabled
	}
	return setting, errors.Wrapf(err, "replica auto-balance is disabled")
}

func (c *VolumeController) updateReplicaLogRequested(e *longhorn.Engine, rs map[string]*longhorn.Replica) {
	needReplicaLogs := false
	for _, r := range rs {
		if r.Spec.LogRequested && r.Status.LogFetched {
			r.Spec.LogRequested = false
		}
		needReplicaLogs = needReplicaLogs || r.Spec.LogRequested
		rs[r.Name] = r
	}
	if e.Spec.LogRequested && e.Status.LogFetched && !needReplicaLogs {
		e.Spec.LogRequested = false
	}
}

func (c *VolumeController) isUnmapMarkSnapChainRemovedEnabled(v *longhorn.Volume) (bool, error) {
	if v.Spec.UnmapMarkSnapChainRemoved != longhorn.UnmapMarkSnapChainRemovedIgnored {
		return v.Spec.UnmapMarkSnapChainRemoved == longhorn.UnmapMarkSnapChainRemovedEnabled, nil
	}

	return c.ds.GetSettingAsBool(types.SettingNameRemoveSnapshotsDuringFilesystemTrim)
}

func (c *VolumeController) syncVolumeUnmapMarkSnapChainRemovedSetting(v *longhorn.Volume, es map[string]*longhorn.Engine, rs map[string]*longhorn.Replica) error {
	if es == nil && rs == nil {
		return nil
	}

	unmapMarkEnabled, err := c.isUnmapMarkSnapChainRemovedEnabled(v)
	if err != nil {
		return err
	}

	for _, e := range es {
		e.Spec.UnmapMarkSnapChainRemovedEnabled = unmapMarkEnabled
	}
	for _, r := range rs {
		r.Spec.UnmapMarkDiskChainRemovedEnabled = unmapMarkEnabled
	}

	return nil
}

// ReconcileVolumeState handles the attaching and detaching of volume
func (c *VolumeController) ReconcileVolumeState(v *longhorn.Volume, es map[string]*longhorn.Engine, rs map[string]*longhorn.Replica) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to reconcile volume state for %v", v.Name)
	}()

	log := getLoggerForVolume(c.logger, v)

	e, err := c.ds.PickVolumeCurrentEngine(v, es)
	if err != nil {
		return err
	}

	if v.Status.CurrentImage == "" {
		v.Status.CurrentImage = v.Spec.EngineImage
	}

	if err := c.checkAndInitVolumeOfflineReplicaRebuilding(v, rs); err != nil {
		return err
	}

	if err := c.checkAndInitVolumeRestore(v); err != nil {
		return err
	}

	if err := c.updateRequestedBackupForVolumeRestore(v, e); err != nil {
		return err
	}

	if err := c.checkAndInitVolumeClone(v); err != nil {
		return err
	}

	if err := c.updateRequestedDataSourceForVolumeCloning(v, e); err != nil {
		return err
	}

	isNewVolume, e, err := c.reconcileVolumeCreation(v, e, es, rs)
	if err != nil {
		return err
	}
	if e == nil {
		log.Warnf("Engine is nil while reconcile volume creation")
		return nil
	}

	c.reconcileLogRequest(e, rs)

	if err := c.reconcileVolumeCondition(v, e, rs, log); err != nil {
		return err
	}

	if err := c.reconcileVolumeSize(v, e, rs); err != nil {
		return err
	}

	v.Status.FrontendDisabled = v.Spec.DisableFrontend

	// Clear SalvageRequested flag if SalvageExecuted flag has been set.
	if e.Spec.SalvageRequested && e.Status.SalvageExecuted {
		e.Spec.SalvageRequested = false
	}

	if isAutoSalvageNeeded(rs) {
		v.Status.Robustness = longhorn.VolumeRobustnessFaulted

		autoSalvage, err := c.ds.GetSettingAsBool(types.SettingNameAutoSalvage)
		if err != nil {
			return err
		}
		// To make sure that we don't miss the `isAutoSalvageNeeded` event, This IF statement makes sure the `e.Spec.SalvageRequested=true`
		// persist in ETCD before Longhorn salvages the failed replicas in the IF statement below it.
		// More explanation: when all replicas fails, Longhorn tries to set `e.Spec.SalvageRequested=true`
		// and try to detach the volume by setting `v.Status.CurrentNodeID = ""`.
		// Because at the end of volume syncVolume(), Longhorn updates CRs in the order: replicas, engine, volume,
		// when volume changes from v.Status.State == longhorn.VolumeStateAttached to v.Status.State == longhorn.VolumeStateDetached,
		// we know that volume RS has been updated and therefore the engine RS also has been updated and persisted in ETCD.
		// At this moment, Longhorn goes into the IF statement below this IF statement and salvage all replicas.
		if autoSalvage && !v.Status.IsStandby && !v.Status.RestoreRequired {
			// Since all replica failed and autoSalvage is enable, mark engine controller salvage requested
			e.Spec.SalvageRequested = true
			log.Infof("All replicas are failed, set engine salvageRequested to %v", e.Spec.SalvageRequested)
		}
		// make sure the volume is detached before automatically salvage replicas
		if autoSalvage && v.Status.State == longhorn.VolumeStateDetached && !v.Status.IsStandby && !v.Status.RestoreRequired {
			lastFailedAt := time.Time{}
			failedUsableReplicas := map[string]*longhorn.Replica{}
			dataExists := false

			for _, r := range rs {
				if r.Spec.HealthyAt == "" {
					continue
				}
				dataExists = true
				if r.Spec.NodeID == "" || r.Spec.DiskID == "" {
					continue
				}
				if isDownOrDeleted, err := c.ds.IsNodeDownOrDeleted(r.Spec.NodeID); err != nil {
					log.WithField("replica", r.Name).WithError(err).Warnf("Failed to check if node %v is still running for failed replica", r.Spec.NodeID)
					continue
				} else if isDownOrDeleted {
					continue
				}
				node, err := c.ds.GetNode(r.Spec.NodeID)
				if err != nil {
					log.WithField("replica", r.Name).WithError(err).Warnf("Failed to get node %v for failed replica", r.Spec.NodeID)
				}
				diskSchedulable := false
				for _, diskStatus := range node.Status.DiskStatus {
					if diskStatus.DiskUUID == r.Spec.DiskID {
						if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Status == longhorn.ConditionStatusTrue {
							diskSchedulable = true
							break
						}
					}
				}
				if !diskSchedulable {
					continue
				}
				failedAt, err := util.ParseTime(r.Spec.FailedAt)
				if err != nil {
					log.WithField("replica", r.Name).WithError(err).Warn("Failed to parse FailedAt timestamp for replica")
					continue
				}
				if failedAt.After(lastFailedAt) {
					lastFailedAt = failedAt
				}
				// all failedUsableReplica contains data
				failedUsableReplicas[r.Name] = r
			}
			if !dataExists {
				log.Warn("Failed to auto salvage volume: no data exists")
			} else {
				// This salvage is for revision counter enabled case
				salvaged := false
				// Bring up the replicas for auto-salvage
				for _, r := range failedUsableReplicas {
					if util.TimestampWithinLimit(lastFailedAt, r.Spec.FailedAt, AutoSalvageTimeLimit) {
						r.Spec.FailedAt = ""
						log.WithField("replica", r.Name).Warn("Automatically salvaging volume replica")
						msg := fmt.Sprintf("Replica %v of volume %v will be automatically salvaged", r.Name, v.Name)
						c.eventRecorder.Event(v, corev1.EventTypeWarning, constant.EventReasonAutoSalvaged, msg)
						salvaged = true
					}
				}
				if salvaged {
					// remount the reattached volume later if possible
					v.Status.RemountRequestedAt = c.nowHandler()
					msg := fmt.Sprintf("Volume %v requested remount at %v after automatically salvaging replicas", v.Name, v.Status.RemountRequestedAt)
					c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonRemount, msg)
					v.Status.Robustness = longhorn.VolumeRobustnessUnknown
					return nil
				}
			}
		}
	} else { // !isAutoSalvageNeeded
		if v.Status.Robustness == longhorn.VolumeRobustnessFaulted && v.Status.State == longhorn.VolumeStateDetached {
			v.Status.Robustness = longhorn.VolumeRobustnessUnknown
			// The volume was faulty and there are usable replicas.
			// Therefore, we set RemountRequestedAt so that KubernetesPodController restarts the workload pod
			v.Status.RemountRequestedAt = c.nowHandler()
			msg := fmt.Sprintf("Volume %v requested remount at %v", v.Name, v.Status.RemountRequestedAt)
			c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonRemount, msg)
			return nil
		}

		// Reattach volume if
		// - volume is detached unexpectedly and there are still healthy replicas
		// - engine dead unexpectedly and there are still healthy replicas when the volume is not attached
		if e.Status.CurrentState == longhorn.InstanceStateError {
			if v.Status.CurrentNodeID != "" || (v.Spec.NodeID != "" && v.Status.CurrentNodeID == "" && v.Status.State != longhorn.VolumeStateAttached) {
				log.Warn("Reattaching the volume since engine of volume dead unexpectedly")
				msg := fmt.Sprintf("Engine of volume %v dead unexpectedly, reattach the volume", v.Name)
				c.eventRecorder.Event(v, corev1.EventTypeWarning, constant.EventReasonDetachedUnexpectedly, msg)
				e.Spec.LogRequested = true
				for _, r := range rs {
					if r.Status.CurrentState == longhorn.InstanceStateRunning {
						r.Spec.LogRequested = true
						rs[r.Name] = r
					}
				}
				v.Status.Robustness = longhorn.VolumeRobustnessFaulted
			}
		}
	}

	if err := c.reconcileAttachDetachStateMachine(v, e, rs, isNewVolume, log); err != nil {
		return err
	}

	if v.Status.CurrentNodeID != "" &&
		v.Status.State == longhorn.VolumeStateAttached &&
		e.Status.CurrentState == longhorn.InstanceStateRunning {
		if e.Spec.RequestedBackupRestore != "" {
			v.Status.Conditions = types.SetCondition(v.Status.Conditions,
				longhorn.VolumeConditionTypeRestore, longhorn.ConditionStatusTrue, longhorn.VolumeConditionReasonRestoreInProgress, "")
		}

		// TODO: reconcileVolumeSize
		// The engine expansion is complete
		if v.Status.ExpansionRequired && v.Spec.Size == e.Status.CurrentSize {
			v.Status.ExpansionRequired = false
			v.Status.FrontendDisabled = false
		}
	}

	return c.checkAndFinishVolumeRestore(v, e, rs)
}

func (c *VolumeController) reconcileAttachDetachStateMachine(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica, isNewVolume bool, log *logrus.Entry) error {
	//TODO: link the state machine graph here

	if isNewVolume || v.Status.State == "" {
		v.Status.State = longhorn.VolumeStateCreating
		return nil
	}

	if v.Spec.NodeID == "" {
		if v.Status.CurrentNodeID == "" {
			switch v.Status.State {
			case longhorn.VolumeStateAttached, longhorn.VolumeStateAttaching, longhorn.VolumeStateCreating:
				c.closeVolumeDependentResources(v, e, rs)
				v.Status.State = longhorn.VolumeStateDetaching
			case longhorn.VolumeStateDetaching:
				c.closeVolumeDependentResources(v, e, rs)
				if c.verifyVolumeDependentResourcesClosed(e, rs) {
					v.Status.State = longhorn.VolumeStateDetached
					c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonDetached, "volume %v has been detached", v.Name)
				}
			case longhorn.VolumeStateDetached:
				// This is a stable state.
				// We attempt to close the resources anyway to make sure that they are closed
				c.closeVolumeDependentResources(v, e, rs)
			}
			return nil
		}
		if v.Status.CurrentNodeID != "" {
			switch v.Status.State {
			case longhorn.VolumeStateAttaching, longhorn.VolumeStateAttached, longhorn.VolumeStateDetached:
				c.closeVolumeDependentResources(v, e, rs)
				v.Status.State = longhorn.VolumeStateDetaching
			case longhorn.VolumeStateDetaching:
				c.closeVolumeDependentResources(v, e, rs)
				if c.verifyVolumeDependentResourcesClosed(e, rs) {
					v.Status.CurrentNodeID = ""
					v.Status.State = longhorn.VolumeStateDetached
					c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonDetached, "volume %v has been detached", v.Name)
				}
			}
			return nil
		}
	}

	if v.Spec.NodeID != "" {
		if v.Status.CurrentNodeID == "" {
			switch v.Status.State {
			case longhorn.VolumeStateAttached:
				c.closeVolumeDependentResources(v, e, rs)
				v.Status.State = longhorn.VolumeStateDetaching
			case longhorn.VolumeStateDetaching:
				c.closeVolumeDependentResources(v, e, rs)
				if c.verifyVolumeDependentResourcesClosed(e, rs) {
					v.Status.State = longhorn.VolumeStateDetached
					c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonDetached, "volume %v has been detached", v.Name)
				}
			case longhorn.VolumeStateDetached:
				if err := c.openVolumeDependentResources(v, e, rs, log); err != nil {
					return err
				}
				v.Status.State = longhorn.VolumeStateAttaching
			case longhorn.VolumeStateAttaching:
				if err := c.openVolumeDependentResources(v, e, rs, log); err != nil {
					return err
				}
				if c.areVolumeDependentResourcesOpened(e, rs) {
					v.Status.CurrentNodeID = v.Spec.NodeID
					v.Status.State = longhorn.VolumeStateAttached
					c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonAttached, "volume %v has been attached to %v", v.Name, v.Status.CurrentNodeID)
				}
			}
			return nil
		}

		if v.Status.CurrentNodeID != "" {
			if v.Spec.NodeID == v.Status.CurrentNodeID {
				switch v.Status.State {
				case longhorn.VolumeStateAttaching, longhorn.VolumeStateDetached:
					c.closeVolumeDependentResources(v, e, rs)
					v.Status.State = longhorn.VolumeStateDetaching
				case longhorn.VolumeStateDetaching:
					c.closeVolumeDependentResources(v, e, rs)
					if c.verifyVolumeDependentResourcesClosed(e, rs) {
						v.Status.CurrentNodeID = ""
						v.Status.State = longhorn.VolumeStateDetached
						c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonDetached, "volume %v has been detached", v.Name)
					}
				case longhorn.VolumeStateAttached:
					// This is a stable state
					// Try to openVolumeDependentResources so that we start the newly added replicas if they exist
					if err := c.openVolumeDependentResources(v, e, rs, log); err != nil {
						return err
					}
					if !c.areVolumeDependentResourcesOpened(e, rs) {
						log.Warnf("Volume is attached but dependent resources are not opened")
					}
				}
				return nil
			}
			if v.Spec.NodeID != v.Status.CurrentNodeID {
				switch v.Status.State {
				case longhorn.VolumeStateDetached, longhorn.VolumeStateAttaching:
					c.closeVolumeDependentResources(v, e, rs)
					v.Status.State = longhorn.VolumeStateDetaching
				case longhorn.VolumeStateDetaching:
					c.closeVolumeDependentResources(v, e, rs)
					if c.verifyVolumeDependentResourcesClosed(e, rs) {
						v.Status.CurrentNodeID = ""
						v.Status.State = longhorn.VolumeStateDetached
						c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonDetached, "volume %v has been detached", v.Name)
					}
				case longhorn.VolumeStateAttached:
					if v.Spec.Migratable && v.Spec.AccessMode == longhorn.AccessModeReadWriteMany && v.Status.CurrentMigrationNodeID != "" {
						if err := c.openVolumeDependentResources(v, e, rs, log); err != nil {
							return err
						}
						if !c.areVolumeDependentResourcesOpened(e, rs) {
							log.Warnf("Volume is attached but dependent resources are not opened")
						}
					} else {
						c.closeVolumeDependentResources(v, e, rs)
						v.Status.State = longhorn.VolumeStateDetaching
					}
				}
				return nil
			}
		}
	}

	return nil
}

func (c *VolumeController) reconcileVolumeCreation(v *longhorn.Volume, e *longhorn.Engine, es map[string]*longhorn.Engine, rs map[string]*longhorn.Replica) (bool, *longhorn.Engine, error) {
	// first time engine creation etc

	var isNewVolume bool
	var err error

	if len(es) == 0 {
		// first time creation
		e, err = c.createEngine(v, "")
		if err != nil {
			return false, e, err
		}
		isNewVolume = true
		es[e.Name] = e
	}

	if len(rs) == 0 {
		// first time creation
		if err = c.replenishReplicas(v, e, rs, ""); err != nil {
			return false, e, err
		}
	}

	return isNewVolume, e, nil
}

func (c *VolumeController) reconcileLogRequest(e *longhorn.Engine, rs map[string]*longhorn.Replica) {
	needReplicaLogs := false
	for _, r := range rs {
		if r.Spec.LogRequested && r.Status.LogFetched {
			r.Spec.LogRequested = false
		}
		needReplicaLogs = needReplicaLogs || r.Spec.LogRequested
		rs[r.Name] = r
	}
	if e.Spec.LogRequested && e.Status.LogFetched && !needReplicaLogs {
		e.Spec.LogRequested = false
	}
}

func (c *VolumeController) reconcileVolumeCondition(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica, log *logrus.Entry) error {
	if len(e.Status.Snapshots) > VolumeSnapshotsWarningThreshold {
		v.Status.Conditions = types.SetCondition(v.Status.Conditions,
			longhorn.VolumeConditionTypeTooManySnapshots, longhorn.ConditionStatusTrue,
			longhorn.VolumeConditionReasonTooManySnapshots, fmt.Sprintf("Snapshots count is %v over the warning threshold %v", len(e.Status.Snapshots), VolumeSnapshotsWarningThreshold))
	} else {
		v.Status.Conditions = types.SetCondition(v.Status.Conditions,
			longhorn.VolumeConditionTypeTooManySnapshots, longhorn.ConditionStatusFalse,
			"", "")
	}

	scheduled := true
	aggregatedReplicaScheduledError := util.NewMultiError()
	for _, r := range rs {
		// check whether the replica need to be scheduled
		if r.Spec.NodeID != "" {
			continue
		}
		if v.Spec.DataLocality == longhorn.DataLocalityStrictLocal {
			if v.Spec.NodeID == "" {
				continue
			}

			r.Spec.HardNodeAffinity = v.Spec.NodeID
		}
		scheduledReplica, multiError, err := c.scheduler.ScheduleReplica(r, rs, v)
		if err != nil {
			return err
		}
		aggregatedReplicaScheduledError.Append(multiError)

		if scheduledReplica == nil {
			if r.Spec.HardNodeAffinity == "" {
				log.WithField("replica", r.Name).Warn("Failed to schedule replica")
				v.Status.Conditions = types.SetCondition(v.Status.Conditions,
					longhorn.VolumeConditionTypeScheduled, longhorn.ConditionStatusFalse,
					longhorn.VolumeConditionReasonReplicaSchedulingFailure, "")
			} else {
				log.WithField("replica", r.Name).Warnf("Failed to schedule replica of volume with HardNodeAffinity = %v", r.Spec.HardNodeAffinity)
				v.Status.Conditions = types.SetCondition(v.Status.Conditions,
					longhorn.VolumeConditionTypeScheduled, longhorn.ConditionStatusFalse,
					longhorn.VolumeConditionReasonLocalReplicaSchedulingFailure, "")
			}
			scheduled = false
			// requeue the volume to retry to schedule the replica after 30s
			c.enqueueVolumeAfter(v, 30*time.Second)
		} else {
			rs[r.Name] = scheduledReplica
		}
	}

	failureMessage := ""
	if scheduled {
		v.Status.Conditions = types.SetCondition(v.Status.Conditions,
			longhorn.VolumeConditionTypeScheduled, longhorn.ConditionStatusTrue, "", "")
	} else if v.Status.CurrentNodeID == "" {
		allowCreateDegraded, err := c.ds.GetSettingAsBool(types.SettingNameAllowVolumeCreationWithDegradedAvailability)
		if err != nil {
			return err
		}
		if allowCreateDegraded {
			atLeastOneReplicaAvailable := false
			for _, r := range rs {
				if r.Spec.NodeID != "" && r.Spec.FailedAt == "" {
					atLeastOneReplicaAvailable = true
					break
				}
			}
			if atLeastOneReplicaAvailable {
				v.Status.Conditions = types.SetCondition(v.Status.Conditions,
					longhorn.VolumeConditionTypeScheduled, longhorn.ConditionStatusTrue, "",
					"Reset schedulable due to allow volume creation with degraded availability")
				scheduled = true
			}
		}
	}
	if !scheduled {
		if len(aggregatedReplicaScheduledError) == 0 {
			aggregatedReplicaScheduledError.Append(util.NewMultiError(longhorn.ErrorReplicaScheduleSchedulingFailed))
		}
		failureMessage = aggregatedReplicaScheduledError.Join()
		scheduledCondition := types.GetCondition(v.Status.Conditions, longhorn.VolumeConditionTypeScheduled)
		if scheduledCondition.Status == longhorn.ConditionStatusFalse {
			v.Status.Conditions = types.SetCondition(v.Status.Conditions,
				longhorn.VolumeConditionTypeScheduled, longhorn.ConditionStatusFalse,
				scheduledCondition.Reason, failureMessage)
		}
	}

	if err := c.ds.UpdatePVAnnotation(v, types.PVAnnotationLonghornVolumeSchedulingError, failureMessage); err != nil {
		log.Warnf("Failed to update PV annotation for volume %v", v.Name)
	}

	return nil
}

func isVolumeOfflineUpgrade(v *longhorn.Volume) bool {
	return v.Status.State == longhorn.VolumeStateDetached && v.Status.CurrentImage != v.Spec.EngineImage
}

func (c *VolumeController) openVolumeDependentResources(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica, log *logrus.Entry) error {
	if isVolumeOfflineUpgrade(v) {
		log.Info("Waiting for offline volume upgrade to finish")
		return nil
	}

	for _, r := range rs {
		// Don't attempt to start the replica or do anything else if it hasn't been scheduled.
		if r.Spec.NodeID == "" {
			continue
		}
		canIMLaunchReplica, err := c.canInstanceManagerLaunchReplica(r)
		if err != nil {
			return err
		}
		if canIMLaunchReplica {
			if r.Spec.FailedAt == "" && r.Spec.EngineImage == v.Status.CurrentImage {
				if r.Status.CurrentState == longhorn.InstanceStateStopped {
					r.Spec.DesireState = longhorn.InstanceStateRunning
				}
			}
		} else {
			// wait for IM is starting when volume is upgrading
			if c.isVolumeUpgrading(v) {
				continue
			}

			// If the engine isn't attached or the node goes down, the replica can be marked as failed.
			// In the attached mode, we can determine whether the replica can fail by relying on the data plane's connectivity status.
			nodeDeleted, err := c.ds.IsNodeDeleted(r.Spec.NodeID)
			if err != nil {
				return err
			}

			if v.Status.State != longhorn.VolumeStateAttached || nodeDeleted {
				msg := fmt.Sprintf("Replica %v is marked as failed since the volume %v is not attached because the instance manager is unable to launch the replica", r.Name, v.Name)
				if nodeDeleted {
					msg = fmt.Sprintf("Replica %v is marked as failed since the node %v is deleted.", r.Name, r.Spec.NodeID)
				}
				log.WithField("replica", r.Name).Warn(msg)
				if r.Spec.FailedAt == "" {
					r.Spec.FailedAt = c.nowHandler()
				}
				r.Spec.DesireState = longhorn.InstanceStateStopped
			}
		}
		rs[r.Name] = r
	}

	replicaAddressMap := map[string]string{}
	for _, r := range rs {
		// Ignore unscheduled replicas
		if r.Spec.NodeID == "" {
			continue
		}
		if r.Spec.EngineImage != v.Status.CurrentImage {
			continue
		}
		if r.Spec.EngineName != e.Name {
			continue
		}
		if r.Spec.FailedAt != "" {
			continue
		}
		if r.Status.CurrentState == longhorn.InstanceStateError {
			continue
		}
		// wait for all potentially healthy replicas become running
		if r.Status.CurrentState != longhorn.InstanceStateRunning {
			return nil
		}
		if r.Status.IP == "" {
			log.WithField("replica", r.Name).Warn("Replica is running but IP is empty")
			continue
		}
		if r.Status.StorageIP == "" {
			log.WithField("replica", r.Name).Warn("Replica is running but storage IP is empty, need to wait for update")
			continue
		}
		if r.Status.Port == 0 {
			log.WithField("replica", r.Name).Warn("Replica is running but Port is empty")
			continue
		}
		if _, ok := e.Spec.ReplicaAddressMap[r.Name]; !ok && isVolumeMigrating(v) && e.Spec.NodeID == v.Spec.NodeID {
			// The volume is migrating from this engine. Don't allow new replicas to be added until migration is
			// complete per https://github.com/longhorn/longhorn/issues/6961.
			log.WithField("replica", r.Name).Warn("Replica is running, but can't be added while migration is ongoing")
			continue
		}
		replicaAddressMap[r.Name] = imutil.GetURL(r.Status.StorageIP, r.Status.Port)
	}
	if len(replicaAddressMap) == 0 {
		return fmt.Errorf("no healthy or scheduled replica for starting")
	}

	if e.Spec.NodeID != "" && e.Spec.NodeID != v.Spec.NodeID {
		return fmt.Errorf("engine is on node %v vs volume on %v, must detach first",
			e.Spec.NodeID, v.Status.CurrentNodeID)
	}
	e.Spec.NodeID = v.Spec.NodeID
	e.Spec.ReplicaAddressMap = replicaAddressMap
	e.Spec.DesireState = longhorn.InstanceStateRunning
	// The volume may be activated
	e.Spec.DisableFrontend = v.Status.FrontendDisabled
	e.Spec.Frontend = v.Spec.Frontend

	return nil
}

func (c *VolumeController) areVolumeDependentResourcesOpened(e *longhorn.Engine, rs map[string]*longhorn.Replica) bool {
	// At least 1 replica should be running
	hasRunningReplica := false
	for _, r := range rs {
		if r.Status.CurrentState == longhorn.InstanceStateRunning {
			hasRunningReplica = true
			break
		}
	}
	return hasRunningReplica && e.Status.CurrentState == longhorn.InstanceStateRunning
}

func (c *VolumeController) closeVolumeDependentResources(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica) {
	v.Status.Conditions = types.SetCondition(v.Status.Conditions,
		longhorn.VolumeConditionTypeRestore, longhorn.ConditionStatusFalse, "", "")

	if v.Status.Robustness != longhorn.VolumeRobustnessFaulted {
		v.Status.Robustness = longhorn.VolumeRobustnessUnknown
	} else {
		if v.Status.RestoreRequired || v.Status.IsStandby {
			v.Status.Conditions = types.SetCondition(v.Status.Conditions,
				longhorn.VolumeConditionTypeRestore, longhorn.ConditionStatusFalse, longhorn.VolumeConditionReasonRestoreFailure, "All replica restore failed and the volume became Faulted")
		}
	}

	if e.Spec.DesireState != longhorn.InstanceStateStopped || e.Spec.NodeID != "" {
		if v.Status.Robustness == longhorn.VolumeRobustnessFaulted {
			e.Spec.LogRequested = true
		}
		// Prevent this field from being unset when restore/DR volumes crash unexpectedly.
		if !v.Status.RestoreRequired && !v.Status.IsStandby {
			e.Spec.BackupVolume = ""
		}
		e.Spec.RequestedBackupRestore = ""
		e.Spec.NodeID = ""
		e.Spec.DesireState = longhorn.InstanceStateStopped
	}
	// must make sure engine stopped first before stopping replicas
	// otherwise we may corrupt the data
	if e.Status.CurrentState != longhorn.InstanceStateStopped {
		return
	}

	// check if any replica has been RW yet
	dataExists := false
	for _, r := range rs {
		if r.Spec.HealthyAt != "" {
			dataExists = true
			break
		}
	}
	for _, r := range rs {
		if r.Spec.HealthyAt == "" && r.Spec.FailedAt == "" && dataExists {
			// This replica must have been rebuilding. Mark it as failed.
			r.Spec.FailedAt = c.nowHandler()
			// Unscheduled replicas are marked failed here when volume is detached.
			// Check if NodeId or DiskID is empty to avoid deleting reusableFailedReplica when replenished.
			if r.Spec.NodeID == "" || r.Spec.DiskID == "" {
				r.Spec.RebuildRetryCount = scheduler.FailedReplicaMaxRetryCount
			}
		}
		if r.Spec.DesireState != longhorn.InstanceStateStopped {
			if v.Status.Robustness == longhorn.VolumeRobustnessFaulted {
				r.Spec.LogRequested = true
			}
			r.Spec.DesireState = longhorn.InstanceStateStopped
			rs[r.Name] = r
		}
	}
}

func (c *VolumeController) verifyVolumeDependentResourcesClosed(e *longhorn.Engine, rs map[string]*longhorn.Replica) bool {
	allReplicasStopped := func() bool {
		for _, r := range rs {
			if r.Status.CurrentState != longhorn.InstanceStateStopped {
				return false
			}
		}
		return true
	}
	return e.Status.CurrentState == longhorn.InstanceStateStopped && allReplicasStopped()
}

func (c *VolumeController) reconcileVolumeSize(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica) error {
	log := getLoggerForVolume(c.logger, v)

	if e.Status.SnapshotsError == "" {
		actualSize := int64(0)
		for _, snapshot := range e.Status.Snapshots {
			size, err := util.ConvertSize(snapshot.Size)
			if err != nil {
				log.WithField("snapshot", snapshot.Name).WithError(err).Warnf("Failed to parse snapshot size %v", snapshot.Size)
				// continue checking for other snapshots
			}
			actualSize += size
		}
		v.Status.ActualSize = actualSize
	}

	if e == nil || rs == nil {
		return nil
	}
	if e.Spec.VolumeSize == v.Spec.Size {
		return nil
	}

	// The expansion is canceled or hasn't been started
	if e.Status.CurrentSize == v.Spec.Size {
		v.Status.ExpansionRequired = false
		c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonCanceledExpansion,
			"Canceled expanding the volume %v, will automatically detach it", v.Name)
	} else {
		if diskScheduleMultiError, err := c.scheduler.CheckReplicasSizeExpansion(v, e.Spec.VolumeSize, v.Spec.Size); err != nil {
			log.WithError(err).Warnf("Failed to start volume expansion")
			if diskScheduleMultiError != nil {
				failureMessage := diskScheduleMultiError.Join()
				if err := c.ds.UpdatePVAnnotation(v, types.PVAnnotationLonghornVolumeSchedulingError, failureMessage); err != nil {
					log.Warnf("Cannot update PV annotation for volume %v", v.Name)
				}
			}
			return nil
		}
		log.Infof("Expanding volume from size %v to size %v", e.Spec.VolumeSize, v.Spec.Size)
		v.Status.ExpansionRequired = true
	}

	e.Spec.VolumeSize = v.Spec.Size
	for _, r := range rs {
		r.Spec.VolumeSize = v.Spec.Size
	}

	return nil
}

func (c *VolumeController) canInstanceManagerLaunchReplica(r *longhorn.Replica) (bool, error) {
	nodeDown, err := c.ds.IsNodeDownOrDeleted(r.Spec.NodeID)
	if err != nil {
		return false, errors.Wrapf(err, "fail to check IsNodeDownOrDeleted %v", r.Spec.NodeID)
	}
	if nodeDown {
		return false, nil
	}
	// Replica already had IM
	if r.Status.InstanceManagerName != "" {
		return true, nil
	}
	defaultIM, err := c.ds.GetInstanceManagerByInstance(r)
	if err != nil {
		return false, errors.Wrapf(err, "failed to find instance manager for replica %v", r.Name)
	}
	return defaultIM.Status.CurrentState == longhorn.InstanceManagerStateRunning ||
		defaultIM.Status.CurrentState == longhorn.InstanceManagerStateStarting, nil
}

func (c *VolumeController) getPreferredReplicaCandidatesForDeletion(rs map[string]*longhorn.Replica) ([]string, error) {
	diskToReplicaMap := make(map[string][]string)
	nodeToReplicaMap := make(map[string][]string)
	zoneToReplicaMap := make(map[string][]string)

	nodeList, err := c.ds.ListNodes()
	if err != nil {
		return nil, err
	}

	for _, r := range rs {
		diskToReplicaMap[r.Spec.NodeID+r.Spec.DiskID] = append(diskToReplicaMap[r.Spec.NodeID+r.Spec.DiskID], r.Name)
		nodeToReplicaMap[r.Spec.NodeID] = append(nodeToReplicaMap[r.Spec.NodeID], r.Name)
		if node, ok := nodeList[r.Spec.NodeID]; ok {
			zoneToReplicaMap[node.Status.Zone] = append(zoneToReplicaMap[node.Status.Zone], r.Name)
		}
	}

	var deletionCandidates []string

	// prefer to delete replicas on the same disk
	deletionCandidates = findValueWithBiggestLength(diskToReplicaMap)
	if len(deletionCandidates) > 1 {
		return deletionCandidates, nil
	}

	// if all replicas are on different disks, prefer to delete replicas on the same node
	deletionCandidates = findValueWithBiggestLength(nodeToReplicaMap)
	if len(deletionCandidates) > 1 {
		return deletionCandidates, nil
	}

	// if all replicas are on different nodes, prefer to delete replicas on the same zone
	deletionCandidates = findValueWithBiggestLength(zoneToReplicaMap)
	if len(deletionCandidates) > 1 {
		return deletionCandidates, nil
	}

	// if all replicas are on different zones, return all replicas' names in the input RS
	deletionCandidates = make([]string, 0, len(rs))
	for rName := range rs {
		deletionCandidates = append(deletionCandidates, rName)
	}
	return deletionCandidates, nil
}

func findValueWithBiggestLength(m map[string][]string) []string {
	targetKey, currentMax := "", 0
	for k, v := range m {
		if len(v) > currentMax {
			targetKey, currentMax = k, len(v)
		}
	}
	return m[targetKey]
}

func isDataLocalityBestEffort(v *longhorn.Volume) bool {
	return v.Spec.DataLocality == longhorn.DataLocalityBestEffort
}

func isDataLocalityStrictLocal(v *longhorn.Volume) bool {
	return v.Spec.DataLocality == longhorn.DataLocalityStrictLocal
}

func isDataLocalityDisabled(v *longhorn.Volume) bool {
	return string(v.Spec.DataLocality) == "" || v.Spec.DataLocality == longhorn.DataLocalityDisabled
}

// hasLocalReplicaOnSameNodeAsEngine returns true if one of the following condition is satisfied:
// 1. there exist a replica on the same node as engine
// 2. there exist a replica with HardNodeAffinity set to engine's NodeID
func hasLocalReplicaOnSameNodeAsEngine(e *longhorn.Engine, rs map[string]*longhorn.Replica) bool {
	for _, r := range rs {
		if e.Spec.NodeID != "" && (r.Spec.NodeID == e.Spec.NodeID || r.Spec.HardNodeAffinity == e.Spec.NodeID) {
			return true
		}
	}
	return false
}

// replenishReplicas will keep replicas count to v.Spec.NumberOfReplicas
// It will count all the potentially usable replicas, since some replicas maybe
// blank or in rebuilding state
func (c *VolumeController) replenishReplicas(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica, hardNodeAffinity string) error {
	concurrentRebuildingLimit, err := c.ds.GetSettingAsInt(types.SettingNameConcurrentReplicaRebuildPerNodeLimit)
	if err != nil {
		return err
	}
	disableReplicaRebuild := concurrentRebuildingLimit == 0

	// If disabled replica rebuild, skip all the rebuild except first time creation.
	if (len(rs) != 0) && disableReplicaRebuild {
		return nil
	}

	if isVolumeMigrating(v) {
		return nil
	}

	if (len(rs) != 0) && v.Status.State != longhorn.VolumeStateAttached {
		return nil
	}

	if e == nil {
		return fmt.Errorf("replenishReplica needs a valid engine")
	}

	// To prevent duplicate IP for different replicas cause problem
	// Wait for engine to:
	// 1. make sure the existing healthy replicas have shown up in engine.spec.ReplicaAddressMap
	// 2. has recognized all the replicas from the spec.ReplicaAddressMap in the status.ReplicaModeMap
	// 3. has cleaned up the extra entries in the status.ReplicaModeMap
	// https://github.com/longhorn/longhorn/issues/687
	if !c.hasEngineStatusSynced(e, rs) {
		return nil
	}

	if currentRebuilding := getRebuildingReplicaCount(e); currentRebuilding != 0 {
		return nil
	}

	log := getLoggerForVolume(c.logger, v)

	replenishCount, updateNodeAffinity := c.getReplenishReplicasCount(v, rs, e)
	if hardNodeAffinity == "" && updateNodeAffinity != "" {
		hardNodeAffinity = updateNodeAffinity
	}

	newVolume := len(rs) == 0

	// For regular rebuild case or data locality case, rebuild one replica at a time
	if (!newVolume && replenishCount > 0) || hardNodeAffinity != "" {
		replenishCount = 1
	}
	for i := 0; i < replenishCount; i++ {
		var reusableFailedReplica *longhorn.Replica
		// TODO: reuse failed replica for replica rebuilding of SPDK volumes
		if v.Spec.BackendStoreDriver == longhorn.BackendStoreDriverTypeV1 {
			reusableFailedReplica, err = c.scheduler.CheckAndReuseFailedReplica(rs, v, hardNodeAffinity)
			if err != nil {
				return errors.Wrapf(err, "failed to reuse a failed replica during replica replenishment")
			}
		}

		if reusableFailedReplica != nil {
			if !c.backoff.IsInBackOffSinceUpdate(reusableFailedReplica.Name, time.Now()) {
				log.Infof("Failed replica %v will be reused during rebuilding", reusableFailedReplica.Name)
				reusableFailedReplica.Spec.FailedAt = ""
				reusableFailedReplica.Spec.HealthyAt = ""

				if datastore.IsReplicaRebuildingFailed(reusableFailedReplica) {
					reusableFailedReplica.Spec.RebuildRetryCount++
				}
				c.backoff.Next(reusableFailedReplica.Name, time.Now())

				rs[reusableFailedReplica.Name] = reusableFailedReplica
				continue
			}
			log.Warnf("Failed to reuse failed replica %v immediately, backoff period is %v now",
				reusableFailedReplica.Name, c.backoff.Get(reusableFailedReplica.Name).Seconds())
			// Couldn't reuse the replica. Add the volume back to the workqueue to check it later
			c.enqueueVolumeAfter(v, c.backoff.Get(reusableFailedReplica.Name))
		}
		if checkBackDuration := c.scheduler.RequireNewReplica(rs, v, hardNodeAffinity); checkBackDuration == 0 {
			if err := c.createReplica(v, e, rs, hardNodeAffinity, !newVolume); err != nil {
				return err
			}
		} else {
			// Couldn't create new replica. Add the volume back to the workqueue to check it later
			c.enqueueVolumeAfter(v, checkBackDuration)
		}
	}
	return nil
}

func getRebuildingReplicaCount(e *longhorn.Engine) int {
	rebuilding := 0
	replicaExists := make(map[string]bool)
	// replicas are currently rebuilding
	for replica, mode := range e.Status.ReplicaModeMap {
		replicaExists[replica] = true
		if mode == longhorn.ReplicaModeWO {
			rebuilding++
		}
	}
	// replicas are to be rebuilt
	for replica := range e.Status.CurrentReplicaAddressMap {
		if !replicaExists[replica] {
			rebuilding++
		}
	}
	return rebuilding
}

type replicaAutoBalanceCount func(*longhorn.Volume, *longhorn.Engine, map[string]*longhorn.Replica) (int, map[string][]string, error)

func (c *VolumeController) getReplicaCountForAutoBalanceLeastEffort(v *longhorn.Volume, e *longhorn.Engine,
	rs map[string]*longhorn.Replica, fnCount replicaAutoBalanceCount) int {
	log := getLoggerForVolume(c.logger, v).WithField("replicaAutoBalanceOption", longhorn.ReplicaAutoBalanceLeastEffort)

	var err error
	defer func() {
		if err != nil {
			log.WithError(err).Warn("Skip replica auto-balance")
		}
	}()

	setting, err := c.getAutoBalancedReplicasSetting(v)
	// Verifying `least-effort` and `best-effort` here because we've set
	// replica auto-balance to always try adjusting replica count with
	// `least-effort` first to achieve minimal redundancy.
	enabled := []string{
		string(longhorn.ReplicaAutoBalanceLeastEffort),
		string(longhorn.ReplicaAutoBalanceBestEffort),
	}
	if err != nil || !util.Contains(enabled, string(setting)) {
		return 0
	}

	if v.Status.Robustness != longhorn.VolumeRobustnessHealthy {
		log.Warnf("Failed to auto-balance volume in %s state", v.Status.Robustness)
		return 0
	}

	var adjustCount int
	adjustCount, _, err = fnCount(v, e, rs)
	if err != nil {
		return 0
	}
	log.Debugf("Found %v replica candidate for auto-balance", adjustCount)
	return adjustCount
}

func (c *VolumeController) getReplicaCountForAutoBalanceBestEffort(v *longhorn.Volume, e *longhorn.Engine,
	rs map[string]*longhorn.Replica,
	fnCount replicaAutoBalanceCount) (int, []string, []string) {
	log := getLoggerForVolume(c.logger, v).WithField("replicaAutoBalanceOption", longhorn.ReplicaAutoBalanceBestEffort)

	var err error
	defer func() {
		if err != nil {
			log.WithError(err).Warn("Skip replica auto-balance")
		}
	}()

	setting, err := c.getAutoBalancedReplicasSetting(v)
	if err != nil || setting != longhorn.ReplicaAutoBalanceBestEffort {
		return 0, nil, []string{}
	}

	if v.Status.Robustness != longhorn.VolumeRobustnessHealthy {
		log.Warnf("Cannot auto-balance volume in %s state", v.Status.Robustness)
		return 0, nil, []string{}
	}

	var unusedCount int
	unusedCount, extraRNames, err := fnCount(v, e, rs)
	if unusedCount == 0 && len(extraRNames) <= 1 {
		return 0, nil, []string{}
	}

	var mostExtraRList []string
	var mostExtraRCount int
	var leastExtraROwners []string
	var leastExtraRCount int
	for owner, rNames := range extraRNames {
		rNameCount := len(rNames)
		if leastExtraRCount == 0 || rNameCount < leastExtraRCount {
			leastExtraRCount = rNameCount
			leastExtraROwners = []string{}
			leastExtraROwners = append(leastExtraROwners, owner)
		} else if rNameCount == leastExtraRCount {
			leastExtraROwners = append(leastExtraROwners, owner)
		}
		if rNameCount > mostExtraRCount {
			mostExtraRList = rNames
			mostExtraRCount = rNameCount
		}
	}

	if mostExtraRCount == 0 || mostExtraRCount == leastExtraRCount {
		return 0, nil, []string{}
	}

	adjustCount := mostExtraRCount - leastExtraRCount - 1
	log.Infof("Found %v replicas from %v to balance to one of node in %v", adjustCount, mostExtraRList, leastExtraROwners)
	return adjustCount, mostExtraRList, leastExtraROwners
}

func (c *VolumeController) getReplicaCountForAutoBalanceZone(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica) (int, map[string][]string, error) {
	log := getLoggerForVolume(c.logger, v).WithField("replicaAutoBalanceType", "zone")

	readyNodes, err := c.listReadySchedulableAndScheduledNodes(v, rs, log)
	if err != nil {
		return 0, nil, err
	}

	var usedZones []string
	var usedNodes []string
	zoneExtraRs := make(map[string][]string)
	// Count the engine node replica first so it doesn't get included in the
	// duplicates list.
	for _, r := range rs {
		node, exist := readyNodes[r.Spec.NodeID]
		if !exist {
			continue
		}
		if r.Spec.NodeID == e.Spec.NodeID {
			nZone := node.Status.Zone
			zoneExtraRs[nZone] = []string{}
			usedZones = append(usedZones, nZone)
			break
		}
	}
	for _, r := range rs {
		if r.Status.CurrentState != longhorn.InstanceStateRunning {
			continue
		}

		node, exist := readyNodes[r.Spec.NodeID]
		if !exist {
			// replica on node not count for auto-balance, could get evicted
			continue
		}

		if r.Spec.NodeID == e.Spec.NodeID {
			// replica on engine node not count for auto-balance
			continue
		}

		nZone := node.Status.Zone
		_, exist = zoneExtraRs[nZone]
		if exist {
			zoneExtraRs[nZone] = append(zoneExtraRs[nZone], r.Name)
		} else {
			zoneExtraRs[nZone] = []string{}
			usedZones = append(usedZones, nZone)
		}
		if !util.Contains(usedNodes, r.Spec.NodeID) {
			usedNodes = append(usedNodes, r.Spec.NodeID)
		}
	}
	log.Debugf("Found %v use zones %v", len(usedZones), usedZones)
	log.Debugf("Found %v use nodes %v", len(usedNodes), usedNodes)
	if v.Spec.NumberOfReplicas == len(zoneExtraRs) {
		log.Debugf("Balanced, %v volume replicas are running on different zones", v.Spec.NumberOfReplicas)
		return 0, zoneExtraRs, nil
	}

	ei, err := c.getEngineImage(v.Status.CurrentImage)
	if err != nil {
		return 0, nil, err
	}

	unusedZone := make(map[string][]string)
	for nodeName, node := range readyNodes {
		if util.Contains(usedZones, node.Status.Zone) {
			// cannot use node in zone because have running replica
			continue
		}

		if util.Contains(usedNodes, nodeName) {
			// cannot use node because have running replica
			continue
		}

		if !node.Spec.AllowScheduling {
			log.Warnf("Failed to use node %v, does not allow scheduling", nodeName)
			continue
		}

		if isReady, _ := c.ds.CheckEngineImageReadiness(ei.Spec.Image, nodeName); !isReady {
			log.Warnf("Failed to use node %v, engine image is not ready", nodeName)
			continue
		}

		unusedZone[node.Status.Zone] = append(unusedZone[node.Status.Zone], nodeName)
	}
	if len(unusedZone) == 0 {
		log.Debugf("Balanced, all ready zones are used by this volume")
		return 0, zoneExtraRs, err
	}

	unevenCount := v.Spec.NumberOfReplicas - len(zoneExtraRs)
	unusedCount := len(unusedZone)
	adjustCount := 0
	if unusedCount < unevenCount {
		adjustCount = unusedCount
	} else {
		adjustCount = unevenCount
	}
	log.Infof("Found %v zone available for auto-balance duplicates in %v", adjustCount, zoneExtraRs)

	return adjustCount, zoneExtraRs, err
}

func (c *VolumeController) listReadySchedulableAndScheduledNodes(volume *longhorn.Volume, rs map[string]*longhorn.Replica, log logrus.FieldLogger) (map[string]*longhorn.Node, error) {
	readyNodes, err := c.ds.ListReadyAndSchedulableNodes()
	if err != nil {
		return nil, err
	}

	filteredReadyNodes := readyNodes
	if len(volume.Spec.NodeSelector) != 0 {
		for nodeName, node := range readyNodes {
			if !types.IsSelectorsInTags(node.Spec.Tags, volume.Spec.NodeSelector) {
				delete(filteredReadyNodes, nodeName)
			}
		}
	}

	// Including unschedulable node because the replica is already scheduled and running
	// Ref: https://github.com/longhorn/longhorn/issues/4502
	for _, r := range rs {
		if r.Status.CurrentState != longhorn.InstanceStateRunning {
			continue
		}

		_, exist := filteredReadyNodes[r.Spec.NodeID]
		if exist {
			continue
		}

		node, err := c.ds.GetNodeRO(r.Spec.NodeID)
		if err != nil && !datastore.ErrorIsNotFound(err) {
			return nil, err
		}

		if node == nil {
			continue
		}

		nodeReadyCondition := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)
		if nodeReadyCondition.Status != longhorn.ConditionStatusTrue {
			continue
		}

		filteredReadyNodes[r.Spec.NodeID] = node
		log.WithFields(logrus.Fields{
			"replica": r.Name,
			"node":    node.Name,
		}).Warnf("Including unschedulable node because the replica is scheduled and running")
	}

	return filteredReadyNodes, nil
}

func (c *VolumeController) getReplicaCountForAutoBalanceNode(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica) (int, map[string][]string, error) {
	log := getLoggerForVolume(c.logger, v).WithField("replicaAutoBalanceType", "node")

	readyNodes, err := c.listReadySchedulableAndScheduledNodes(v, rs, log)
	if err != nil {
		return 0, nil, err
	}
	nodeExtraRs := make(map[string][]string)
	for _, r := range rs {
		if r.Status.CurrentState != longhorn.InstanceStateRunning {
			continue
		}

		_, exist := readyNodes[r.Spec.NodeID]
		if !exist {
			log.Warnf("Node %v is not ready or schedulable, replica %v could get evicted", r.Spec.NodeID, r.Name)
			continue
		}

		nodeID := r.Spec.NodeID
		_, isDuplicate := nodeExtraRs[nodeID]
		if isDuplicate {
			nodeExtraRs[nodeID] = append(nodeExtraRs[nodeID], r.Name)
		} else {
			nodeExtraRs[nodeID] = []string{}
		}

		if len(nodeExtraRs[nodeID]) > v.Spec.NumberOfReplicas {
			msg := fmt.Sprintf("Too many replicas running on node %v", nodeExtraRs[nodeID])
			log.WithField("nodeID", nodeID).Warn(msg)
			return 0, nil, nil
		}
	}

	if v.Spec.NumberOfReplicas == len(nodeExtraRs) {
		log.Debugf("Balanced, volume replicas are running on different nodes")
		return 0, nodeExtraRs, nil
	}

	ei, err := c.getEngineImage(v.Status.CurrentImage)
	if err != nil {
		return 0, nodeExtraRs, err
	}
	for nodeName, node := range readyNodes {
		_, exist := nodeExtraRs[nodeName]
		if exist {
			continue
		}

		if !node.Spec.AllowScheduling {
			log.Warnf("Failed to use node %v, does not allow scheduling", nodeName)
			delete(readyNodes, nodeName)
			continue
		}

		if isReady, _ := c.ds.CheckEngineImageReadiness(ei.Spec.Image, node.Name); !isReady {
			log.Warnf("Failed to use node %v, engine image is not ready", nodeName)
			delete(readyNodes, nodeName)
			continue
		}
	}
	if len(nodeExtraRs) == len(readyNodes) {
		log.Debugf("Balanced, all ready nodes are used by this volume")
		return 0, nodeExtraRs, nil
	}

	unevenCount := v.Spec.NumberOfReplicas - len(nodeExtraRs)
	unusedCount := len(readyNodes) - len(nodeExtraRs)
	adjustCount := 0
	if unusedCount < unevenCount {
		adjustCount = unusedCount
	} else {
		adjustCount = unevenCount
	}
	if adjustCount < 0 {
		adjustCount = 0
	}
	log.Infof("Found %v node available for auto-balance duplicates in %v", adjustCount, nodeExtraRs)

	return adjustCount, nodeExtraRs, err
}

func (c *VolumeController) getReplenishReplicasCount(v *longhorn.Volume, rs map[string]*longhorn.Replica, e *longhorn.Engine) (int, string) {
	usableCount := 0
	for _, r := range rs {
		// The failed to schedule local replica shouldn't be counted
		if isDataLocalityBestEffort(v) && r.Spec.HealthyAt == "" && r.Spec.FailedAt == "" && r.Spec.NodeID == "" &&
			v.Status.CurrentNodeID != "" && r.Spec.HardNodeAffinity == v.Status.CurrentNodeID {
			continue
		}
		// Skip the replica has been requested eviction.
		if r.Spec.FailedAt == "" && (!r.Status.EvictionRequested) && r.Spec.Active {
			usableCount++
		}
	}

	// Only create 1 replica while volume is in cloning process
	if isTargetVolumeOfCloning(v) {
		if usableCount == 0 {
			return 1, ""
		}
		return 0, ""
	}

	switch {
	case v.Spec.NumberOfReplicas < usableCount:
		return 0, ""
	case v.Spec.NumberOfReplicas > usableCount:
		return v.Spec.NumberOfReplicas - usableCount, ""
	case v.Spec.NumberOfReplicas == usableCount:
		if adjustCount := c.getReplicaCountForAutoBalanceLeastEffort(v, e, rs, c.getReplicaCountForAutoBalanceZone); adjustCount != 0 {
			return adjustCount, ""
		}
		if adjustCount := c.getReplicaCountForAutoBalanceLeastEffort(v, e, rs, c.getReplicaCountForAutoBalanceNode); adjustCount != 0 {
			return adjustCount, ""
		}
		adjustNodeAffinity := ""
		var nCandidates []string
		adjustCount, _, nCandidates := c.getReplicaCountForAutoBalanceBestEffort(v, e, rs, c.getReplicaCountForAutoBalanceNode)
		if adjustCount == 0 {
			adjustCount, _, zCandidates := c.getReplicaCountForAutoBalanceBestEffort(v, e, rs, c.getReplicaCountForAutoBalanceZone)
			if adjustCount != 0 {
				nCandidates = c.getNodeCandidatesForAutoBalanceZone(v, e, rs, zCandidates)
			}
		}
		if adjustCount != 0 {
			// TODO: remove checking and let schedular handle this part after
			// https://github.com/longhorn/longhorn/issues/2667
			schedulableCandidates := c.getIsSchedulableToDiskNodes(v, nCandidates)
			if len(schedulableCandidates) != 0 {
				// TODO: select replica auto-balance best-effort node from candidate list.
				// https://github.com/longhorn/longhorn/issues/2667
				adjustNodeAffinity = schedulableCandidates[0]
			}
			return adjustCount, adjustNodeAffinity
		}
		return adjustCount, adjustNodeAffinity
	}
	return 0, ""
}

func (c *VolumeController) getIsSchedulableToDiskNodes(v *longhorn.Volume, nodeNames []string) (schedulableNodeNames []string) {
	log := getLoggerForVolume(c.logger, v)
	defer func() {
		if len(schedulableNodeNames) == 0 {
			// TODO: record the message to condition
			log.Warn("Found 0 node has at least one schedulable disk")
		} else {
			log.Infof("Found node %v has at least one schedulable disk", schedulableNodeNames)
		}
	}()

	if len(nodeNames) == 0 {
		return schedulableNodeNames
	}

	for _, nodeName := range nodeNames {
		scheduleNode := false
		node, err := c.ds.GetNode(nodeName)
		if err != nil {
			continue
		}
		for diskName, diskStatus := range node.Status.DiskStatus {
			diskSpec, exists := node.Spec.Disks[diskName]
			if !exists {
				continue
			}

			if !diskSpec.AllowScheduling || diskSpec.EvictionRequested {
				continue
			}

			if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Status != longhorn.ConditionStatusTrue {
				continue
			}

			diskInfo, err := c.scheduler.GetDiskSchedulingInfo(diskSpec, diskStatus)
			if err != nil {
				continue
			}

			if c.scheduler.IsSchedulableToDisk(v.Spec.Size, v.Status.ActualSize, diskInfo) {
				scheduleNode = true
				break
			}
		}
		if scheduleNode {
			schedulableNodeNames = append(schedulableNodeNames, nodeName)
		}
	}
	return schedulableNodeNames
}

func (c *VolumeController) getNodeCandidatesForAutoBalanceZone(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica, zones []string) (candidateNames []string) {
	log := getLoggerForVolume(c.logger, v).WithFields(
		logrus.Fields{
			"replicaAutoBalanceOption": longhorn.ReplicaAutoBalanceBestEffort,
			"replicaAutoBalanceType":   "zone",
		},
	)

	var err error
	defer func() {
		if err != nil {
			log.WithError(err).Warn("Skip replica zone auto-balance")
		}
	}()

	if len(zones) == 0 {
		return candidateNames
	}

	if v.Status.Robustness != longhorn.VolumeRobustnessHealthy {
		log.Warnf("Failed to auto-balance volume in %s state", v.Status.Robustness)
		return candidateNames
	}

	readyNodes, err := c.ds.ListReadyAndSchedulableNodes()
	if err != nil {
		return candidateNames
	}

	ei, err := c.getEngineImage(v.Status.CurrentImage)
	if err != nil {
		return candidateNames
	}
	for nName, n := range readyNodes {
		for _, zone := range zones {
			if n.Status.Zone != zone {
				delete(readyNodes, nName)
				continue
			}
		}

		if !n.Spec.AllowScheduling {
			// cannot use node, does not allow scheduling.
			delete(readyNodes, nName)
			continue
		}

		if isReady, _ := c.ds.CheckEngineImageReadiness(ei.Spec.Image, nName); !isReady {
			// cannot use node, engine image is not ready
			delete(readyNodes, nName)
			continue
		}

		for _, r := range rs {
			if r.Spec.NodeID == nName {
				delete(readyNodes, nName)
				break
			}
		}
	}

	for nName := range readyNodes {
		candidateNames = append(candidateNames, nName)
	}
	if len(candidateNames) != 0 {
		log.Infof("Found node candidates: %v ", candidateNames)
	}
	return candidateNames
}

func (c *VolumeController) hasEngineStatusSynced(e *longhorn.Engine, rs map[string]*longhorn.Replica) bool {
	connectedReplicaCount := 0
	for _, r := range rs {
		if r.Spec.FailedAt == "" && r.Spec.NodeID != "" && r.Spec.Active {
			connectedReplicaCount++
		}
	}

	if len(e.Spec.ReplicaAddressMap) != connectedReplicaCount {
		return false
	}
	if len(e.Spec.ReplicaAddressMap) != len(e.Status.ReplicaModeMap) {
		return false
	}
	for rName := range e.Spec.ReplicaAddressMap {
		mode, exists := e.Status.ReplicaModeMap[rName]
		if !exists {
			return false
		}
		if mode == longhorn.ReplicaModeERR {
			return false
		}
	}
	return true
}

func (c *VolumeController) upgradeEngineForVolume(v *longhorn.Volume, es map[string]*longhorn.Engine, rs map[string]*longhorn.Replica) error {
	var err error

	if len(es) > 1 {
		return nil
	}

	e, err := c.ds.PickVolumeCurrentEngine(v, es)
	if err != nil {
		return err
	}
	if e == nil {
		return nil
	}

	log := getLoggerForVolume(c.logger, v).WithFields(logrus.Fields{
		"engine":                   e.Name,
		"volumeDesiredEngineImage": v.Spec.EngineImage,
	})

	if !c.isVolumeUpgrading(v) {
		// it must be a rollback
		if e.Spec.EngineImage != v.Spec.EngineImage {
			e.Spec.EngineImage = v.Spec.EngineImage
			e.Spec.UpgradedReplicaAddressMap = map[string]string{}
			return nil
		}
		// If the engine already has new engine image or it has been stopped,
		// live upgrade is not needed, there is no point to keep e.Spec.UpgradedReplicaAddressMap
		if e.Spec.EngineImage == e.Status.CurrentImage || e.Status.CurrentImage == "" {
			e.Spec.UpgradedReplicaAddressMap = map[string]string{}
		}
		return nil
	}

	// If volume is detached accidentally during the live upgrade,
	// the live upgrade info and the inactive replicas are meaningless.
	if v.Status.State == longhorn.VolumeStateDetached {
		if e.Spec.EngineImage != v.Spec.EngineImage {
			e.Spec.EngineImage = v.Spec.EngineImage
			e.Spec.UpgradedReplicaAddressMap = map[string]string{}
		}
		for _, r := range rs {
			if r.Spec.EngineImage != v.Spec.EngineImage {
				r.Spec.EngineImage = v.Spec.EngineImage
				rs[r.Name] = r
			}
			if !r.Spec.Active {
				log.Infof("Removing inactive replica %v when the volume is detached accidentally during the live upgrade", r.Name)
				if err := c.deleteReplica(r, rs); err != nil {
					return err
				}
			}
		}
		// TODO current replicas should be calculated by checking if there is
		// any other image exists except for the v.Spec.EngineImage
		v.Status.CurrentImage = v.Spec.EngineImage
		return nil
	}

	// only start live upgrade if volume is healthy
	if v.Status.State != longhorn.VolumeStateAttached || v.Status.Robustness != longhorn.VolumeRobustnessHealthy {
		return nil
	}

	volumeAndReplicaNodes := []string{v.Status.CurrentNodeID}
	for _, r := range rs {
		if r.Spec.NodeID == "" {
			continue
		}
		volumeAndReplicaNodes = append(volumeAndReplicaNodes, r.Spec.NodeID)
	}

	oldImage, err := c.getEngineImage(v.Status.CurrentImage)
	if err != nil {
		log.WithError(err).Warnf("Failed to get engine image %v for live upgrade", v.Status.CurrentImage)
		return nil
	}

	if isReady, err := c.ds.CheckEngineImageReadiness(oldImage.Spec.Image, volumeAndReplicaNodes...); !isReady {
		log.WithError(err).Warnf("Engine live upgrade from %v, but the image wasn't ready", oldImage.Spec.Image)
		return nil
	}
	newImage, err := c.getEngineImage(v.Spec.EngineImage)
	if err != nil {
		log.WithError(err).Warnf("Failed to get engine image %v for live upgrade", v.Spec.EngineImage)
		return nil
	}
	if isReady, err := c.ds.CheckEngineImageReadiness(newImage.Spec.Image, volumeAndReplicaNodes...); !isReady {
		log.WithError(err).Warnf("Engine live upgrade to %v, but the image wasn't ready", newImage.Spec.Image)
		return nil
	}

	if oldImage.Status.GitCommit == newImage.Status.GitCommit {
		log.Warnf("Engine image %v and %v are identical, delay upgrade until detach for volume", oldImage.Spec.Image, newImage.Spec.Image)
		return nil
	}

	if oldImage.Status.ControllerAPIVersion > newImage.Status.ControllerAPIVersion ||
		oldImage.Status.ControllerAPIVersion < newImage.Status.ControllerAPIMinVersion {
		log.Warnf("Failed to live upgrade from %v to %v: the old controller version %v "+
			"is not compatible with the new controller version %v and the new controller minimal version %v",
			oldImage.Spec.Image, newImage.Spec.Image,
			oldImage.Status.ControllerAPIVersion, newImage.Status.ControllerAPIVersion, newImage.Status.ControllerAPIMinVersion)
		return nil
	}

	unknownReplicas := map[string]*longhorn.Replica{}
	dataPathToOldRunningReplica := map[string]*longhorn.Replica{}
	dataPathToNewReplica := map[string]*longhorn.Replica{}
	for _, r := range rs {
		dataPath := types.GetReplicaDataPath(r.Spec.DiskPath, r.Spec.DataDirectoryName)
		if r.Spec.EngineImage == v.Status.CurrentImage && r.Status.CurrentState == longhorn.InstanceStateRunning && r.Spec.HealthyAt != "" {
			dataPathToOldRunningReplica[dataPath] = r
		} else if r.Spec.EngineImage == v.Spec.EngineImage {
			dataPathToNewReplica[dataPath] = r
		} else {
			log.Warnf("Found unknown replica with image %v for live upgrade", r.Spec.EngineImage)
			unknownReplicas[r.Name] = r
		}
	}

	// Skip checking and creating new replicas for the 2 cases:
	//   1. Volume is degraded.
	//   2. The new replicas is activated and all old replicas are already purged.
	if len(dataPathToOldRunningReplica) >= v.Spec.NumberOfReplicas {
		if err := c.createAndStartMatchingReplicas(v, rs, dataPathToOldRunningReplica, dataPathToNewReplica, func(r *longhorn.Replica, engineImage string) {
			r.Spec.EngineImage = engineImage
		}, v.Spec.EngineImage); err != nil {
			return err
		}
	}

	if e.Spec.EngineImage != v.Spec.EngineImage {
		replicaAddressMap := map[string]string{}
		for _, r := range dataPathToNewReplica {
			// wait for all potentially healthy replicas become running
			if r.Status.CurrentState != longhorn.InstanceStateRunning {
				return nil
			}
			if r.Status.IP == "" {
				log.WithField("replica", r.Name).Warn("replica is running but IP is empty")
				continue
			}
			if r.Status.StorageIP == "" {
				log.WithField("replica", r.Name).Warn("Replica is running but storage IP is empty, need to wait for update")
				continue
			}
			if r.Status.Port == 0 {
				log.WithField("replica", r.Name).Warn("Replica is running but port is 0")
				continue
			}
			replicaAddressMap[r.Name] = imutil.GetURL(r.Status.StorageIP, r.Status.Port)
		}
		// Only upgrade e.Spec.EngineImage if there are enough new upgraded replica.
		// This prevent the deadlock in the case that an upgrade from engine image
		// is followed immediately by an other upgrade.
		// More specifically, after the 1st upgrade, e.Status.ReplicaModeMap empty.
		// Therefore, dataPathToOldRunningReplica, dataPathToOldRunningReplica, and replicaAddressMap are also empty.
		// Now, if we set e.Spec.UpgradedReplicaAddressMap to an empty map in the second upgrade,
		// the second engine upgrade will be blocked since len(e.Spec.UpgradedReplicaAddressMap) == 0.
		// On the other hand, the engine controller blocks the engine's status from being refreshed
		// and keep the e.Status.ReplicaModeMap to be empty map. The system enter a deadlock for the volume.
		if len(replicaAddressMap) == v.Spec.NumberOfReplicas {
			e.Spec.UpgradedReplicaAddressMap = replicaAddressMap
			e.Spec.EngineImage = v.Spec.EngineImage
		}
	}
	if e.Status.CurrentImage != v.Spec.EngineImage ||
		e.Status.CurrentState != longhorn.InstanceStateRunning {
		return nil
	}

	c.switchActiveReplicas(rs, func(r *longhorn.Replica, engineImage string) bool {
		return r.Spec.EngineImage == engineImage && r.DeletionTimestamp.IsZero()
	}, v.Spec.EngineImage)

	e.Spec.ReplicaAddressMap = e.Spec.UpgradedReplicaAddressMap
	e.Spec.UpgradedReplicaAddressMap = map[string]string{}
	// cleanupCorruptedOrStaleReplicas() will take care of old replicas
	log.Infof("Engine %v has been upgraded from %v to %v", e.Name, v.Status.CurrentImage, v.Spec.EngineImage)
	v.Status.CurrentImage = v.Spec.EngineImage

	return nil
}

func (c *VolumeController) updateRequestedBackupForVolumeRestore(v *longhorn.Volume, e *longhorn.Engine) (err error) {
	if e == nil {
		return nil
	}

	if v.Spec.FromBackup == "" {
		return nil
	}

	// If the volume is not restoring, skip setting e.Spec.RequestedBackupRestore
	if !v.Status.RestoreRequired {
		return nil
	}

	// For DR volume, we set RequestedBackupRestore to the LastBackup
	if v.Status.IsStandby {
		if v.Status.LastBackup != "" && v.Status.LastBackup != e.Spec.RequestedBackupRestore {
			e.Spec.RequestedBackupRestore = v.Status.LastBackup
		}
		return nil
	}

	// For non-DR restoring volume, we set RequestedBackupRestore to the backup in v.Spec.FromBackup
	if _, backupName, err := c.getInfoFromBackupURL(v); err == nil && backupName != "" {
		e.Spec.RequestedBackupRestore = backupName
		return nil
	}

	return nil
}

func (c *VolumeController) checkAndInitVolumeOfflineReplicaRebuilding(v *longhorn.Volume, rs map[string]*longhorn.Replica) error {
	log := getLoggerForVolume(c.logger, v)

	if v.Spec.BackendStoreDriver != longhorn.BackendStoreDriverTypeV2 {
		return nil
	}

	switch v.Spec.OfflineReplicaRebuilding {
	case longhorn.OfflineReplicaRebuildingIgnored:
		offlineReplicaRebuilding, err := c.ds.GetSettingValueExisted(types.SettingNameOfflineReplicaRebuilding)
		if err != nil {
			log.WithError(err).Errorf("Failed to get setting %v", types.SettingNameOfflineReplicaRebuilding)
			return nil
		}
		if offlineReplicaRebuilding == string(longhorn.OfflineReplicaRebuildingDisabled) {
			return nil
		}
	case longhorn.OfflineReplicaRebuildingDisabled:
		return nil
	}

	if len(rs) == 0 {
		return nil
	}

	healthyReplicaCount := 0

	replicas, err := c.ds.ListVolumeReplicas(v.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get replicas for volume %v offline replica rebuilding", v.Name)
	}
	for _, r := range replicas {
		if r.Spec.HealthyAt != "" && r.Spec.FailedAt == "" {
			healthyReplicaCount++
		}
	}

	if healthyReplicaCount == 0 {
		return nil
	}

	if healthyReplicaCount >= v.Spec.NumberOfReplicas {
		return nil
	}

	if !v.Status.OfflineReplicaRebuildingRequired {
		log.Info("Requesting offline replica rebuilding for the volume")
	}

	v.Status.OfflineReplicaRebuildingRequired = true

	return nil
}

func (c *VolumeController) checkAndInitVolumeRestore(v *longhorn.Volume) error {
	log := getLoggerForVolume(c.logger, v)

	if v.Spec.FromBackup == "" || v.Status.RestoreInitiated {
		return nil
	}

	bName, _, _, err := backupstore.DecodeBackupURL(v.Spec.FromBackup)
	if err != nil {
		return fmt.Errorf("failed to get backup name from backup URL %v: %v", v.Spec.FromBackup, err)
	}

	backup, err := c.ds.GetBackupRO(bName)
	if err != nil {
		return fmt.Errorf("failed to inspect the backup config %v: %v", v.Spec.FromBackup, err)
	}

	size, err := util.ConvertSize(backup.Status.Size)
	if err != nil {
		return fmt.Errorf("failed to get the size of backup %v: %v", v.Spec.FromBackup, err)
	}
	v.Status.ActualSize = size

	// If KubernetesStatus is set on Backup, restore it.
	kubeStatus := &longhorn.KubernetesStatus{}
	if statusJSON, ok := backup.Status.Labels[types.KubernetesStatusLabel]; ok {
		if err := json.Unmarshal([]byte(statusJSON), kubeStatus); err != nil {
			log.WithError(err).Warnf("Ignore KubernetesStatus JSON for backup %v", backup.Name)
		} else {
			// We were able to unmarshal KubernetesStatus. Set the Ref fields.
			if kubeStatus.PVCName != "" && kubeStatus.LastPVCRefAt == "" {
				kubeStatus.LastPVCRefAt = backup.Status.SnapshotCreatedAt
			}
			if len(kubeStatus.WorkloadsStatus) != 0 && kubeStatus.LastPodRefAt == "" {
				kubeStatus.LastPodRefAt = backup.Status.SnapshotCreatedAt
			}

			// Do not restore the PersistentVolume fields.
			kubeStatus.PVName = ""
			kubeStatus.PVStatus = ""
		}
	}
	v.Status.KubernetesStatus = *kubeStatus

	if v.Spec.Standby {
		v.Status.IsStandby = true
	}

	v.Status.RestoreRequired = true
	v.Status.RestoreInitiated = true

	return nil
}

func (c *VolumeController) checkAndFinishVolumeRestore(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica) error {
	log := getLoggerForVolume(c.logger, v)

	if e == nil {
		return nil
	}
	// a restore/DR volume is considered as finished if the following conditions are satisfied:
	// 1) The restored backup is up-to-date;
	// 2) The volume is no longer a DR volume;
	// 3) The restore/DR volume is
	//   3.1) it's state `Healthy`;
	//   3.2) or it's state `Degraded` with all the scheduled replica included in the engine
	isPurging := false
	for _, status := range e.Status.PurgeStatus {
		if status.IsPurging {
			isPurging = true
			break
		}
	}
	if !(e.Spec.RequestedBackupRestore != "" && e.Spec.RequestedBackupRestore == e.Status.LastRestoredBackup &&
		!v.Spec.Standby) {
		return nil
	}

	allScheduledReplicasIncluded, err := c.checkAllScheduledReplicasIncluded(v, e, rs)
	if err != nil {
		return err
	}

	degradedVolumeSupported, err := c.ds.GetSettingAsBool(types.SettingNameAllowVolumeCreationWithDegradedAvailability)
	if err != nil {
		return err
	}

	if !isPurging && ((v.Status.Robustness == longhorn.VolumeRobustnessHealthy && allScheduledReplicasIncluded) || (v.Status.Robustness == longhorn.VolumeRobustnessDegraded && degradedVolumeSupported)) {
		log.Info("Restore/DR volume finished")
		v.Status.IsStandby = false
		v.Status.RestoreRequired = false
	}

	return nil
}

func (c *VolumeController) checkAllScheduledReplicasIncluded(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica) (bool, error) {
	healthReplicaCount := 0
	hasReplicaNotIncluded := false

	for _, r := range rs {
		// skip unscheduled replicas
		if r.Spec.NodeID == "" {
			continue
		}
		if isDownOrDeleted, err := c.ds.IsNodeDownOrDeleted(r.Spec.NodeID); err != nil {
			return false, err
		} else if isDownOrDeleted {
			continue
		}
		if mode := e.Status.ReplicaModeMap[r.Name]; mode == longhorn.ReplicaModeRW {
			healthReplicaCount++
		} else {
			hasReplicaNotIncluded = true
		}
	}

	return healthReplicaCount > 0 && (healthReplicaCount >= v.Spec.NumberOfReplicas || !hasReplicaNotIncluded), nil
}

func (c *VolumeController) updateRequestedDataSourceForVolumeCloning(v *longhorn.Volume, e *longhorn.Engine) (err error) {
	defer func() {
		err = errors.Wrap(err, "failed to updateRequestedDataSourceForVolumeCloning")
	}()

	if e == nil {
		return nil
	}
	if isTargetVolumeOfCloning(v) && v.Status.CloneStatus.State == longhorn.VolumeCloneStateInitiated {
		ds, err := types.NewVolumeDataSource(longhorn.VolumeDataSourceTypeSnapshot, map[string]string{
			types.VolumeNameKey:   v.Status.CloneStatus.SourceVolume,
			types.SnapshotNameKey: v.Status.CloneStatus.Snapshot,
		})
		if err != nil {
			return err
		}
		e.Spec.RequestedDataSource = ds
		return nil
	}
	e.Spec.RequestedDataSource = ""
	return nil
}

func (c *VolumeController) checkAndInitVolumeClone(v *longhorn.Volume) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to checkAndInitVolumeClone for volume %v", v.Name)
	}()

	dataSource := v.Spec.DataSource
	if !types.IsDataFromVolume(dataSource) || v.Status.CloneStatus.State != longhorn.VolumeCloneStateEmpty {
		return nil
	}

	sourceVolName := types.GetVolumeName(dataSource)
	snapshotName := types.GetSnapshotName(dataSource)
	sourceVol, err := c.ds.GetVolume(sourceVolName)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			v.Status.CloneStatus.State = longhorn.VolumeCloneStateFailed
			c.eventRecorder.Eventf(v, corev1.EventTypeWarning, constant.EventReasonVolumeCloneFailed, "cannot find the source volume %v", sourceVolName)
			return nil
		}
		return err
	}
	// Wait for the source volume to be attach
	// TODO: do we need to check the status of volume-clone AD ticket ???
	if sourceVol.Status.State != longhorn.VolumeStateAttached {
		return nil
	}

	e, err := c.ds.GetVolumeCurrentEngine(sourceVolName)
	if err != nil {
		return err
	}

	if snapshotName == "" {
		// Use a deterministic UUID for snapshotName in case this reconciliation fails and we hit this code block again
		// in the next reconciliation. We don't want to generate multiple snapshots. Create the UUID by hashing the UIDs
		// of the source and destination volume to avoid problems with reused volume names.
		snapshotName = util.DeterministicUUID(string(sourceVol.GetUID()) + string(v.GetUID()))
		labels := map[string]string{types.GetLonghornLabelKey(types.LonghornLabelSnapshotForCloningVolume): v.Name}
		snapshot, err := c.createSnapshot(snapshotName, labels, sourceVol, e)
		if err != nil {
			return errors.Wrapf(err, "failed to create snapshot of source volume %v", sourceVol.Name)
		}
		snapshotName = snapshot.Name
	}

	// Store data into the volume clone status. Make sure that the created snapshot
	// persit in the volume spec before continue
	v.Status.CloneStatus.SourceVolume = sourceVolName
	v.Status.CloneStatus.Snapshot = snapshotName
	v.Status.CloneStatus.State = longhorn.VolumeCloneStateInitiated
	c.eventRecorder.Eventf(v, corev1.EventTypeNormal, constant.EventReasonVolumeCloneInitiated, "source volume %v, snapshot %v", sourceVolName, snapshotName)

	return nil
}

func (c *VolumeController) checkForAutoAttachment(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica, scheduled bool) error {
	if v.Spec.NodeID != "" || v.Status.CurrentNodeID != "" {
		return nil
	}
	if !(v.Status.State == "" || v.Status.State == longhorn.VolumeStateDetached) {
		return nil
	}
	// Do not intervene the auto reattachment workflow during the engine crashing and volume recovery.
	if v.Status.PendingNodeID != "" {
		return nil
	}
	// It's meaningless to do auto attachment if the volume scheduling fails
	if !scheduled {
		return nil
	}

	exportingBackingImageDataSources, err := c.ds.ListBackingImageDataSourcesExportingFromVolume(v.Name)
	if err != nil {
		return err
	}

	// Do auto attachment for:
	//   1. restoring/DR volumes.
	//   2. expansion.
	//   3. Eviction requested on this volume.
	//   4. The target volume of a cloning
	//   5. The source volume of a cloning
	//   6, Export data as a backing image
	isRestoringDRVol := v.Status.RestoreRequired || v.Status.IsStandby
	isExpansionVol := v.Status.ExpansionRequired
	isEvictionRequestedOnVol := hasReplicaEvictionRequested(rs)
	isTargetVolOfCloning := isTargetVolumeOfCloning(v)
	sourceVolumeOfCloning, err := c.isSourceVolumeOfCloning(v)
	isExportingBackingImage := len(exportingBackingImageDataSources) != 0
	if err != nil {
		return err
	}
	if isRestoringDRVol || isExpansionVol || isEvictionRequestedOnVol ||
		isTargetVolOfCloning || sourceVolumeOfCloning || isExportingBackingImage {
		// Should use c.controllerID or v.Status.OwnerID as CurrentNodeID,
		// otherwise they may be not equal
		v.Status.CurrentNodeID = v.Status.OwnerID
	}

	return nil
}

// isSourceVolumeOfCloning checks if the input volume is the source volume of an on-going cloning process
func (c *VolumeController) isSourceVolumeOfCloning(v *longhorn.Volume) (bool, error) {
	vols, err := c.ds.ListVolumes()
	if err != nil {
		return false, err
	}
	for _, vol := range vols {
		if isTargetVolumeOfCloning(vol) && types.GetVolumeName(vol.Spec.DataSource) == v.Name {
			return true, nil
		}
	}
	return false, nil
}

func (c *VolumeController) checkForAutoDetachment(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica) error {
	log := getLoggerForVolume(c.logger, v)

	if v.Spec.NodeID != "" || v.Status.CurrentNodeID == "" || e == nil {
		return nil
	}

	if v.Status.ExpansionRequired {
		_, err := c.ds.GetNodeRO(v.Status.CurrentNodeID)
		if err == nil || (err != nil && !datastore.ErrorIsNotFound(err)) {
			return nil
		}

		log.Infof("Preparing to do auto-detachment of expanding volume %v, because the node %v is unavailable", v.Name, v.Status.CurrentNodeID)
		v.Status.CurrentNodeID = ""
		return nil
	}

	// Don't do auto-detachment if the eviction is going on.
	if hasReplicaEvictionRequested(rs) {
		return nil
	}

	// While cloning is happening, don't auto-detach target volume or source volume of the cloning
	if isTargetVolumeOfCloning(v) {
		return nil
	}
	if isSourceVolOfCloning, err := c.isSourceVolumeOfCloning(v); err != nil {
		return err
	} else if isSourceVolOfCloning {
		return nil
	}

	// This volume is being exported as a backing image.
	exportingBackingImageDataSources, err := c.ds.ListBackingImageDataSourcesExportingFromVolume(v.Name)
	if err != nil {
		return err
	}
	if len(exportingBackingImageDataSources) != 0 {
		return nil
	}

	// Do auto-detachment for non-restore/DR volumes.
	if !v.Status.RestoreRequired && !v.Status.IsStandby {
		v.Status.CurrentNodeID = ""
		return nil
	}

	// Do auto-detachment for restore/DR volumes.
	if v.Status.CurrentNodeID != v.Status.OwnerID {
		log.Info("Found the restore/DR volume node is down, will detach it first then re-attach it to restart the restoring")
		v.Status.CurrentNodeID = ""
		return nil
	}
	// Can automatically detach/activate the restore/DR volume on the running node if the following conditions are satisfied:
	// 1) The restored backup is up-to-date;
	// 2) The volume is no longer a DR volume;
	// 3) The restore/DR volume is
	//   3.1) using the old engine image. And it's still running.
	//	 3.2) or using the latest engine image without purging snapshots. And
	//	   3.2.1) it's state `Healthy`;
	//	   3.2.2) or it's state `Degraded` with all the scheduled replica included in the engine
	cliAPIVersion, err := c.ds.GetEngineImageCLIAPIVersion(v.Status.CurrentImage)
	if err != nil {
		return err
	}
	isPurging := false
	for _, status := range e.Status.PurgeStatus {
		if status.IsPurging {
			isPurging = true
			break
		}
	}

	// After the volume is detached, the engine stops and does not perform recovery,
	// so it should ensure that the backup volume is synced and updated at least once
	// so that the engine restores with the latest backup before the volume is detached
	if backupVolumeName, isExist := v.Labels[types.LonghornLabelBackupVolume]; isExist && backupVolumeName != "" {
		backupVolume, err := c.ds.GetBackupVolumeRO(backupVolumeName)
		if err != nil && !datastore.ErrorIsNotFound(err) {
			return errors.Wrapf(err, "failed to get backup volume: %v", v.Name)
		}
		if backupVolume != nil && backupVolume.Status.LastSyncedAt.Before(&backupVolume.Spec.SyncRequestedAt) {
			return nil
		}
	}

	// make sure engine finish restoring with the latest backup and no longer a DR volume
	if !(e.Spec.RequestedBackupRestore != "" &&
		e.Spec.RequestedBackupRestore == e.Status.LastRestoredBackup &&
		!v.Spec.Standby) {
		return nil
	}

	allScheduledReplicasIncluded, err := c.checkAllScheduledReplicasIncluded(v, e, rs)
	if err != nil {
		return err
	}

	degradedVolumeSupported, err := c.ds.GetSettingAsBool(types.SettingNameAllowVolumeCreationWithDegradedAvailability)
	if err != nil {
		return err
	}

	if (cliAPIVersion >= engineapi.CLIVersionFour && !isPurging && ((v.Status.Robustness == longhorn.VolumeRobustnessHealthy && allScheduledReplicasIncluded) || (v.Status.Robustness == longhorn.VolumeRobustnessDegraded && degradedVolumeSupported))) ||
		(cliAPIVersion < engineapi.CLIVersionFour && (v.Status.Robustness == longhorn.VolumeRobustnessHealthy || v.Status.Robustness == longhorn.VolumeRobustnessDegraded)) {
		log.Info("Preparing to do auto detachment for restore/DR volume")
		v.Status.CurrentNodeID = ""
		v.Status.IsStandby = false
		v.Status.RestoreRequired = false
	}

	return nil
}

func (c *VolumeController) getInfoFromBackupURL(v *longhorn.Volume) (string, string, error) {
	if v.Spec.FromBackup == "" {
		return "", "", nil
	}

	backupName, backupVolumeName, _, err := backupstore.DecodeBackupURL(v.Spec.FromBackup)
	return backupVolumeName, backupName, err
}

func (c *VolumeController) createEngine(v *longhorn.Volume, currentEngineName string) (*longhorn.Engine, error) {
	log := getLoggerForVolume(c.logger, v)

	engine := &longhorn.Engine{
		ObjectMeta: metav1.ObjectMeta{
			Name:            types.GenerateEngineNameForVolume(v.Name, currentEngineName),
			OwnerReferences: datastore.GetOwnerReferencesForVolume(v),
		},
		Spec: longhorn.EngineSpec{
			InstanceSpec: longhorn.InstanceSpec{
				VolumeName:         v.Name,
				VolumeSize:         v.Spec.Size,
				EngineImage:        v.Status.CurrentImage,
				BackendStoreDriver: v.Spec.BackendStoreDriver,
				DesireState:        longhorn.InstanceStateStopped,
			},
			Frontend:                  v.Spec.Frontend,
			ReplicaAddressMap:         map[string]string{},
			UpgradedReplicaAddressMap: map[string]string{},
			RevisionCounterDisabled:   v.Spec.RevisionCounterDisabled,
		},
	}

	if v.Spec.FromBackup != "" && v.Status.RestoreRequired {
		backupVolumeName, backupName, err := c.getInfoFromBackupURL(v)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get backup volume when creating engine object of restored volume %v", v.Name)
		}
		engine.Spec.BackupVolume = backupVolumeName
		engine.Spec.RequestedBackupRestore = backupName

		log.Infof("Creating engine %v for restored volume, BackupVolume is %v, RequestedBackupRestore is %v",
			engine.Name, engine.Spec.BackupVolume, engine.Spec.RequestedBackupRestore)
	}

	unmapMarkEnabled, err := c.isUnmapMarkSnapChainRemovedEnabled(v)
	if err != nil {
		return nil, err
	}
	engine.Spec.UnmapMarkSnapChainRemovedEnabled = unmapMarkEnabled

	if currentEngineName == "" {
		engine.Spec.Active = true
	}

	return c.ds.CreateEngine(engine)
}

func (c *VolumeController) createReplica(v *longhorn.Volume, e *longhorn.Engine, rs map[string]*longhorn.Replica,
	hardNodeAffinity string, isRebuildingReplica bool) error {
	log := getLoggerForVolume(c.logger, v)

	replica := &longhorn.Replica{
		ObjectMeta: metav1.ObjectMeta{
			Name:            types.GenerateReplicaNameForVolume(v.Name),
			OwnerReferences: datastore.GetOwnerReferencesForVolume(v),
		},
		Spec: longhorn.ReplicaSpec{
			InstanceSpec: longhorn.InstanceSpec{
				VolumeName:         v.Name,
				VolumeSize:         v.Spec.Size,
				EngineImage:        v.Status.CurrentImage,
				BackendStoreDriver: v.Spec.BackendStoreDriver,
				DesireState:        longhorn.InstanceStateStopped,
			},
			EngineName:                       e.Name,
			Active:                           true,
			BackingImage:                     v.Spec.BackingImage,
			HardNodeAffinity:                 hardNodeAffinity,
			RevisionCounterDisabled:          v.Spec.RevisionCounterDisabled,
			UnmapMarkDiskChainRemovedEnabled: e.Spec.UnmapMarkSnapChainRemovedEnabled,
		},
	}
	if isRebuildingReplica {
		// TODO: reuse failed replica for replica rebuilding of SPDK volumes
		if v.Spec.BackendStoreDriver == longhorn.BackendStoreDriverTypeV2 {
			if !v.Spec.DisableFrontend || !v.Status.OfflineReplicaRebuildingRequired {
				log.Tracef("Online replica rebuilding for replica %v is not supported for SPDK volumes", replica.Name)
				return nil
			}
		}

		log.Infof("A new replica %v will be replenished during rebuilding", replica.Name)
		// Prevent this new replica from being reused after rebuilding failure.
		replica.Spec.RebuildRetryCount = scheduler.FailedReplicaMaxRetryCount
	}

	replica, err := c.ds.CreateReplica(replica)
	if err != nil {
		return err
	}
	rs[replica.Name] = replica

	return nil
}

func (c *VolumeController) duplicateReplica(r *longhorn.Replica, v *longhorn.Volume) *longhorn.Replica {
	replica := &longhorn.Replica{
		ObjectMeta: metav1.ObjectMeta{
			Name:            types.GenerateReplicaNameForVolume(r.Spec.VolumeName),
			OwnerReferences: datastore.GetOwnerReferencesForVolume(v),
		},
		Spec: r.DeepCopy().Spec,
	}
	return replica
}

func (c *VolumeController) enqueueVolume(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to get key for object %#v: %v", obj, err))
		return
	}

	c.queue.Add(key)
}

func (c *VolumeController) enqueueVolumeAfter(obj interface{}, duration time.Duration) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("enqueueVolumeAfter: failed to get key for object %#v: %v", obj, err))
		return
	}

	c.queue.AddAfter(key, duration)
}

func (c *VolumeController) enqueueControlleeChange(obj interface{}) {
	if deletedState, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = deletedState.Obj
	}

	metaObj, err := meta.Accessor(obj)
	if err != nil {
		c.logger.WithError(err).Warnf("failed to convert obj %v to metav1.Object", obj)
		return
	}
	ownerRefs := metaObj.GetOwnerReferences()
	for _, ref := range ownerRefs {
		namespace := metaObj.GetNamespace()
		c.ResolveRefAndEnqueue(namespace, &ref)
		return
	}
}

func (c *VolumeController) ResolveRefAndEnqueue(namespace string, ref *metav1.OwnerReference) {
	if ref.Kind != types.LonghornKindVolume {
		// TODO: Will stop checking this wrong reference kind after all Longhorn components having used the new kinds
		if ref.Kind != ownerKindVolume {
			return
		}
	}
	volume, err := c.ds.GetVolume(ref.Name)
	if err != nil {
		return
	}
	if volume.UID != ref.UID {
		// The controller we found with this Name is not the same one that the
		// OwnerRef points to.
		return
	}
	c.enqueueVolume(volume)
}

// generateRecurringJobName uses a new name to create a new recurring job that original name exists but configuration is different
// or find a recreated recurring job name that had been restored and configuration is the same
func generateRecurringJobName(log *logrus.Entry, ds *datastore.DataStore, job *longhorn.VolumeRecurringJobInfo) (string, string, error) {
	// reuse restored recurring job that RecurringJobSpec is the same.
	allRecurringJobs, err := ds.ListRecurringJobsRO()
	if err != nil {
		log.WithError(err).Warn("Failed to list all recurring jobs")
		return "", "", err
	}
	for existJobName, existJob := range allRecurringJobs {
		if !strings.HasPrefix(existJobName, types.VolumeRecurringJobRestorePrefix) {
			continue
		}
		job.JobSpec.Name = existJob.Spec.Name
		if reflect.DeepEqual(existJob.Spec, job.JobSpec) {
			return "", existJobName, nil
		}
	}

	// generateJobName returns a name that contains prefix and 16 random characters
	generateJobName := func(prefix string) string {
		return prefix + util.RandomID() + util.RandomID()
	}

	newJobName := generateJobName(types.VolumeRecurringJobRestorePrefix)
	job.JobSpec.Name = newJobName

	return newJobName, "", nil
}

// restoreVolumeRecurringJobLabel label restored recurring jobs/groups from the backup target when doing a backup restoration
func restoreVolumeRecurringJobLabel(v *longhorn.Volume, jobName string, job *longhorn.VolumeRecurringJobInfo) error {
	// set Volume recurring jobs/groups
	if job.FromJob {
		labelKey := types.GetRecurringJobLabelKey(types.LonghornLabelRecurringJob, jobName)
		// enable recurring jobs
		v.Labels[labelKey] = types.LonghornLabelValueEnabled
	}
	for _, groupName := range job.FromGroup {
		groupLabelKey := types.GetRecurringJobLabelKey(types.LonghornLabelRecurringJobGroup, groupName)
		// enable recurring groups
		v.Labels[groupLabelKey] = types.LonghornLabelValueEnabled
	}

	return nil
}

// createRecurringJobNotExist add recurring jobs and group back to a restoring volume and create recurring jobs if not exist
func createRecurringJobNotExist(log *logrus.Entry, ds *datastore.DataStore, v *longhorn.Volume, jobName string, job *longhorn.VolumeRecurringJobInfo) (string, error) {
	if existJob, err := ds.GetRecurringJob(jobName); err != nil {
		if !apierrors.IsNotFound(err) {
			return "", errors.Wrapf(err, "failed to get the recurring job %v", jobName)
		}
	} else if !reflect.DeepEqual(existJob.Spec, job.JobSpec) {
		// create a new recurring job if job name is used and configuration is different.
		newJobName, existJobName, err := generateRecurringJobName(log, ds, job)
		if err != nil {
			return "", errors.Wrapf(err, "failed to create a new recurring job %v", jobName)
		}
		if existJobName != "" {
			return existJobName, nil
		}
		jobName = newJobName
	} else {
		return jobName, nil
	}

	// create the recurring job if not exist
	log.Infof("Creating the recurring job %v when restoring recurring jobs", jobName)
	if _, err := ds.CreateRecurringJob(&longhorn.RecurringJob{ObjectMeta: metav1.ObjectMeta{Name: jobName}, Spec: job.JobSpec}); err != nil {
		return "", errors.Wrapf(err, "failed to create a recurring job %v", jobName)
	}

	return jobName, nil
}

// restoreVolumeRecurringJobs create recurring jobs/groups from the backup volume when restoring from a backup, except for the DR volume
func (c *VolumeController) restoreVolumeRecurringJobs(v *longhorn.Volume) error {
	log := getLoggerForVolume(c.logger, v)

	backupVolumeRecurringJobsInfo := make(map[string]longhorn.VolumeRecurringJobInfo)
	bvName, exist := v.Labels[types.LonghornLabelBackupVolume]
	if !exist {
		log.Warn("Failed to find the backup volume label")
		return nil
	}

	bv, err := c.ds.GetBackupVolumeRO(bvName)
	if err != nil {
		return errors.Wrapf(err, "failed to get the backup volume %v info", bvName)
	}

	volumeRecurringJobInfoStr, exist := bv.Status.Labels[types.VolumeRecurringJobInfoLabel]
	if !exist {
		return nil
	}

	if err := json.Unmarshal([]byte(volumeRecurringJobInfoStr), &backupVolumeRecurringJobsInfo); err != nil {
		return errors.Wrapf(err, "failed to unmarshal information of volume recurring jobs, backup volume %v", bvName)
	}

	for jobName, job := range backupVolumeRecurringJobsInfo {
		if job.JobSpec.Groups == nil {
			job.JobSpec.Groups = []string{}
		}
		if job.JobSpec.Labels == nil {
			job.JobSpec.Labels = map[string]string{}
		}
		if jobName, err = createRecurringJobNotExist(log, c.ds, v, jobName, &job); err != nil {
			return err
		}
		restoreVolumeRecurringJobLabel(v, jobName, &job)
	}

	return nil
}

// shouldRestoreRecurringJobs check if it needs to restore recurring jobs/groups before a backup restoration
func (c *VolumeController) shouldRestoreRecurringJobs(v *longhorn.Volume) bool {
	if v.Spec.FromBackup == "" || v.Spec.Standby || v.Status.RestoreInitiated {
		return false
	}

	if v.Spec.RestoreVolumeRecurringJob == longhorn.RestoreVolumeRecurringJobEnabled {
		return true
	}

	if v.Spec.RestoreVolumeRecurringJob == longhorn.RestoreVolumeRecurringJobDisabled {
		return false
	}

	// Avoid recurring job restoration overrides the new recurring jobs added by users.
	existingJobs := datastore.MarshalLabelToVolumeRecurringJob(v.Labels)
	for jobName, job := range existingJobs {
		if !job.IsGroup || jobName != longhorn.RecurringJobGroupDefault {
			c.logger.Warn("User already specified recurring jobs for this volume, cannot continue restoring recurring jobs from the backup volume labels")
			return false
		}
	}

	restoringRecurringJobs, err := c.ds.GetSettingAsBool(types.SettingNameRestoreVolumeRecurringJobs)
	if err != nil {
		c.logger.WithError(err).Warnf("Failed to get %v setting", types.SettingNameRestoreVolumeRecurringJobs)
	}

	return restoringRecurringJobs
}

func (c *VolumeController) updateRecurringJobs(v *longhorn.Volume) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to update recurring jobs for %v", v.Name)
	}()

	existingVolume := v.DeepCopy()

	if err := c.syncPVCRecurringJobLabels(v); err != nil {
		return errors.Wrapf(err, "failed attempting to sync PVC recurring job labels from Volume %v", v.Name)
	}

	if err = datastore.FixupRecurringJob(v); err != nil {
		return err
	}

	// DR volume will not restore volume recurring jobs/groups configuration.
	if c.shouldRestoreRecurringJobs(v) {
		if err := c.restoreVolumeRecurringJobs(v); err != nil {
			c.logger.WithError(err).Warn("Failed to restore the volume recurring jobs/groups")
		}
	}

	if err == nil && !reflect.DeepEqual(existingVolume.Labels, v.Labels) {
		_, err = c.ds.UpdateVolume(v)
	}

	return nil
}

func (c *VolumeController) syncPVCRecurringJobLabels(volume *longhorn.Volume) error {
	kubeStatus := volume.Status.KubernetesStatus
	if kubeStatus.PVCName == "" || kubeStatus.LastPVCRefAt != "" {
		return nil
	}

	pvc, err := c.ds.GetPersistentVolumeClaim(kubeStatus.Namespace, kubeStatus.PVCName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	hasSourceLabel, err := hasRecurringJobSourceLabel(pvc)
	if err != nil {
		return errors.Wrapf(err, "failed to check recurring job source")
	}

	if !hasSourceLabel {
		c.logger.Debugf("Ignoring recurring job labels on Volume %v PVC %v due to missing source label", volume.Name, pvc.Name)

		return nil
	}

	if err := syncRecurringJobLabelsToTargetResource(types.LonghornKindVolume, volume, pvc, c.logger); err != nil {
		return errors.Wrapf(err, "failed to sync recurring job labels from PVC %v to Volume %v", pvc.Name, volume.Name)
	}
	return nil
}

func (c *VolumeController) getEngineImage(image string) (*longhorn.EngineImage, error) {
	name := types.GetEngineImageChecksumName(image)
	img, err := c.ds.GetEngineImage(name)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get engine image %v", image)
	}
	return img, nil
}

func (c *VolumeController) getCurrentEngineAndCleanupOthers(v *longhorn.Volume, es map[string]*longhorn.Engine) (current *longhorn.Engine, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to clean up the extra engines for %v", v.Name)
	}()
	current, extras, err := datastore.GetCurrentEngineAndExtras(v, es)
	if err != nil {
		return nil, err
	}

	for _, extra := range extras {
		if extra.DeletionTimestamp == nil {
			if err := c.deleteEngine(extra, es); err != nil {
				return nil, err
			}
		}
	}
	return current, nil
}

func (c *VolumeController) createAndStartMatchingReplicas(v *longhorn.Volume,
	rs, pathToOldRs, pathToNewRs map[string]*longhorn.Replica,
	fixupFunc func(r *longhorn.Replica, obj string), obj string) error {
	log := getLoggerForVolume(c.logger, v)
	for path, r := range pathToOldRs {
		if pathToNewRs[path] != nil {
			continue
		}
		clone := c.duplicateReplica(r, v)
		clone.Spec.DesireState = longhorn.InstanceStateRunning
		clone.Spec.Active = false
		fixupFunc(clone, obj)
		newReplica, err := c.ds.CreateReplica(clone)
		if err != nil {
			return errors.Wrapf(err, "failed to create matching replica %v for volume %v", clone.Name, v.Name)
		}
		log.Infof("Cloned a new matching replica %v from %v", newReplica.Name, r.Name)
		pathToNewRs[path] = newReplica
		rs[newReplica.Name] = newReplica
	}
	return nil
}

func (c *VolumeController) deleteInvalidMigrationReplicas(rs, pathToOldRs, pathToNewRs map[string]*longhorn.Replica) error {
	for path, r := range pathToNewRs {
		matchTheOldReplica := pathToOldRs[path] != nil && r.Spec.DesireState == pathToOldRs[path].Spec.DesireState
		newReplicaIsAvailable := r.DeletionTimestamp == nil && r.Spec.DesireState == longhorn.InstanceStateRunning &&
			r.Status.CurrentState != longhorn.InstanceStateError && r.Status.CurrentState != longhorn.InstanceStateUnknown && r.Status.CurrentState != longhorn.InstanceStateStopping
		if matchTheOldReplica && newReplicaIsAvailable {
			continue
		}
		delete(pathToNewRs, path)
		if err := c.deleteReplica(r, rs); err != nil {
			return errors.Wrapf(err, "failed to delete the new replica %v when there is no matching old replica in path %v", r.Name, path)
		}
	}
	return nil
}

func (c *VolumeController) switchActiveReplicas(rs map[string]*longhorn.Replica,
	activeCondFunc func(r *longhorn.Replica, obj string) bool, obj string) {

	// Deletion of an active replica will trigger the cleanup process to
	// delete the volume data on the disk.
	// Set `active` at last to prevent data loss
	for _, r := range rs {
		if r.Spec.Active != activeCondFunc(r, obj) {
			logrus.Infof("Switching replica %v active state from %v to %v due to %v", r.Name, r.Spec.Active, !r.Spec.Active, obj)
			r.Spec.Active = !r.Spec.Active
		}
	}
}

func (c *VolumeController) processMigration(v *longhorn.Volume, es map[string]*longhorn.Engine, rs map[string]*longhorn.Replica) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to process migration for %v", v.Name)
	}()

	if !isMigratableVolume(v) {
		return nil
	}

	log := getLoggerForVolume(c.logger, v).WithField("migrationNodeID", v.Spec.MigrationNodeID)

	// only process if volume is attached and running
	if v.Spec.NodeID == "" || v.Status.CurrentNodeID == "" || len(es) == 0 {
		return nil
	}
	if v.Status.Robustness != longhorn.VolumeRobustnessDegraded && v.Status.Robustness != longhorn.VolumeRobustnessHealthy {
		log.Warnf("Skip the migration processing since the volume current robustness is %v", v.Status.Robustness)
		return nil
	}

	// cannot process migrate when upgrading
	if c.isVolumeUpgrading(v) {
		log.Warn("Skip the migration processing since the volume is being upgraded")
		return nil
	}

	// the only time there should be more then 1 engines is when we are migrating or upgrading
	// if there are more then 1 and we no longer have a migration id set we can cleanup the extra engine
	if v.Spec.MigrationNodeID == "" {
		if len(es) < 2 && v.Status.CurrentMigrationNodeID == "" {
			return nil
		}

		// in the case of a confirmation we need to switch the v.Status.CurrentNodeID to v.Spec.NodeID
		// so that currentEngine becomes the migration engine
		if v.Status.CurrentNodeID != v.Spec.NodeID {
			log.Infof("volume migration complete switching current node id from %v to %v", v.Status.CurrentNodeID, v.Spec.NodeID)
			v.Status.CurrentNodeID = v.Spec.NodeID
		}

		// The latest current engine is based on the multiple node related fields of the volume basides all engine desired node ID,
		// If the new engine matches, it means a migration confirmation then it's time to remove the old engine.
		// If the old engine matches, it means a migration rollback hence cleaning up the migration engine is required.
		//
		// But once the volume is in the mid of the auto-recovery flow during migration rollback, there may be no engine considered as a current engine.
		// Then all we have to do is waiting for the old engine back and running first. (Corner case)
		currentEngine, extras, err := datastore.GetNewCurrentEngineAndExtras(v, es)
		if err != nil {
			log.WithError(err).Warn("Failed to finish the migration confirmation or rollback, need to wait for the current engine becoming attached first")
			return nil
		}
		// The cleanup can be done only after the new current engine is found.
		for i := range extras {
			e := extras[i]
			if e.DeletionTimestamp == nil {
				if err := c.deleteEngine(e, es); err != nil {
					return err
				}
				log.Infof("Removing extra engine %v after switching the current engine to %v", e.Name, currentEngine.Name)
			}
		}

		currentEngine.Spec.Active = true

		// cleanupCorruptedOrStaleReplicas() will take care of old replicas
		c.switchActiveReplicas(rs, func(r *longhorn.Replica, engineName string) bool {
			return r.Spec.EngineName == engineName && r.Spec.HealthyAt != ""
		}, currentEngine.Name)

		// migration rollback or confirmation finished
		v.Status.CurrentMigrationNodeID = ""

		return nil
	}

	// init the volume migration
	if v.Status.CurrentMigrationNodeID == "" {
		v.Status.CurrentMigrationNodeID = v.Spec.MigrationNodeID
	}

	revertRequired := false
	defer func() {
		if !revertRequired {
			return
		}

		log.Warnf("The migration engine or all migration replicas crashed, will clean them up now")

		currentEngine, err := c.getCurrentEngineAndCleanupOthers(v, es)
		if err != nil {
			err = errors.Wrap(err, "failed to get the current engine and clean up others during the migration revert")
			return
		}

		for _, r := range rs {
			if r.Spec.EngineName == currentEngine.Name {
				continue
			}
			if err := c.deleteReplica(r, rs); err != nil {
				err = errors.Wrapf(err, "failed to delete the migration replica %v during the migration revert", r.Name)
				return
			}
		}
	}()

	currentEngine, extras, err := datastore.GetCurrentEngineAndExtras(v, es)
	if err != nil {
		return err
	}

	if currentEngine.Status.CurrentState != longhorn.InstanceStateRunning {
		revertRequired = true
		log.Warnf("Need to revert the migration since the current engine %v is state %v", currentEngine.Name, currentEngine.Status.CurrentState)
		return nil
	}

	var availableEngines int
	var migrationEngine *longhorn.Engine
	for _, extra := range extras {
		if extra.DeletionTimestamp != nil {
			continue
		}

		// a valid migration engine is either a newly created engine that hasn't been assigned a node yet
		// or an existing engine that is running on the migration node, any additional extra engines would be unexpected.
		availableEngines++
		isValidMigrationEngine := extra.Spec.NodeID == "" || extra.Spec.NodeID == v.Spec.MigrationNodeID
		if isValidMigrationEngine {
			migrationEngine = extra
		}
	}

	// verify that we are in a valid state for migration
	// we only consider engines without a deletion timestamp as candidates for the migration engine
	unexpectedEngineCount := availableEngines > 1
	invalidMigrationEngine := availableEngines > 0 && migrationEngine == nil
	if unexpectedEngineCount || invalidMigrationEngine {
		revertRequired = true
		log.Warnf("Unexpected state for migration, current engine count %v has invalid migration engine %v",
			len(es), invalidMigrationEngine)
		return nil
	}

	if migrationEngine == nil {
		migrationEngine, err = c.createEngine(v, currentEngine.Name)
		if err != nil {
			return err
		}
		es[migrationEngine.Name] = migrationEngine
	}

	log = log.WithField("migrationEngine", migrationEngine.Name)

	ready := false
	if ready, revertRequired, err = c.prepareReplicasAndEngineForMigration(v, currentEngine, migrationEngine, rs); err != nil {
		return err
	}
	if !ready || revertRequired {
		return nil
	}

	log.Info("volume migration engine is ready")
	return nil
}

func isMigratableVolume(v *longhorn.Volume) bool {
	return v.Spec.Migratable && v.Spec.AccessMode == longhorn.AccessModeReadWriteMany
}

func (c *VolumeController) prepareReplicasAndEngineForMigration(v *longhorn.Volume, currentEngine, migrationEngine *longhorn.Engine, rs map[string]*longhorn.Replica) (ready, revertRequired bool, err error) {
	log := getLoggerForVolume(c.logger, v).WithFields(logrus.Fields{"migrationNodeID": v.Spec.MigrationNodeID, "migrationEngine": migrationEngine.Name})

	// Check the migration engine current status
	if migrationEngine.Spec.NodeID != "" && migrationEngine.Spec.NodeID != v.Spec.MigrationNodeID {
		log.Warnf("Migration engine is on node %v but volume is somehow required to be migrated to %v, will do revert then restart the migration",
			migrationEngine.Spec.NodeID, v.Spec.MigrationNodeID)
		return false, true, nil
	}
	if migrationEngine.DeletionTimestamp != nil {
		log.Warn("Migration engine is in deletion, will start or continue migration revert")
		return false, true, nil
	}
	if migrationEngine.Spec.DesireState != longhorn.InstanceStateStopped && migrationEngine.Spec.DesireState != longhorn.InstanceStateRunning {
		log.Warnf("Need to do revert since the migration engine contains an invalid desire state %v", migrationEngine.Spec.DesireState)
		return false, true, nil
	}
	if migrationEngine.Status.CurrentState == longhorn.InstanceStateError {
		log.Errorf("The migration engine is state %v, need to do revert then retry the migration", migrationEngine.Status.CurrentState)
		return false, true, nil
	}

	// Sync migration replicas with old replicas
	currentAvailableReplicas := map[string]*longhorn.Replica{}
	migrationReplicas := map[string]*longhorn.Replica{}
	for _, r := range rs {
		isUnavailable, err := c.IsReplicaUnavailable(r)
		if err != nil {
			return false, false, err
		}
		if isUnavailable {
			continue
		}
		dataPath := types.GetReplicaDataPath(r.Spec.DiskPath, r.Spec.DataDirectoryName)
		if r.Spec.EngineName == currentEngine.Name {
			switch currentEngine.Status.ReplicaModeMap[r.Name] {
			case longhorn.ReplicaModeWO:
				log.Infof("Need to revert rather than starting migration since the current replica %v is mode WriteOnly, which means the rebuilding is in progress", r.Name)
				return false, true, nil
			case longhorn.ReplicaModeRW:
				currentAvailableReplicas[dataPath] = r
			case "":
				log.Warnf("Running replica %v wasn't added to engine, will ignore it and continue migration", r.Name)
			default:
				log.Warnf("Unexpected mode %v for the current replica %v, will ignore it and continue migration", currentEngine.Status.ReplicaModeMap[r.Name], r.Name)
			}
		} else if r.Spec.EngineName == migrationEngine.Name {
			migrationReplicas[dataPath] = r
		} else {
			log.Warnf("During migration found unknown replica with engine %v, will directly remove it", r.Spec.EngineName)
			if err := c.deleteReplica(r, rs); err != nil {
				return false, false, err
			}
		}
	}

	if err := c.deleteInvalidMigrationReplicas(rs, currentAvailableReplicas, migrationReplicas); err != nil {
		return false, false, err
	}

	if err := c.createAndStartMatchingReplicas(v, rs, currentAvailableReplicas, migrationReplicas, func(r *longhorn.Replica, engineName string) {
		r.Spec.EngineName = engineName
	}, migrationEngine.Name); err != nil {
		return false, false, err
	}

	if len(migrationReplicas) == 0 {
		log.Warnf("Volume %v: no valid migration replica during the migration, need to do revert first", v.Name)
		return false, true, nil
	}

	// Sync the migration engine with migration replicas
	replicaAddressMap := map[string]string{}
	allMigrationReplicasReady := true
	for _, r := range migrationReplicas {
		if r.Status.CurrentState != longhorn.InstanceStateRunning {
			allMigrationReplicasReady = false
			continue
		}
		if r.Status.IP == "" {
			log.Warnf("Replica %v is running but IP is empty", r.Name)
			continue
		}
		if r.Status.StorageIP == "" {
			log.Warnf("Replica %v is running but storage IP is empty", r.Name)
			continue
		}
		if r.Status.Port == 0 {
			log.Warnf("Replica %v is running but Port is empty", r.Name)
			continue
		}
		replicaAddressMap[r.Name] = imutil.GetURL(r.Status.StorageIP, r.Status.Port)
	}
	if migrationEngine.Spec.DesireState != longhorn.InstanceStateStopped {
		if len(replicaAddressMap) == 0 {
			log.Warn("No available migration replica for the current running engine, will direct do revert")
			return false, true, nil
		}
		// If there are some migration replicas not ready yet or not in the engine, need to restart the engine so that
		// the missed replicas can be added to the engine without rebuilding.
		if !allMigrationReplicasReady || !reflect.DeepEqual(migrationEngine.Spec.ReplicaAddressMap, replicaAddressMap) {
			log.Warn("The current available migration replicas do not match the record in the migration engine status, will restart the migration engine then update the replica map")
			migrationEngine.Spec.NodeID = ""
			migrationEngine.Spec.ReplicaAddressMap = map[string]string{}
			migrationEngine.Spec.DesireState = longhorn.InstanceStateStopped
			return false, false, nil
		}
	} else { // migrationEngine.Spec.DesireState == longhorn.InstanceStateStopped
		if migrationEngine.Status.CurrentState != longhorn.InstanceStateStopped || !allMigrationReplicasReady {
			return false, false, nil
		}
	}

	migrationEngine.Spec.NodeID = v.Spec.MigrationNodeID
	migrationEngine.Spec.ReplicaAddressMap = replicaAddressMap
	migrationEngine.Spec.DesireState = longhorn.InstanceStateRunning

	if migrationEngine.Status.CurrentState != longhorn.InstanceStateRunning {
		return false, false, nil
	}

	return true, false, nil
}

func (c *VolumeController) IsReplicaUnavailable(r *longhorn.Replica) (bool, error) {
	if r.Spec.NodeID == "" || r.Spec.DiskID == "" || r.Spec.DiskPath == "" || r.Spec.DataDirectoryName == "" {
		return true, nil
	}

	isDownOrDeleted, err := c.ds.IsNodeDownOrDeleted(r.Spec.NodeID)
	if err != nil {
		return true, errors.Wrapf(err, "failed  to check if node %v is still running for failed replica %v", r.Spec.NodeID, r.Name)
	}
	if isDownOrDeleted {
		return true, nil
	}

	node, err := c.ds.GetNode(r.Spec.NodeID)
	if err != nil {
		return true, errors.Wrapf(err, "failed to get node %v for failed replica %v", r.Spec.NodeID, r.Name)
	}
	for _, diskStatus := range node.Status.DiskStatus {
		if diskStatus.DiskUUID != r.Spec.DiskID {
			continue
		}
		if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeReady).Status != longhorn.ConditionStatusTrue {
			return true, nil
		}
	}

	return false, nil
}

// isResponsibleFor picks a running node that has the default engine image deployed.
// We need the default engine image deployed on the node to perform operation like backup operations.
// Prefer picking the node v.Spec.NodeID if it meet the above requirement.
func (c *VolumeController) isResponsibleFor(v *longhorn.Volume, defaultEngineImage string) (bool, error) {
	var err error
	defer func() {
		err = errors.Wrap(err, "error while checking isResponsibleFor")
	}()

	readyNodesWithDefaultEI, err := c.ds.ListReadyNodesWithEngineImage(defaultEngineImage)
	if err != nil {
		return false, err
	}

	isResponsible := isControllerResponsibleFor(c.controllerID, c.ds, v.Name, v.Spec.NodeID, v.Status.OwnerID)

	// No node in the system has the default engine image,
	// Fall back to the default logic where we pick a running node to be the owner
	if len(readyNodesWithDefaultEI) == 0 {
		return isResponsible, nil
	}

	preferredOwnerEngineAvailable, err := c.ds.CheckEngineImageReadiness(defaultEngineImage, v.Spec.NodeID)
	if err != nil {
		return false, err
	}
	currentOwnerEngineAvailable, err := c.ds.CheckEngineImageReadiness(defaultEngineImage, v.Status.OwnerID)
	if err != nil {
		return false, err
	}
	currentNodeEngineAvailable, err := c.ds.CheckEngineImageReadiness(defaultEngineImage, c.controllerID)
	if err != nil {
		return false, err
	}

	isPreferredOwner := currentNodeEngineAvailable && isResponsible
	continueToBeOwner := currentNodeEngineAvailable && !preferredOwnerEngineAvailable && c.controllerID == v.Status.OwnerID
	requiresNewOwner := currentNodeEngineAvailable && !preferredOwnerEngineAvailable && !currentOwnerEngineAvailable

	return isPreferredOwner || continueToBeOwner || requiresNewOwner, nil
}

func (c *VolumeController) deleteReplica(r *longhorn.Replica, rs map[string]*longhorn.Replica) error {
	// Must call Update before removal to keep the fields up to date
	if _, err := c.ds.UpdateReplica(r); err != nil {
		return err
	}
	if err := c.ds.DeleteReplica(r.Name); err != nil {
		return err
	}
	delete(rs, r.Name)
	return nil
}

func (c *VolumeController) deleteEngine(e *longhorn.Engine, es map[string]*longhorn.Engine) error {
	// Must call Update before removal to keep the fields up to date
	if _, err := c.ds.UpdateEngine(e); err != nil {
		return err
	}
	if err := c.ds.DeleteEngine(e.Name); err != nil {
		return err
	}
	delete(es, e.Name)
	return nil
}

// enqueueVolumesForShareManager enqueues all volumes that are currently claimed by this share manager
func (c *VolumeController) enqueueVolumesForShareManager(obj interface{}) {
	sm, isShareManager := obj.(*longhorn.ShareManager)
	if !isShareManager {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to requeue the claimed volumes
		sm, ok = deletedState.Obj.(*longhorn.ShareManager)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained non ShareManager object: %#v", deletedState.Obj))
			return
		}
	}

	// we can queue the key directly since a share manager only manages a single volume from it's own namespace
	// and there is no need for us to retrieve the whole object, since we already know the volume name
	key := sm.Namespace + "/" + sm.Name
	c.queue.Add(key)
}

// ReconcileShareManagerState is responsible for syncing the state of shared volumes with their share manager
func (c *VolumeController) ReconcileShareManagerState(volume *longhorn.Volume) error {
	log := getLoggerForVolume(c.logger, volume)
	sm, err := c.ds.GetShareManager(volume.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to get share manager for volume %v", volume.Name)
	}

	if volume.Spec.AccessMode != longhorn.AccessModeReadWriteMany || volume.Spec.Migratable {
		if sm != nil {
			log.Info("Removing share manager for non shared volume")
			if err := c.ds.DeleteShareManager(volume.Name); err != nil && !datastore.ErrorIsNotFound(err) {
				return err
			}
		}
		return nil
	}

	// no ShareManager create a new one
	if sm == nil {
		sm, err = c.createShareManagerForVolume(volume, c.smImage)
		if err != nil {
			return errors.Wrapf(err, "failed to create share manager %v", volume.Name)
		}
	}

	if sm.Spec.Image != c.smImage {
		sm.Spec.Image = c.smImage
		sm.ObjectMeta.Labels = types.GetShareManagerLabels(volume.Name, c.smImage)
		if sm, err = c.ds.UpdateShareManager(sm); err != nil {
			return err
		}

		log.Infof("Updated image for share manager from %v to %v", sm.Spec.Image, c.smImage)
	}

	// kill the workload pods, when the share manager goes into error state
	// easiest approach is to set the RemountRequestedAt variable,
	// since that is already responsible for killing the workload pods
	if sm.Status.State == longhorn.ShareManagerStateError || sm.Status.State == longhorn.ShareManagerStateUnknown {
		volume.Status.RemountRequestedAt = c.nowHandler()
		msg := fmt.Sprintf("Volume %v requested remount at %v", volume.Name, volume.Status.RemountRequestedAt)
		c.eventRecorder.Eventf(volume, corev1.EventTypeNormal, constant.EventReasonRemount, msg)
	}

	// sync the share state and endpoint
	volume.Status.ShareState = sm.Status.State
	volume.Status.ShareEndpoint = sm.Status.Endpoint
	return nil
}

func (c *VolumeController) createShareManagerForVolume(volume *longhorn.Volume, image string) (*longhorn.ShareManager, error) {
	sm := &longhorn.ShareManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:            volume.Name,
			Namespace:       c.namespace,
			Labels:          types.GetShareManagerLabels(volume.Name, image),
			OwnerReferences: datastore.GetOwnerReferencesForVolume(volume),
		},
		Spec: longhorn.ShareManagerSpec{
			Image: image,
		},
	}

	return c.ds.CreateShareManager(sm)
}

// enqueueVolumesForBackupVolume enqueues the volumes which is/are DR volumes or
// the volume name matches backup volume name
func (c *VolumeController) enqueueVolumesForBackupVolume(obj interface{}) {
	bv, isBackupVolume := obj.(*longhorn.BackupVolume)
	if !isBackupVolume {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to requeue the claimed volumes
		bv, ok = deletedState.Obj.(*longhorn.BackupVolume)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained non BackupVolume object: %#v", deletedState.Obj))
			return
		}
	}

	// Update last backup for the volume name matches backup volume name
	var matchedVolumeName string
	_, err := c.ds.GetVolume(bv.Name)
	if err == nil {
		matchedVolumeName = bv.Name
		key := bv.Namespace + "/" + bv.Name
		c.queue.Add(key)
	}

	// Update last backup for DR volumes
	volumes, err := c.ds.ListDRVolumesROWithBackupVolumeName(bv.Name)
	if err != nil {
		return
	}
	for volumeName := range volumes {
		if volumeName == matchedVolumeName {
			// Skip the volume which be enqueued already
			continue
		}

		key := bv.Namespace + "/" + volumeName
		c.queue.Add(key)
	}
}

func (c *VolumeController) enqueueVolumesForBackingImageDataSource(obj interface{}) {
	bids, isBackingImageDataSource := obj.(*longhorn.BackingImageDataSource)
	if !isBackingImageDataSource {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to requeue the claimed volumes
		bids, ok = deletedState.Obj.(*longhorn.BackingImageDataSource)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained non BackingImageDataSource object: %#v", deletedState.Obj))
			return
		}
	}

	if volumeName := bids.Labels[types.GetLonghornLabelKey(types.LonghornLabelExportFromVolume)]; volumeName != "" {
		key := bids.Namespace + "/" + volumeName
		c.queue.Add(key)
	}
}

func (c *VolumeController) enqueueNodeChange(obj interface{}) {
	node, ok := obj.(*longhorn.Node)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		node, ok = deletedState.Obj.(*longhorn.Node)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	replicas, err := c.ds.ListReplicasRO()
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to list replicas when enqueuing node %v: %v", node.Name, err))
		return
	}
	for _, r := range replicas {
		vol, err := c.ds.GetVolumeRO(r.Spec.VolumeName)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to get volume %v of replica %v when enqueuing node %v: %v", r.Spec.VolumeName, r.Name, node.Name, err))
			continue
		}
		replicaAutoBalance, err := c.getAutoBalancedReplicasSetting(vol)
		if err != nil {
			c.logger.Warnf(err.Error())
		}
		if r.Spec.NodeID == "" || r.Spec.FailedAt != "" || replicaAutoBalance != longhorn.ReplicaAutoBalanceDisabled {
			c.enqueueVolume(vol)
		}
	}
}

func isSettingRelatedToVolume(obj interface{}) bool {
	setting, ok := obj.(*longhorn.Setting)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return false
		}

		// use the last known state, to enqueue, dependent objects
		setting, ok = deletedState.Obj.(*longhorn.Setting)
		if !ok {
			return false
		}
	}

	return types.SettingsRelatedToVolume[setting.Name] == types.LonghornLabelValueIgnored
}

func (c *VolumeController) enqueueSettingChange(obj interface{}) {
	setting, ok := obj.(*longhorn.Setting)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to requeue the claimed volumes
		setting, ok = deletedState.Obj.(*longhorn.Setting)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained non Setting object: %#v", deletedState.Obj))
			return
		}
	}

	vs, err := c.ds.ListVolumesFollowsGlobalSettingsRO(map[string]bool{setting.Name: true})
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to list volumes when enqueuing setting %v: %v", types.SettingNameRemoveSnapshotsDuringFilesystemTrim, err))
		return
	}
	for _, v := range vs {
		c.enqueueVolume(v)
	}
}

// ReconcileBackupVolumeState is responsible for syncing the state of backup volumes to volume.status
func (c *VolumeController) ReconcileBackupVolumeState(volume *longhorn.Volume) error {
	log := getLoggerForVolume(c.logger, volume)

	// Update last backup for the DR/restore volume or
	// update last backup for the volume name matches backup volume name
	var backupVolumeName string
	if volume.Status.RestoreRequired {
		name, ok := volume.Labels[types.LonghornLabelBackupVolume]
		if !ok {
			log.Warn("Failed to find the backup volume label")
			return nil
		}
		backupVolumeName = name
	} else {
		backupVolumeName = volume.Name
	}

	bv, err := c.ds.GetBackupVolume(backupVolumeName)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to get backup volume %s for volume %v", backupVolumeName, volume.Name)
	}

	// Clean up last backup if the BackupVolume CR gone
	if bv == nil || !bv.DeletionTimestamp.IsZero() {
		volume.Status.LastBackup = ""
		volume.Status.LastBackupAt = ""
		return nil
	}
	// Set last backup
	volume.Status.LastBackup = bv.Status.LastBackupName
	volume.Status.LastBackupAt = bv.Status.LastBackupAt
	return nil
}

// TODO: this block of code is duplicated of CreateSnapshot in MANAGER package.
// Once we have Snapshot CR, we should refactor this

func (c *VolumeController) createSnapshot(snapshotName string, labels map[string]string, volume *longhorn.Volume, e *longhorn.Engine) (*longhorn.SnapshotInfo, error) {
	if volume.Name == "" {
		return nil, fmt.Errorf("volume name required")
	}

	if err := util.VerifySnapshotLabels(labels); err != nil {
		return nil, err
	}

	if err := c.checkVolumeNotInMigration(volume); err != nil {
		return nil, err
	}

	engineCliClient, err := engineapi.GetEngineBinaryClient(c.ds, volume.Name, c.controllerID)
	if err != nil {
		return nil, err
	}

	engineClientProxy, err := engineapi.GetCompatibleClient(e, engineCliClient, c.ds, c.logger, c.proxyConnCounter)
	if err != nil {
		return nil, err
	}
	defer engineClientProxy.Close()

	// Check if we have already created a snapshot with this name.
	// TODO: Update longhorn-engine and longhorn-instance-manager so that SnapshotCreate returns an identifiable
	// error/code when a snapshot exists so that this check isn't necessary.
	if snapshotName != "" {
		snap, err := engineClientProxy.SnapshotGet(e, snapshotName)
		if err != nil {
			return nil, err
		}
		if snap != nil {
			return snap, nil
		}
	}

	snapshotName, err = engineClientProxy.SnapshotCreate(e, snapshotName, labels)
	if err != nil {
		return nil, err
	}

	snap, err := engineClientProxy.SnapshotGet(e, snapshotName)
	if err != nil {
		return nil, err
	}

	if snap == nil {
		return nil, fmt.Errorf("failed to found just created snapshot '%s', for volume '%s'", snapshotName, volume.Name)
	}

	logrus.Infof("Created snapshot %v with labels %+v for volume %v", snapshotName, labels, volume.Name)
	return snap, nil
}

func (c *VolumeController) checkVolumeNotInMigration(volume *longhorn.Volume) error {
	if volume.Spec.MigrationNodeID != "" {
		return fmt.Errorf("cannot operate during migration")
	}
	return nil
}

// ReconcilePersistentVolume is responsible for syncing the state with the PersistentVolume
func (c *VolumeController) ReconcilePersistentVolume(volume *longhorn.Volume) error {
	log := getLoggerForVolume(c.logger, volume)

	kubeStatus := volume.Status.KubernetesStatus
	if kubeStatus.PVName == "" {
		return nil
	}

	pv, err := c.ds.GetPersistentVolume(kubeStatus.PVName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	existingPV := pv.DeepCopy()
	defer func() {
		if !reflect.DeepEqual(existingPV.Spec, pv.Spec) {
			logrus.Infof("Updating PersistentVolume %v", pv.Name)
			_, err = c.ds.UpdatePersistentVolume(pv)

			// requeue if it's conflict
			if apierrors.IsConflict(errors.Cause(err)) {
				log.WithError(err).Debugf("Requeue volume due to error")
				c.enqueueVolume(volume)
			}
		}
	}()

	if volume.Spec.DataLocality == longhorn.DataLocalityStrictLocal && volume.Spec.NodeID != "" {
		pv.Spec.NodeAffinity = &corev1.VolumeNodeAffinity{
			Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{
				util.GetNodeSelectorTermMatchExpressionNodeName(volume.Spec.NodeID),
			}},
		}
	}
	return nil
}
