package controller

import (
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/util"
)

type SnapshotController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds                     *datastore.DataStore
	cacheSyncs             []cache.InformerSynced
	engineClientCollection engineapi.EngineClientCollection

	proxyConnCounter util.Counter
}

func NewSnapshotController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	namespace string,
	controllerID string,
	engineClientCollection engineapi.EngineClientCollection,
	proxyConnCounter util.Counter,
) *SnapshotController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	sc := &SnapshotController{
		baseController: newBaseController("longhorn-snapshot", logger),

		namespace:              namespace,
		controllerID:           controllerID,
		kubeClient:             kubeClient,
		eventRecorder:          eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-snapshot-controller"}),
		ds:                     ds,
		engineClientCollection: engineClientCollection,
		proxyConnCounter:       proxyConnCounter,
	}

	ds.SnapshotInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    sc.enqueueSnapshot,
		UpdateFunc: func(old, cur interface{}) { sc.enqueueSnapshot(cur) },
		DeleteFunc: sc.enqueueSnapshot,
	}, 0)
	sc.cacheSyncs = append(sc.cacheSyncs, ds.SnapshotInformer.HasSynced)
	ds.EngineInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		UpdateFunc: sc.enqueueEngineChange,
	}, 0)
	sc.cacheSyncs = append(sc.cacheSyncs, ds.EngineInformer.HasSynced)

	ds.VolumeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		DeleteFunc: sc.enqueueVolumeChange,
	}, 0)
	sc.cacheSyncs = append(sc.cacheSyncs, ds.VolumeInformer.HasSynced)

	return sc
}

func (sc *SnapshotController) enqueueSnapshot(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	sc.queue.Add(key)
}

func (sc *SnapshotController) enqueueEngineChange(oldObj, curObj interface{}) {
	curEngine, ok := curObj.(*longhorn.Engine)
	if !ok {
		deletedState, ok := curObj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", curObj))
			return
		}
		// use the last known state, to enqueue, dependent objects
		curEngine, ok = deletedState.Obj.(*longhorn.Engine)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	vol, err := sc.ds.GetVolumeRO(curEngine.Spec.VolumeName)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("snapshot controller failed to get volume %v when enqueuing engine %v: %v", curEngine.Spec.VolumeName, curEngine.Name, err))
		return
	}

	if vol.Status.OwnerID != sc.controllerID {
		return
	}

	oldEngine, ok := oldObj.(*longhorn.Engine)
	if !ok {
		deletedState, ok := oldObj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", oldObj))
			return
		}
		// use the last known state, to enqueue, dependent objects
		oldEngine, ok = deletedState.Obj.(*longhorn.Engine)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	needEnqueueSnapshots := curEngine.Status.CurrentState != oldEngine.Status.CurrentState ||
		!reflect.DeepEqual(curEngine.Status.PurgeStatus, oldEngine.Status.PurgeStatus) ||
		!reflect.DeepEqual(curEngine.Status.Snapshots, oldEngine.Status.Snapshots)

	if !needEnqueueSnapshots {
		return
	}

	snapshots, err := sc.ds.ListVolumeSnapshotsRO(curEngine.Spec.VolumeName)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to list snapshots for volume %v when enqueuing engine %v: %v", curEngine.Spec.VolumeName, oldEngine.Name, err))
		return
	}

	snapshots = filterSnapshotsForEngineEnqueuing(oldEngine, curEngine, snapshots)
	for _, snap := range snapshots {
		sc.enqueueSnapshot(snap)
	}

	return
}

func filterSnapshotsForEngineEnqueuing(oldEngine, curEngine *longhorn.Engine, snapshots map[string]*longhorn.Snapshot) map[string]*longhorn.Snapshot {
	targetSnapshots := make(map[string]*longhorn.Snapshot)

	if curEngine.Status.CurrentState != oldEngine.Status.CurrentState {
		return snapshots
	}

	if !reflect.DeepEqual(curEngine.Status.PurgeStatus, oldEngine.Status.PurgeStatus) {
		for snapName, snap := range snapshots {
			if snap.DeletionTimestamp != nil {
				targetSnapshots[snapName] = snap
			}
		}
	}

	for snapName, snap := range snapshots {
		if !reflect.DeepEqual(oldEngine.Status.Snapshots[snapName], curEngine.Status.Snapshots[snapName]) {
			targetSnapshots[snapName] = snap
		}
	}

	return targetSnapshots
}

func (sc *SnapshotController) enqueueVolumeChange(obj interface{}) {
	vol, ok := obj.(*longhorn.Volume)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}
		// use the last known state, to enqueue, dependent objects
		vol, ok = deletedState.Obj.(*longhorn.Volume)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}
	if vol.DeletionTimestamp.IsZero() {
		return
	}

	snapshots, err := sc.ds.ListVolumeSnapshotsRO(vol.Name)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("snapshot controller failed to list snapshots when enqueuing volume %v: %v", vol.Name, err))
		return
	}
	for _, snap := range snapshots {
		sc.enqueueSnapshot(snap)
	}
}

func (sc *SnapshotController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer sc.queue.ShutDown()

	sc.logger.Info("Starting Longhorn Snapshot Controller")
	defer sc.logger.Info("Shutting down Longhorn Snapshot Controller")

	if !cache.WaitForNamedCacheSync(sc.name, stopCh, sc.cacheSyncs...) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(sc.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (sc *SnapshotController) worker() {
	for sc.processNextWorkItem() {
	}
}

func (sc *SnapshotController) processNextWorkItem() bool {
	key, quit := sc.queue.Get()
	if quit {
		return false
	}
	defer sc.queue.Done(key)
	err := sc.syncHandler(key.(string))
	sc.handlerErr(err, key)
	return true
}

func (sc *SnapshotController) handlerErr(err error, key interface{}) {
	if err == nil {
		sc.queue.Forget(key)
		return
	}

	sc.logger.WithError(err).Warnf("Error syncing Longhorn snapshot %v", key)
	sc.queue.AddRateLimited(key)
	return
}

func (sc *SnapshotController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: fail to sync snapshot %v", sc.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != sc.namespace {
		return nil
	}
	return sc.reconcile(name)
}

func (sc *SnapshotController) reconcile(snapshotName string) (err error) {
	snapshot, err := sc.ds.GetSnapshot(snapshotName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	isResponsible, err := sc.isResponsibleFor(snapshot)
	if err != nil {
		return err
	}
	if !isResponsible {
		return nil
	}

	log := getLoggerForSnapshot(sc.logger, snapshot)

	existingSnapshot := snapshot.DeepCopy()
	defer func() {
		if err != nil && !shouldUpdateObject(err) {
			return
		}
		if reflect.DeepEqual(existingSnapshot.Status, snapshot.Status) {
			return
		}

		snapshot, err = sc.ds.UpdateSnapshotStatus(snapshot)
		if err != nil {
			return
		}
		sc.generatingEventsForSnapshot(existingSnapshot, snapshot)
		return
	}()

	// deleting snapshotCR
	if !snapshot.DeletionTimestamp.IsZero() {
		isVolDeletedOrBeingDeleted, err := sc.isVolumeDeletedOrBeingDeleted(snapshot.Spec.Volume)
		if err != nil {
			return err
		}
		if isVolDeletedOrBeingDeleted {
			log.Infof("Removing finalizer for snapshot %v", snapshot.Name)
			return sc.ds.RemoveFinalizerForSnapshot(snapshot)
		}

		engine, err := sc.getTheOnlyEngineCRforSnapshot(snapshot)
		if err != nil {
			return err
		}
		if engine.Status.CurrentState != longhorn.InstanceStateRunning {
			// TODO: how to prevent duplicated event here
			sc.eventRecorder.Eventf(snapshot, v1.EventTypeWarning, "SnapshotDeleteError", "cannot delete snapshot because the volume engine %v is not running", engine.Name)
			sc.logger.Errorf("cannot delete snapshot because the volume engine %v is not running", engine.Name)
			return nil
		}
		// Delete the snapshot from engine process
		if err := sc.handleSnapshotDeletion(snapshot, engine); err != nil {
			return err
		}

		// Wait for the snapshot to be removed from engine.Status.Snapshots
		engine, err = sc.ds.GetEngineRO(engine.Name)
		if err != nil {
			return err
		}
		snapshotInfo, ok := engine.Status.Snapshots[snapshot.Name]
		if !ok {
			log.Infof("Removing finalizer for snapshot %v", snapshot.Name)
			return sc.ds.RemoveFinalizerForSnapshot(snapshot)
		}

		if err := syncSnapshotWithSnapshotInfo(snapshot, snapshotInfo, engine.Spec.VolumeSize); err != nil {
			return err
		}

		return nil
	}

	engine, err := sc.getTheOnlyEngineCRforSnapshot(snapshot)
	if err != nil {
		return err
	}

	// newly created snapshotCR by user
	requestCreateNewSnapshot := snapshot.Spec.CreateSnapshot
	alreadyCreatedBefore := snapshot.Status.CreationTime != ""
	if requestCreateNewSnapshot && !alreadyCreatedBefore {
		if engine.Status.CurrentState != longhorn.InstanceStateRunning {
			snapshot.Status.Error = fmt.Sprintf("cannot take snapshot because the volume engine %v is not running", engine.Name)
			return nil
		}
		err = sc.handleSnapshotCreate(snapshot, engine)
		if err != nil {
			snapshot.Status.Error = err.Error()
			return reconcileError{error: err, shouldUpdateObject: true}
		}
	}

	engine, err = sc.ds.GetEngineRO(engine.Name)
	if err != nil {
		return err
	}
	snapshotInfo, ok := engine.Status.Snapshots[snapshot.Name]
	if !ok {
		if !requestCreateNewSnapshot || alreadyCreatedBefore {
			// The snapshotInfo exists inside engine.Status.Snapshots before but disappears now.
			// Mark snapshotCR as lost track of the corresponding snapshotInfo
			log.Infof("snapshot CR %v lost track of its snapshotInfo", snapshot.Name)
			snapshot.Status.Error = "lost track of the corresponding snapshot info inside volume engine"
		}
		// Newly created snapshotCR, wait for the snapshotInfo to be appeared inside engine.Status.Snapshot
		snapshot.Status.ReadyToUse = false
		return nil
	}

	if err := syncSnapshotWithSnapshotInfo(snapshot, snapshotInfo, engine.Spec.VolumeSize); err != nil {
		return err
	}

	return nil
}

func (sc *SnapshotController) generatingEventsForSnapshot(existingSnapshot, snapshot *longhorn.Snapshot) {
	if !existingSnapshot.Status.MarkRemoved && snapshot.Status.MarkRemoved {
		sc.eventRecorder.Event(snapshot, v1.EventTypeWarning, "SnapshotDelete", "snapshot is marked as removed")
	}
	if snapshot.Spec.CreateSnapshot && existingSnapshot.Status.CreationTime == "" && snapshot.Status.CreationTime != "" {
		sc.eventRecorder.Eventf(snapshot, v1.EventTypeNormal, "SnapshotCreate", "successfully provisioned the snapshot")
	}
	if snapshot.Status.Error != "" && existingSnapshot.Status.Error != snapshot.Status.Error {
		sc.eventRecorder.Eventf(snapshot, v1.EventTypeWarning, "SnapshotError", "%v", snapshot.Status.Error)
	}
	if existingSnapshot.Status.ReadyToUse != snapshot.Status.ReadyToUse {
		if snapshot.Status.ReadyToUse {
			sc.eventRecorder.Eventf(snapshot, v1.EventTypeNormal, "SnapshotUpdate", "snapshot becomes ready to use")
		} else {
			sc.eventRecorder.Eventf(snapshot, v1.EventTypeWarning, "SnapshotUpdate", "snapshot becomes not ready to use")
		}
	}
}

func (sc *SnapshotController) isVolumeDeletedOrBeingDeleted(volumeName string) (bool, error) {
	volume, err := sc.ds.GetVolumeRO(volumeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return !volume.DeletionTimestamp.IsZero(), nil
}

func syncSnapshotWithSnapshotInfo(snap *longhorn.Snapshot, snapInfo *longhorn.SnapshotInfo, restoreSize int64) error {
	size, err := strconv.ParseInt(snapInfo.Size, 10, 64)
	if err != nil {
		return err
	}
	snap.Status.Parent = snapInfo.Parent
	snap.Status.Children = snapInfo.Children
	snap.Status.MarkRemoved = snapInfo.Removed
	snap.Status.UserCreated = snapInfo.UserCreated
	snap.Status.CreationTime = snapInfo.Created
	snap.Status.Size = size
	snap.Status.Labels = snapInfo.Labels
	snap.Status.RestoreSize = restoreSize
	if snap.Status.MarkRemoved || snap.DeletionTimestamp != nil {
		snap.Status.ReadyToUse = false
	} else {
		snap.Status.ReadyToUse = true
		snap.Status.Error = ""
	}
	return nil
}

func (sc *SnapshotController) getTheOnlyEngineCRforSnapshot(snapshot *longhorn.Snapshot) (*longhorn.Engine, error) {
	engines, err := sc.ds.ListVolumeEngines(snapshot.Spec.Volume)
	if err != nil {
		return nil, errors.Wrap(err, "getTheOnlyEngineCRforSnapshot")
	}
	if len(engines) != 1 {
		return nil, fmt.Errorf("getTheOnlyEngineCRforSnapshot: found more than 1 engines for volume %v", snapshot.Spec.Volume)
	}
	var engine *longhorn.Engine
	for _, e := range engines {
		engine = e
		break
	}
	return engine, nil
}

func (sc *SnapshotController) handleSnapshotCreate(snapshot *longhorn.Snapshot, engine *longhorn.Engine) error {
	engineCliClient, err := GetBinaryClientForEngine(engine, sc.engineClientCollection, engine.Status.CurrentImage)
	if err != nil {
		return err
	}

	engineClientProxy, err := engineapi.GetCompatibleClient(engine, engineCliClient, sc.ds, sc.logger, sc.proxyConnCounter)
	if err != nil {
		return err
	}
	defer engineClientProxy.Close()

	snapshotInfo, err := engineClientProxy.SnapshotGet(engine, snapshot.Name)
	if err != nil {
		return err
	}
	if snapshotInfo == nil {
		sc.logger.Infof("creating snapshot %v of volume %v", snapshot.Name, snapshot.Spec.Volume)
		_, err = engineClientProxy.SnapshotCreate(engine, snapshot.Name, snapshot.Spec.Labels)
		if err != nil {
			return err
		}
	}
	return nil
}

// handleSnapshotDeletion reaches out to engine process to check and delete the snapshot
func (sc *SnapshotController) handleSnapshotDeletion(snapshot *longhorn.Snapshot, engine *longhorn.Engine) error {
	engineCliClient, err := GetBinaryClientForEngine(engine, sc.engineClientCollection, engine.Status.CurrentImage)
	if err != nil {
		return err
	}
	engineClientProxy, err := engineapi.GetCompatibleClient(engine, engineCliClient, sc.ds, sc.logger, sc.proxyConnCounter)
	if err != nil {
		return err
	}
	defer engineClientProxy.Close()

	snapshotInfo, err := engineClientProxy.SnapshotGet(engine, snapshot.Name)
	if err != nil {
		return err
	}
	if snapshotInfo == nil {
		return nil
	}

	if !snapshotInfo.Removed {
		sc.logger.Infof("deleting snapshot %v", snapshot.Name)
		if err = engineClientProxy.SnapshotDelete(engine, snapshot.Name); err != nil {
			return err
		}
	}
	// TODO: Check if the purge failure is handled somewhere else
	purgeStatus, err := engineClientProxy.SnapshotPurgeStatus(engine)
	if err != nil {
		return errors.Wrap(err, "failed to get snapshot purge status")
	}
	isPurging := false
	for _, status := range purgeStatus {
		if status.IsPurging {
			isPurging = true
			break
		}
	}
	if !isPurging {
		sc.logger.Infof("start SnapshotPurge to delete snapshot %v", snapshot.Name)
		if err := engineClientProxy.SnapshotPurge(engine); err != nil {
			return err
		}
	}

	return nil
}

func (sc *SnapshotController) isResponsibleFor(snap *longhorn.Snapshot) (bool, error) {
	var err error
	defer func() {
		err = errors.Wrap(err, "error while checking isResponsibleFor")
	}()

	volume, err := sc.ds.GetVolumeRO(snap.Spec.Volume)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return sc.controllerID == volume.Status.OwnerID, nil
}

func getLoggerForSnapshot(logger logrus.FieldLogger, snap *longhorn.Snapshot) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"snapshot": snap.Name,
		},
	)
}

type reconcileError struct {
	error
	shouldUpdateObject bool
}

func shouldUpdateObject(err error) bool {
	switch v := err.(type) {
	case reconcileError:
		return v.shouldUpdateObject
	}
	return false
}
