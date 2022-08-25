package controller

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/kubernetes/pkg/controller"

	imapi "github.com/longhorn/longhorn-instance-manager/pkg/api"
	imclient "github.com/longhorn/longhorn-instance-manager/pkg/client"
	imutil "github.com/longhorn/longhorn-instance-manager/pkg/util"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	unknownReplicaPrefix = "UNKNOWN-"
)

var (
	EnginePollInterval = 5 * time.Second
	EnginePollTimeout  = 30 * time.Second

	EngineMonitorConflictRetryCount = 5

	purgeWaitIntervalInSecond = 24 * 60 * 60
)

const (
	ConflictRetryCount = 5
)

type EngineController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced

	backoff *flowcontrol.Backoff

	instanceHandler *InstanceHandler

	engines            engineapi.EngineClientCollection
	engineMonitorMutex *sync.RWMutex
	engineMonitorMap   map[string]chan struct{}

	proxyConnCounter util.Counter
}

type EngineMonitor struct {
	logger logrus.FieldLogger

	namespace     string
	ds            *datastore.DataStore
	eventRecorder record.EventRecorder

	Name             string
	engines          engineapi.EngineClientCollection
	stopCh           chan struct{}
	expansionBackoff *flowcontrol.Backoff

	controllerID string
	// used to notify the controller that monitoring has stopped
	monitorVoluntaryStopCh chan struct{}

	proxyConnCounter util.Counter
}

func NewEngineController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	engines engineapi.EngineClientCollection,
	namespace string, controllerID string,
	proxyConnCounter util.Counter) *EngineController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events("")})

	ec := &EngineController{
		baseController: newBaseController("longhorn-engine", logger),

		ds:        ds,
		namespace: namespace,

		controllerID:  controllerID,
		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-engine-controller"}),

		backoff: flowcontrol.NewBackOff(time.Second*10, time.Minute*5),

		engines:            engines,
		engineMonitorMutex: &sync.RWMutex{},
		engineMonitorMap:   map[string]chan struct{}{},

		proxyConnCounter: proxyConnCounter,
	}
	ec.instanceHandler = NewInstanceHandler(ds, ec, ec.eventRecorder)

	ds.EngineInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ec.enqueueEngine,
		UpdateFunc: func(old, cur interface{}) { ec.enqueueEngine(cur) },
		DeleteFunc: ec.enqueueEngine,
	})
	ec.cacheSyncs = append(ec.cacheSyncs, ds.EngineInformer.HasSynced)

	ds.InstanceManagerInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    ec.enqueueInstanceManagerChange,
		UpdateFunc: func(old, cur interface{}) { ec.enqueueInstanceManagerChange(cur) },
		DeleteFunc: ec.enqueueInstanceManagerChange,
	}, 0)
	ec.cacheSyncs = append(ec.cacheSyncs, ds.InstanceManagerInformer.HasSynced)

	return ec
}

func (ec *EngineController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer ec.queue.ShutDown()

	ec.logger.Info("Start Longhorn engine controller")
	defer ec.logger.Info("Shutting down Longhorn engine controller")

	if !cache.WaitForNamedCacheSync("longhorn engines", stopCh, ec.cacheSyncs...) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(ec.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (ec *EngineController) worker() {
	for ec.processNextWorkItem() {
	}
}

func (ec *EngineController) processNextWorkItem() bool {
	key, quit := ec.queue.Get()

	if quit {
		return false
	}
	defer ec.queue.Done(key)

	err := ec.syncEngine(key.(string))
	ec.handleErr(err, key)

	return true
}

func (ec *EngineController) handleErr(err error, key interface{}) {
	if err == nil {
		ec.queue.Forget(key)
		return
	}

	log := ec.logger.WithField("engine", key)
	if ec.queue.NumRequeues(key) < maxRetries {
		log.WithError(err).Warn("Error syncing Longhorn engine")
		ec.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	log.WithError(err).Warn("Dropping Longhorn engine out of the queue")
	ec.queue.Forget(key)
}

func getLoggerForEngine(logger logrus.FieldLogger, e *longhorn.Engine) *logrus.Entry {
	return logger.WithField("engine", e.Name)
}

func (ec *EngineController) getEngineClientProxy(e *longhorn.Engine, image string) (engineapi.EngineClientProxy, error) {
	engineCliClient, err := GetBinaryClientForEngine(e, ec.engines, image)
	if err != nil {
		return nil, err
	}

	return engineapi.GetCompatibleClient(e, engineCliClient, ec.ds, ec.logger, ec.proxyConnCounter)
}

func (ec *EngineController) syncEngine(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "fail to sync engine for %v", key)
	}()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != ec.namespace {
		// Not ours, don't do anything
		return nil
	}

	log := ec.logger.WithField("engine", name)
	engine, err := ec.ds.GetEngine(name)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			log.Info("Engine has been deleted")
			return nil
		}
		return err
	}

	defaultEngineImage, err := ec.ds.GetSettingValueExisted(types.SettingNameDefaultEngineImage)
	if err != nil {
		return err
	}

	isResponsible, err := ec.isResponsibleFor(engine, defaultEngineImage)
	if err != nil {
		return err
	}
	if !isResponsible {
		return nil
	}
	if engine.Status.OwnerID != ec.controllerID {
		engine.Status.OwnerID = ec.controllerID
		engine, err = ec.ds.UpdateEngineStatus(engine)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
		log.Infof("Engine got new owner %v", ec.controllerID)
	}

	if engine.DeletionTimestamp != nil {
		if err := ec.DeleteInstance(engine); err != nil {
			return errors.Wrapf(err, "failed to cleanup the related engine process before deleting engine %v", engine.Name)
		}
		return ec.ds.RemoveFinalizerForEngine(engine)
	}

	existingEngine := engine.DeepCopy()
	defer func() {
		// we're going to update engine assume things changes
		if err == nil && !reflect.DeepEqual(existingEngine.Status, engine.Status) {
			_, err = ec.ds.UpdateEngineStatus(engine)
		}
		// requeue if it's conflict
		if apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Debug("Requeue engine due to conflict")
			ec.enqueueEngine(engine)
			err = nil
		}
	}()

	isCLIAPIVersionOne := false
	if engine.Status.CurrentImage != "" {
		isCLIAPIVersionOne, err = ec.ds.IsEngineImageCLIAPIVersionOne(engine.Status.CurrentImage)
		if err != nil {
			return err
		}
	}

	syncReplicaAddressMap := false
	if len(engine.Spec.UpgradedReplicaAddressMap) != 0 && engine.Status.CurrentImage != engine.Spec.EngineImage {
		if err := ec.Upgrade(engine); err != nil {
			// Engine live upgrade failure shouldn't block the following engine state update.
			log.Error(err)
			// Sync replica address map as usual when the upgrade fails.
			syncReplicaAddressMap = true
		}
	} else if len(engine.Spec.UpgradedReplicaAddressMap) == 0 {
		syncReplicaAddressMap = true
	}
	if syncReplicaAddressMap && !reflect.DeepEqual(engine.Status.CurrentReplicaAddressMap, engine.Spec.ReplicaAddressMap) {
		log.Debug("Updating engine current replica address map")
		engine.Status.CurrentReplicaAddressMap = engine.Spec.ReplicaAddressMap
		// Make sure the CurrentReplicaAddressMap persist in the etcd before continue
		return nil
	}

	if err := ec.instanceHandler.ReconcileInstanceState(engine, &engine.Spec.InstanceSpec, &engine.Status.InstanceStatus); err != nil {
		return err
	}

	// For incompatible engine, skip starting engine monitor and clean up fields when the engine is not running
	if isCLIAPIVersionOne {
		if engine.Status.CurrentState != longhorn.InstanceStateRunning {
			engine.Status.Endpoint = ""
			engine.Status.ReplicaModeMap = nil
		}
		return nil
	}

	if engine.Status.CurrentState == longhorn.InstanceStateRunning {
		// we allow across monitoring temporaily due to migration case
		if !ec.isMonitoring(engine) {
			ec.startMonitoring(engine)
		} else if engine.Status.ReplicaModeMap != nil {
			if err := ec.ReconcileEngineState(engine); err != nil {
				return err
			}
		}
	} else if ec.isMonitoring(engine) {
		// engine is not running
		ec.resetAndStopMonitoring(engine)
	}

	if syncErr := ec.syncSnapshotCRs(engine); syncErr != nil {
		return errors.Wrapf(err, "Failed to sync with snapshot CRs for engine %v: %v", engine.Name, syncErr)
	}

	return nil
}

func (ec *EngineController) enqueueEngine(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	ec.queue.Add(key)
}

func (ec *EngineController) enqueueInstanceManagerChange(obj interface{}) {
	im, isInstanceManager := obj.(*longhorn.InstanceManager)
	if !isInstanceManager {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		im, ok = deletedState.Obj.(*longhorn.InstanceManager)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	imType, err := datastore.CheckInstanceManagerType(im)
	if err != nil || imType != longhorn.InstanceManagerTypeEngine {
		return
	}

	engineMap := map[string]*longhorn.Engine{}

	es, err := ec.ds.ListEnginesRO()
	if err != nil {
		ec.logger.WithError(err).Warnf("failed to list engines")
	}
	for _, e := range es {
		// when attaching, instance manager name is not available
		// when detaching, node ID is not available
		if e.Spec.NodeID == im.Spec.NodeID || e.Status.InstanceManagerName == im.Name {
			engineMap[e.Name] = e
		}
	}

	for _, e := range engineMap {
		ec.enqueueEngine(e)
	}

}

func (ec *EngineController) CreateInstance(obj interface{}) (*longhorn.InstanceProcess, error) {
	e, ok := obj.(*longhorn.Engine)
	if !ok {
		return nil, fmt.Errorf("BUG: invalid object for engine process creation: %v", obj)
	}
	if e.Spec.VolumeName == "" || e.Spec.NodeID == "" {
		return nil, fmt.Errorf("missing parameters for engine process creation: %v", e)
	}
	frontend := e.Spec.Frontend
	if e.Spec.DisableFrontend {
		frontend = longhorn.VolumeFrontendEmpty
	}

	im, err := ec.ds.GetInstanceManagerByInstance(obj)
	if err != nil {
		return nil, err
	}
	c, err := engineapi.NewInstanceManagerClient(im)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	return c.EngineProcessCreate(e.Name, e.Spec.VolumeName, e.Spec.EngineImage, frontend, e.Status.CurrentReplicaAddressMap, e.Spec.RevisionCounterDisabled, e.Spec.SalvageRequested)
}

func (ec *EngineController) DeleteInstance(obj interface{}) error {
	e, ok := obj.(*longhorn.Engine)
	if !ok {
		return fmt.Errorf("BUG: invalid object for engine process deletion: %v", obj)
	}
	log := getLoggerForEngine(ec.logger, e)

	if err := ec.deleteInstanceWithCLIAPIVersionOne(e); err != nil {
		return err
	}

	var im *longhorn.InstanceManager
	var err error
	// Not assigned or not updated, try best to delete
	if e.Status.InstanceManagerName == "" {
		if e.Spec.NodeID == "" {
			log.Warnf("Engine %v does not set instance manager name and node ID, will skip the actual process deletion", e.Name)
			return nil
		}
		im, err = ec.ds.GetInstanceManagerByInstance(obj)
		if err != nil {
			log.Warnf("Failed to detect instance manager for engine %v, will skip the actual process deletion: %v", e.Name, err)
			return nil
		}
		log.Infof("Try best to clean up the process for engine %v in instance manager %v", e.Name, im.Name)
	} else {
		im, err = ec.ds.GetInstanceManager(e.Status.InstanceManagerName)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			// The related node may be directly deleted.
			log.Warnf("The engine instance manager %v is gone during the engine instance %v deletion. Will do nothing for the deletion", e.Status.InstanceManagerName, e.Name)
			return nil
		}
	}

	if im.Status.CurrentState != longhorn.InstanceManagerStateRunning {
		return nil
	}

	// For the engine process in instance manager v0.7.0, we need to use the cmdline to delete the process
	// and stop the iscsi
	if im.Status.APIVersion == engineapi.IncompatibleInstanceManagerAPIVersion {
		url := imutil.GetURL(im.Status.IP, engineapi.InstanceManagerDefaultPort)
		args := []string{"--url", url, "engine", "delete", "--name", e.Name}
		if _, err := util.ExecuteWithoutTimeout([]string{}, engineapi.GetDeprecatedInstanceManagerBinary(e.Status.CurrentImage), args...); err != nil && !types.ErrorIsNotFound(err) {
			return err
		}

		// Directly remove the instance from the map. Best effort.
		delete(im.Status.Instances, e.Name)
		if _, err := ec.ds.UpdateInstanceManagerStatus(im); err != nil {
			return err
		}
		return nil
	}

	c, err := engineapi.NewInstanceManagerClient(im)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.ProcessDelete(e.Name); err != nil && !types.ErrorIsNotFound(err) {
		return err
	}

	return nil
}

func (ec *EngineController) deleteInstanceWithCLIAPIVersionOne(e *longhorn.Engine) (err error) {
	isCLIAPIVersionOne := false
	if e.Status.CurrentImage != "" {
		isCLIAPIVersionOne, err = ec.ds.IsEngineImageCLIAPIVersionOne(e.Status.CurrentImage)
		if err != nil {
			return err
		}
	}

	if isCLIAPIVersionOne {
		pod, err := ec.kubeClient.CoreV1().Pods(ec.namespace).Get(context.TODO(), e.Name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get pod for old engine %v", e.Name)
		}
		if apierrors.IsNotFound(err) {
			pod = nil
		}

		ec.logger.WithField("engine", e.Name).Debug("Prepared to delete engine pod because of outdated version")
		if err := ec.deleteOldEnginePod(pod, e); err != nil {
			return err
		}
	}
	return nil
}

func (ec *EngineController) deleteOldEnginePod(pod *v1.Pod, e *longhorn.Engine) (err error) {
	// pod already stopped
	if pod == nil {
		return nil
	}

	log := ec.logger.WithField("pod", pod.Name)
	if pod.DeletionTimestamp != nil {
		if pod.DeletionGracePeriodSeconds != nil && *pod.DeletionGracePeriodSeconds != 0 {
			// force deletion in the case of node lost
			deletionDeadline := pod.DeletionTimestamp.Add(time.Duration(*pod.DeletionGracePeriodSeconds) * time.Second)
			now := time.Now().UTC()
			if now.After(deletionDeadline) {
				log.Debugf("engine pod still exists after grace period %v passed, force deletion: now %v, deadline %v",
					pod.DeletionGracePeriodSeconds, now, deletionDeadline)
				gracePeriod := int64(0)
				if err := ec.kubeClient.CoreV1().Pods(ec.namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod}); err != nil {
					log.WithError(err).Debugf("failed to force delete engine pod")
					return nil
				}
			}
		}
		return nil
	}

	if err := ec.kubeClient.CoreV1().Pods(ec.namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{}); err != nil {
		ec.eventRecorder.Eventf(e, v1.EventTypeWarning, EventReasonFailedStopping, "Error stopping pod for old engine %v: %v", pod.Name, err)
		return nil
	}
	ec.eventRecorder.Eventf(e, v1.EventTypeNormal, EventReasonStop, "Stops pod for old engine %v", pod.Name)
	return nil
}

func (ec *EngineController) GetInstance(obj interface{}) (*longhorn.InstanceProcess, error) {
	e, ok := obj.(*longhorn.Engine)
	if !ok {
		return nil, fmt.Errorf("BUG: invalid object for engine process get: %v", obj)
	}

	var (
		im  *longhorn.InstanceManager
		err error
	)
	if e.Status.InstanceManagerName == "" {
		im, err = ec.ds.GetInstanceManagerByInstance(obj)
		if err != nil {
			return nil, err
		}
	} else {
		im, err = ec.ds.GetInstanceManager(e.Status.InstanceManagerName)
		if err != nil {
			return nil, err
		}
	}
	c, err := engineapi.NewInstanceManagerClient(im)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	return c.ProcessGet(e.Name)
}

func (ec *EngineController) LogInstance(ctx context.Context, obj interface{}) (*engineapi.InstanceManagerClient, *imapi.LogStream, error) {
	e, ok := obj.(*longhorn.Engine)
	if !ok {
		return nil, nil, fmt.Errorf("BUG: invalid object for engine process log: %v", obj)
	}

	im, err := ec.ds.GetInstanceManager(e.Status.InstanceManagerName)
	if err != nil {
		return nil, nil, err
	}
	c, err := engineapi.NewInstanceManagerClient(im)
	if err != nil {
		return nil, nil, err
	}

	// TODO: #2441 refactor this when we do the resource monitoring refactor
	stream, err := c.ProcessLog(ctx, e.Name)
	return c, stream, err
}

func (ec *EngineController) isMonitoring(e *longhorn.Engine) bool {
	ec.engineMonitorMutex.RLock()
	defer ec.engineMonitorMutex.RUnlock()

	_, ok := ec.engineMonitorMap[e.Name]
	return ok
}

func (ec *EngineController) startMonitoring(e *longhorn.Engine) {
	stopCh := make(chan struct{})
	monitorVoluntaryStopCh := make(chan struct{})
	monitor := &EngineMonitor{
		logger:                 ec.logger.WithField("engine", e.Name),
		Name:                   e.Name,
		namespace:              e.Namespace,
		ds:                     ec.ds,
		eventRecorder:          ec.eventRecorder,
		engines:                ec.engines,
		stopCh:                 stopCh,
		monitorVoluntaryStopCh: monitorVoluntaryStopCh,
		expansionBackoff:       flowcontrol.NewBackOff(time.Second*10, time.Minute*5),
		controllerID:           ec.controllerID,
		proxyConnCounter:       ec.proxyConnCounter,
	}

	ec.engineMonitorMutex.Lock()
	defer ec.engineMonitorMutex.Unlock()

	if _, ok := ec.engineMonitorMap[e.Name]; ok {
		ec.logger.Warnf("BUG: Monitoring for %v already exists", e.Name)
		return
	}
	ec.engineMonitorMap[e.Name] = stopCh

	go monitor.Run()

	go func() {
		<-monitorVoluntaryStopCh
		ec.engineMonitorMutex.Lock()
		delete(ec.engineMonitorMap, e.Name)
		ec.logger.WithField("engine", e.Name).Debug("removed the engine from ec.engineMonitorMap")
		ec.engineMonitorMutex.Unlock()
	}()
}

func (ec *EngineController) resetAndStopMonitoring(e *longhorn.Engine) {
	if _, err := ec.ds.ResetMonitoringEngineStatus(e); err != nil {
		utilruntime.HandleError(errors.Wrapf(err, "failed to update engine %v to stop monitoring", e.Name))
		// better luck next time
		return
	}

	ec.stopMonitoring(e.Name)

}

func (ec *EngineController) stopMonitoring(engineName string) {
	ec.engineMonitorMutex.Lock()
	defer ec.engineMonitorMutex.Unlock()

	stopCh, ok := ec.engineMonitorMap[engineName]
	if !ok {
		ec.logger.WithField("engine", engineName).Warn("Stop monitoring engine called even though there is no monitoring")
		return
	}

	select {
	case <-stopCh:
		// stopCh channel is already closed
	default:
		close(stopCh)
	}

}

func (m *EngineMonitor) Run() {
	m.logger.Debug("Start monitoring engine")
	defer func() {
		m.logger.Debug("Stop monitoring engine")
		close(m.monitorVoluntaryStopCh)
	}()

	ticker := time.NewTicker(EnginePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if needStop := m.sync(); needStop {
				return
			}
		case <-m.stopCh:
			return
		}
	}
}

func (m *EngineMonitor) sync() bool {
	for count := 0; count < EngineMonitorConflictRetryCount; count++ {
		engine, err := m.ds.GetEngine(m.Name)
		if err != nil {
			if datastore.ErrorIsNotFound(err) {
				m.logger.Info("stop monitoring because the engine no longer exists")
				return true
			}
			utilruntime.HandleError(errors.Wrapf(err, "fail to get engine %v for monitoring", m.Name))
			return false
		}

		if engine.Status.OwnerID != m.controllerID {
			m.logger.Infof("stop monitoring the engine on this node (%v) because the engine has new ownerID %v", m.controllerID, engine.Status.OwnerID)
			return true
		}

		// engine is maybe starting
		if engine.Status.CurrentState != longhorn.InstanceStateRunning {
			return false
		}

		// engine is upgrading
		if engine.Status.CurrentImage != engine.Spec.EngineImage || len(engine.Spec.UpgradedReplicaAddressMap) != 0 {
			return false
		}

		if err := m.refresh(engine); err == nil || !apierrors.IsConflict(errors.Cause(err)) {
			utilruntime.HandleError(errors.Wrapf(err, "failed to update status for engine %v", m.Name))
			break
		}
		// Retry if the error is due to conflict
	}

	return false
}

func (m *EngineMonitor) refresh(engine *longhorn.Engine) error {
	existingEngine := engine.DeepCopy()

	addressReplicaMap := map[string]string{}
	for replica, address := range engine.Status.CurrentReplicaAddressMap {
		if addressReplicaMap[address] != "" {
			return fmt.Errorf("invalid ReplicaAddressMap: duplicate addresses")
		}
		addressReplicaMap[address] = replica
	}

	engineCliClient, err := GetBinaryClientForEngine(engine, m.engines, engine.Status.CurrentImage)
	if err != nil {
		return err
	}

	engineClientProxy, err := engineapi.GetCompatibleClient(engine, engineCliClient, m.ds, m.logger, m.proxyConnCounter)
	if err != nil {
		return err
	}
	defer engineClientProxy.Close()

	replicaURLModeMap, err := engineClientProxy.ReplicaList(engine)
	if err != nil {
		return err
	}

	currentReplicaModeMap := map[string]longhorn.ReplicaMode{}
	for url, r := range replicaURLModeMap {
		addr := engineapi.GetAddressFromBackendReplicaURL(url)
		replica, exists := addressReplicaMap[addr]
		if !exists {
			// we have a entry doesn't exist in our spec
			replica = unknownReplicaPrefix + url

			// The unknown replica will remove and record the event during
			// ReconcileEngineState.
			// https://github.com/longhorn/longhorn/issues/4120
			currentReplicaModeMap[replica] = r.Mode
			m.logger.Warnf("Found unknown replica %v in the Replica URL Mode Map", addr)
			continue
		}

		currentReplicaModeMap[replica] = r.Mode

		if engine.Status.ReplicaModeMap != nil {
			if r.Mode != engine.Status.ReplicaModeMap[replica] {
				switch r.Mode {
				case longhorn.ReplicaModeERR:
					m.eventRecorder.Eventf(engine, v1.EventTypeWarning, EventReasonFaulted, "Detected replica %v (%v) in error", replica, addr)
				case longhorn.ReplicaModeWO:
					m.eventRecorder.Eventf(engine, v1.EventTypeNormal, EventReasonRebuilding, "Detected rebuilding replica %v (%v)", replica, addr)
				case longhorn.ReplicaModeRW:
					m.eventRecorder.Eventf(engine, v1.EventTypeNormal, EventReasonRebuilt, "Detected replica %v (%v) has been rebuilt", replica, addr)
				default:
					m.logger.Errorf("Invalid engine replica mode %v", r.Mode)
				}
			}
		}
	}
	engine.Status.ReplicaModeMap = currentReplicaModeMap

	snapshots, err := engineClientProxy.SnapshotList(engine)
	if err != nil {
		engine.Status.Snapshots = map[string]*longhorn.SnapshotInfo{}
		engine.Status.SnapshotsError = err.Error()
	} else {
		engine.Status.Snapshots = snapshots
		engine.Status.SnapshotsError = ""
	}

	// TODO: find a more advanced way to handle invocations for incompatible running engines
	cliAPIVersion, err := m.ds.GetEngineImageCLIAPIVersion(engine.Status.CurrentImage)
	if err != nil {
		return err
	}
	if cliAPIVersion >= engineapi.MinCLIVersion {
		volumeInfo, err := engineClientProxy.VolumeGet(engine)
		if err != nil {
			return err
		}
		endpoint, err := engineapi.GetEngineEndpoint(volumeInfo, engine.Status.IP)
		if err != nil {
			return err
		}
		engine.Status.Endpoint = endpoint

		if volumeInfo.LastExpansionError != "" && volumeInfo.LastExpansionFailedAt != "" &&
			(engine.Status.LastExpansionError != volumeInfo.LastExpansionError ||
				engine.Status.LastExpansionFailedAt != volumeInfo.LastExpansionFailedAt) {
			m.eventRecorder.Eventf(engine, v1.EventTypeWarning, EventReasonFailedExpansion,
				"Failed to expand the engine at %v: %v", volumeInfo.LastExpansionFailedAt, volumeInfo.LastExpansionError)
			m.expansionBackoff.Next(engine.Name, time.Now())
		}
		engine.Status.CurrentSize = volumeInfo.Size
		engine.Status.IsExpanding = volumeInfo.IsExpanding
		engine.Status.LastExpansionError = volumeInfo.LastExpansionError
		engine.Status.LastExpansionFailedAt = volumeInfo.LastExpansionFailedAt

		if engine.Status.Endpoint == "" && !engine.Spec.DisableFrontend && engine.Spec.Frontend != longhorn.VolumeFrontendEmpty {
			m.logger.Infof("Preparing to start frontend %v", engine.Spec.Frontend)
			if err := engineClientProxy.VolumeFrontendStart(engine); err != nil {
				return errors.Wrapf(err, "failed to start frontend %v", engine.Spec.Frontend)
			}
		}

		// The rebuild failure will be handled by ec.startRebuilding()
		rebuildStatus, err := engineClientProxy.ReplicaRebuildStatus(engine)
		if err != nil {
			return err
		}
		engine.Status.RebuildStatus = rebuildStatus
	} else {
		// For incompatible running engine, the current size is always `engine.Spec.VolumeSize`.
		engine.Status.CurrentSize = engine.Spec.VolumeSize
		engine.Status.RebuildStatus = map[string]*longhorn.RebuildStatus{}
	}

	// TODO: Check if the purge failure is handled somewhere else
	purgeStatus, err := engineClientProxy.SnapshotPurgeStatus(engine)
	if err != nil {
		m.logger.WithError(err).Error("failed to get snapshot purge status")
	} else {
		engine.Status.PurgeStatus = purgeStatus
	}

	removeInvalidEngineOpStatus(engine)

	// Make sure the engine object is updated before engineapi calls.
	if !reflect.DeepEqual(existingEngine.Status, engine.Status) {
		if engine, err = m.ds.UpdateEngineStatus(engine); err != nil {
			return err
		}
		existingEngine = engine.DeepCopy()
	}

	if cliAPIVersion >= engineapi.MinCLIVersion {
		// Cannot continue to start restoration if expansion is not complete
		if engine.Spec.VolumeSize > engine.Status.CurrentSize {
			// Cannot tolerate the rebuilding replicas during the expansion
			hasRebuildingReplicas := false
			for name, mode := range engine.Status.ReplicaModeMap {
				if mode == longhorn.ReplicaModeWO {
					engine.Status.ReplicaModeMap[name] = longhorn.ReplicaModeERR
					hasRebuildingReplicas = true
				}
			}
			if hasRebuildingReplicas {
				m.logger.Warn("cannot contain any rebuilding replicas during engine expansion, will mark all rebuilding replicas as ERROR")
				return nil
			}
			if !engine.Status.IsExpanding && !m.expansionBackoff.IsInBackOffSince(engine.Name, time.Now()) {
				m.logger.Infof("start engine expansion from %v to %v", engine.Status.CurrentSize, engine.Spec.VolumeSize)
				// The error info and the backoff interval will be updated later.
				if err := engineClientProxy.VolumeExpand(engine); err != nil {
					return err
				}
			}
			return nil
		}
		if engine.Spec.VolumeSize < engine.Status.CurrentSize {
			return fmt.Errorf("BUG: The expected size %v of engine %v should not be smaller than the current size %v", engine.Spec.VolumeSize, engine.Name, engine.Status.CurrentSize)
		}

		// engine.Spec.VolumeSize == engine.Status.CurrentSize.
		// This means expansion is complete/unnecessary, and it's safe to clean up the backoff entry if exists.
		m.expansionBackoff.DeleteEntry(engine.Name)
	}

	rsMap, err := engineClientProxy.BackupRestoreStatus(engine)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			m.logger.Warn(err)
			return
		}

		engine.Status.RestoreStatus = rsMap

		removeInvalidEngineOpStatus(engine)

		if !reflect.DeepEqual(existingEngine.Status, engine.Status) {
			e, updateErr := m.ds.UpdateEngineStatus(engine)
			if updateErr != nil {
				err = errors.Wrapf(err, "engine monitor: Failed to update the status for engine %v: %v", engine.Name, updateErr)
				return
			}
			engine = e
		}
	}()

	needRestore, err := preRestoreCheckAndSync(m.logger, engine, rsMap, addressReplicaMap, cliAPIVersion, m.ds, engineClientProxy)
	if err != nil {
		return err
	}

	// Incremental restoration will implicitly expand the DR volume once the backup volume is expanded
	if needRestore {
		if err = restoreBackup(m.logger, engine, rsMap, cliAPIVersion, m.ds, engineClientProxy); err != nil {
			return err
		}
	}

	var snapshotCloneStatusMap map[string]*longhorn.SnapshotCloneStatus
	if cliAPIVersion >= engineapi.CLIVersionFive {
		if snapshotCloneStatusMap, err = engineClientProxy.SnapshotCloneStatus(engine); err != nil {
			return err
		}
	}

	engine.Status.CloneStatus = snapshotCloneStatusMap

	needClone, err := preCloneCheck(engine)
	if err != nil {
		return err
	}
	if needClone {
		if err = cloneSnapshot(engine, engineClientProxy, m.ds); err != nil {
			return err
		}
	}

	return nil
}

func (ec *EngineController) syncSnapshotCRs(engine *longhorn.Engine) error {
	snapshotCRs, err := ec.ds.ListVolumeSnapshotsRO(engine.Spec.VolumeName)
	if err != nil {
		return err
	}
	if engine.Status.SnapshotsError != "" {
		return nil
	}

	for snapName, snapCR := range snapshotCRs {
		requestCreateNewSnapshot := snapCR.Spec.CreateSnapshot
		alreadyCreatedBefore := snapCR.Status.CreationTime != ""
		if _, ok := engine.Status.Snapshots[snapName]; !ok && (!requestCreateNewSnapshot || alreadyCreatedBefore) && snapCR.DeletionTimestamp == nil {
			ec.logger.Infof("Deleting snapshot CR for the snapshot %v", snapName)
			if err := ec.ds.DeleteSnapshot(snapName); err != nil && !apierrors.IsNotFound(err) {
				utilruntime.HandleError(fmt.Errorf("syncSnapshotCRs: failed to delete snapshot CR %v: %v", snapCR.Name, err))
			}
		}
	}

	volume, err := ec.ds.GetVolumeRO(engine.Spec.VolumeName)
	if err != nil {
		return err
	}

	for snapName := range engine.Status.Snapshots {
		// Don't create snapshot CR for the volume-head snapshot
		if snapName == "volume-head" {
			continue
		}

		if _, ok := snapshotCRs[snapName]; !ok {
			// snapshotCR does not exist, create a new one
			snapCR := &longhorn.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: snapName,
				},
				Spec: longhorn.SnapshotSpec{
					Volume:         volume.Name,
					CreateSnapshot: false,
				},
			}
			ec.logger.Infof("Creating snapshot CR for the snapshot %v", snapName)
			if _, err := ec.ds.CreateSnapshot(snapCR); err != nil && !apierrors.IsAlreadyExists(err) {
				utilruntime.HandleError(fmt.Errorf("syncSnapshotCRs: failed to create snapshot CR %v: %v", snapCR.Name, err))
			}
		}
	}

	return nil
}

func preRestoreCheckAndSync(log logrus.FieldLogger, engine *longhorn.Engine,
	rsMap map[string]*longhorn.RestoreStatus, addressReplicaMap map[string]string,
	cliAPIVersion int, ds *datastore.DataStore, engineClientProxy engineapi.EngineClientProxy) (needRestore bool, err error) {
	defer func() {
		if err != nil {
			err = errors.Wrapf(err, "failed pre-restore check for engine %v", engine.Name)
			// Need to manually update the restore status if the the check fails
			for _, status := range rsMap {
				status.Error = err.Error()
			}
			needRestore = false
		}
	}()

	if rsMap == nil {
		return false, nil
	}
	if cliAPIVersion < engineapi.CLIVersionFour {
		isRestoring, isConsensual := syncWithRestoreStatusForCompatibleEngine(log, engine, rsMap)

		if isRestoring || !isConsensual || engine.Spec.RequestedBackupRestore == "" || engine.Spec.RequestedBackupRestore == engine.Status.LastRestoredBackup {
			return false, nil
		}
	} else {
		if !syncWithRestoreStatus(log, engine, rsMap, addressReplicaMap, engineClientProxy) {
			return false, nil
		}
	}

	if engine.Spec.BackupVolume == "" {
		return false, fmt.Errorf("BUG: backup volume is empty for backup restoration of engine %v", engine.Name)
	}

	if cliAPIVersion >= engineapi.MinCLIVersion {
		return checkSizeBeforeRestoration(log, engine, ds)
	}

	return true, nil
}

func syncWithRestoreStatus(log logrus.FieldLogger, engine *longhorn.Engine, rsMap map[string]*longhorn.RestoreStatus,
	addressReplicaMap map[string]string, engineClientProxy engineapi.EngineClientProxy) bool {
	for _, status := range engine.Status.PurgeStatus {
		if status.IsPurging {
			return false
		}
	}

	for _, status := range rsMap {
		if status.Error != "" {
			log.WithError(errors.New(status.Error)).Warn("need to wait for the restore error handling before the restore invocation")
			return false
		}
	}

	allReplicasAreRestoring := true
	isConsensual := true
	lastRestoredInitialized := false
	lastRestored := ""
	for url, status := range rsMap {
		if !status.IsRestoring {
			allReplicasAreRestoring = false

			// Verify the rebuilding replica after the restore complete. This call will set the replica mode to RW.
			if status.LastRestored != "" {
				replicaName := addressReplicaMap[engineapi.GetAddressFromBackendReplicaURL(url)]
				if mode, exists := engine.Status.ReplicaModeMap[replicaName]; exists && mode == longhorn.ReplicaModeWO {
					if err := engineClientProxy.ReplicaRebuildVerify(engine, url); err != nil {
						log.WithError(err).Errorf("Failed to verify the rebuild of replica %v after restore completion", url)
						engine.Status.ReplicaModeMap[url] = longhorn.ReplicaModeERR
						return false
					}
					log.Infof("Verified the rebuild of replica %v after restore completion", url)
				}
			}
		}
		if !lastRestoredInitialized {
			lastRestored = status.LastRestored
			lastRestoredInitialized = true
		}
		if status.LastRestored != lastRestored {
			isConsensual = false
		}
	}
	if isConsensual {
		if engine.Status.LastRestoredBackup != lastRestored {
			log.Infof("Update last restored backup from %v to %v", engine.Status.LastRestoredBackup, lastRestored)
		}
		engine.Status.LastRestoredBackup = lastRestored
	} else {
		if engine.Status.LastRestoredBackup != "" {
			log.Debugf("Clean up the field LastRestoredBackup %v due to the inconsistency. Maybe it's caused by replica rebuilding", engine.Status.LastRestoredBackup)
		}
		engine.Status.LastRestoredBackup = ""
	}

	if engine.Spec.RequestedBackupRestore != "" && engine.Spec.RequestedBackupRestore != engine.Status.LastRestoredBackup && !allReplicasAreRestoring {
		return true
	}
	return false
}

func syncWithRestoreStatusForCompatibleEngine(log logrus.FieldLogger, engine *longhorn.Engine, rsMap map[string]*longhorn.RestoreStatus) (bool, bool) {
	isRestoring := false
	isConsensual := true
	lastRestored := ""
	for _, status := range rsMap {
		if status.IsRestoring {
			isRestoring = true
		}
	}
	// Engine is not restoring, pick the lastRestored from replica then update LastRestoredBackup
	if !isRestoring {
		for addr, status := range rsMap {
			if lastRestored != "" && status.LastRestored != lastRestored {
				// this error shouldn't prevent the engine from updating the other status
				log.Errorf("BUG: different lastRestored values expected %v actual %v even though engine is not restoring",
					lastRestored, status.LastRestored)
				isConsensual = false
			}
			if status.Error != "" {
				log.WithError(errors.New(status.Error)).Errorf("received restore error from replica %v", addr)
				isConsensual = false
			}
			lastRestored = status.LastRestored
		}
		if isConsensual {
			engine.Status.LastRestoredBackup = lastRestored
		}
	}
	return isRestoring, isConsensual
}

func checkSizeBeforeRestoration(log logrus.FieldLogger, engine *longhorn.Engine, ds *datastore.DataStore) (bool, error) {
	bv, err := ds.GetBackupVolumeRO(engine.Spec.BackupVolume)
	if err != nil {
		return false, err
	}
	// Need to wait for BackupVolume CR syncing with the remote backup target.
	if bv.Status.Size == "" {
		return false, nil
	}
	bvSize, err := strconv.ParseInt(bv.Status.Size, 10, 64)
	if err != nil {
		return false, err
	}

	for i := 0; i < ConflictRetryCount; i++ {
		v, err := ds.GetVolume(engine.Spec.VolumeName)
		if err != nil {
			return false, err
		}

		if bvSize < v.Spec.Size {
			return false, fmt.Errorf("engine monitor: BUG: the backup volume size %v is smaller than the size %v of the DR volume %v", bvSize, engine.Spec.VolumeSize, v.Name)
		} else if bvSize > v.Spec.Size {
			// TODO: Find a way to update volume.Spec.Size outside of the controller
			// The volume controller will update `engine.Spec.VolumeSize` later then trigger expansion call
			log.WithField("volume", v.Name).Infof("Prepare to expand the DR volume size from %v to %v", v.Spec.Size, bvSize)
			v.Spec.Size = bvSize
			if _, err := ds.UpdateVolume(v); err != nil {
				if !datastore.ErrorIsConflict(err) {
					return false, err
				}
				log.WithField("volume", v.Name).Debug("Retrying size update for DR volume before restore")
				continue
			}
			return false, nil
		}
	}

	return true, nil
}

func restoreBackup(log logrus.FieldLogger, engine *longhorn.Engine, rsMap map[string]*longhorn.RestoreStatus, cliAPIVersion int, ds *datastore.DataStore, engineClientProxy engineapi.EngineClientProxy) error {
	// Get default backup target
	backupTarget, err := ds.GetBackupTargetRO(types.DefaultBackupTargetName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return fmt.Errorf("cannot found the %s backup target", types.DefaultBackupTargetName)
	}

	backupTargetClient, err := getBackupTargetClient(ds, backupTarget)
	if err != nil {
		return errors.Wrapf(err, "cannot get backup target config for backup restoration of engine %v", engine.Name)
	}

	if cliAPIVersion < engineapi.CLIVersionFour {
		// For compatible engines, `LastRestoredBackup` is required to indicate if the restore is incremental restore
		log.Infof("Prepare to restore backup, backup target: %v, backup volume: %v, requested restored backup name: %v, last restored backup name: %v",
			backupTargetClient.URL, engine.Spec.BackupVolume, engine.Spec.RequestedBackupRestore, engine.Status.LastRestoredBackup)
		if err = engineClientProxy.BackupRestore(engine, backupTargetClient.URL, engine.Spec.RequestedBackupRestore, engine.Spec.BackupVolume, engine.Status.LastRestoredBackup, backupTargetClient.Credential); err != nil {
			if extraErr := handleRestoreErrorForCompatibleEngine(log, engine, rsMap, err); extraErr != nil {
				return extraErr
			}
		}
	} else {
		if err = engineClientProxy.BackupRestore(engine, backupTargetClient.URL, engine.Spec.RequestedBackupRestore, engine.Spec.BackupVolume, "", backupTargetClient.Credential); err != nil {
			log.Infof("Prepare to restore backup, backup target: %v, backup volume: %v, requested restored backup name: %v",
				backupTargetClient.URL, engine.Spec.BackupVolume, engine.Spec.RequestedBackupRestore)
			if extraErr := handleRestoreError(log, engine, rsMap, err); extraErr != nil {
				return extraErr
			}
		}
	}

	return nil
}

func handleRestoreError(log logrus.FieldLogger, engine *longhorn.Engine, rsMap map[string]*longhorn.RestoreStatus, err error) error {
	taskErr, ok := err.(imclient.TaskError)
	if !ok {
		return errors.Wrapf(err, "failed to restore backup %v in engine monitor, will retry the restore later",
			engine.Spec.RequestedBackupRestore)
	}

	for _, re := range taskErr.ReplicaErrors {
		if status, exists := rsMap[re.Address]; exists {
			if strings.Contains(re.Error(), "already in progress") || strings.Contains(re.Error(), "already restored backup") {
				log.WithError(re).Debugf("Ignore restore error from replica %v", re.Address)
				continue
			}
			status.Error = re.Error()
		}
	}

	return nil
}

func handleRestoreErrorForCompatibleEngine(log logrus.FieldLogger, engine *longhorn.Engine, rsMap map[string]*longhorn.RestoreStatus, err error) error {
	taskErr, ok := err.(imclient.TaskError)
	if !ok {
		return errors.Wrapf(err, "failed to restore backup %v with last restored backup %v in engine monitor",
			engine.Spec.RequestedBackupRestore, engine.Status.LastRestoredBackup)
	}

	for _, re := range taskErr.ReplicaErrors {
		if status, exists := rsMap[re.Address]; exists {
			status.Error = re.Error()
		}
	}
	log.WithError(taskErr).Warnf("Some replicas of the compatible engine failed to start restoring backup %v with last restored backup %v in engine monitor",
		engine.Spec.RequestedBackupRestore, engine.Status.LastRestoredBackup)

	return nil
}

func preCloneCheck(engine *longhorn.Engine) (needClone bool, err error) {
	if engine.Spec.RequestedDataSource == "" {
		return false, nil
	}
	for _, status := range engine.Status.CloneStatus {
		// Already in-cloning or finished cloning the snapshot
		if status != nil && status.State != "" {
			return false, nil
		}
	}
	return true, nil
}

func cloneSnapshot(engine *longhorn.Engine, engineClientProxy engineapi.EngineClientProxy, ds *datastore.DataStore) error {
	sourceVolumeName := types.GetVolumeName(engine.Spec.RequestedDataSource)
	snapshotName := types.GetSnapshotName(engine.Spec.RequestedDataSource)
	sourceEngines, err := ds.ListVolumeEngines(sourceVolumeName)
	if err != nil {
		return err
	}
	if len(sourceEngines) != 1 {
		return fmt.Errorf("failed to get engine for the source volume %v. The source volume has %v engines", sourceVolumeName, len(sourceEngines))
	}
	var sourceEngine *longhorn.Engine
	for _, e := range sourceEngines {
		sourceEngine = e
	}
	sourceEngineControllerURL := imutil.GetURL(sourceEngine.Status.StorageIP, sourceEngine.Status.Port)
	if err := engineClientProxy.SnapshotClone(engine, snapshotName, sourceEngineControllerURL); err != nil {
		// There is only 1 replica during volume cloning,
		// so if the cloning failed, it must be that the replica failed to clone.
		for _, status := range engine.Status.CloneStatus {
			status.Error = err.Error()
			status.State = engineapi.ProcessStateError
		}
		return err
	}
	return nil
}

func (ec *EngineController) ReconcileEngineState(e *longhorn.Engine) error {
	if err := ec.removeUnknownReplica(e); err != nil {
		return err
	}
	if err := ec.rebuildNewReplica(e); err != nil {
		return err
	}
	return nil
}

func GetBinaryClientForEngine(e *longhorn.Engine, engines engineapi.EngineClientCollection, image string) (client *engineapi.EngineBinary, err error) {
	defer func() {
		err = errors.Wrapf(err, "cannot get client for engine %v", e.Name)
	}()
	if e.Status.CurrentState != longhorn.InstanceStateRunning {
		return nil, fmt.Errorf("engine is not running")
	}
	if image == "" {
		return nil, fmt.Errorf("require specify engine image")
	}
	if e.Status.IP == "" || e.Status.Port == 0 {
		return nil, fmt.Errorf("require IP and Port")
	}

	client, err = engines.NewEngineClient(&engineapi.EngineClientRequest{
		VolumeName:  e.Spec.VolumeName,
		EngineImage: image,
		IP:          e.Status.IP,
		Port:        e.Status.Port,
	})
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (ec *EngineController) removeUnknownReplica(e *longhorn.Engine) error {
	unknownReplicaMap := map[string]longhorn.ReplicaMode{}
	for replica, mode := range e.Status.ReplicaModeMap {
		// unknown replicas have been named as `unknownReplicaPrefix-<replica URL>`
		if strings.HasPrefix(replica, unknownReplicaPrefix) {
			unknownReplicaMap[strings.TrimPrefix(replica, unknownReplicaPrefix)] = mode
		}
	}
	if len(unknownReplicaMap) == 0 {
		return nil
	}

	for url := range unknownReplicaMap {
		engineClientProxy, err := ec.getEngineClientProxy(e, e.Status.CurrentImage)
		if err != nil {
			return errors.Wrapf(err, "failed to get the engine client %v when removing unknown replica %v in mode %v from engine", e.Name, url, unknownReplicaMap[url])
		}

		go func(url string) {
			defer engineClientProxy.Close()

			ec.eventRecorder.Eventf(e, v1.EventTypeNormal, EventReasonDelete, "Removing unknown replica %v in mode %v from engine", url, unknownReplicaMap[url])
			if err := engineClientProxy.ReplicaRemove(e, url); err != nil {
				ec.eventRecorder.Eventf(e, v1.EventTypeWarning, EventReasonFailedDeleting, "Failed to remove unknown replica %v in mode %v from engine: %v", url, unknownReplicaMap[url], err)
			} else {
				ec.eventRecorder.Eventf(e, v1.EventTypeNormal, EventReasonDelete, "Removed unknown replica %v in mode %v from engine", url, unknownReplicaMap[url])
			}
		}(url)
	}
	return nil
}

func (ec *EngineController) rebuildNewReplica(e *longhorn.Engine) error {
	rebuildingInProgress := false
	replicaExists := make(map[string]bool)
	for replica, mode := range e.Status.ReplicaModeMap {
		replicaExists[replica] = true
		if mode == longhorn.ReplicaModeWO {
			rebuildingInProgress = true
			break
		}
	}
	// We cannot rebuild more than one replica at one time
	if rebuildingInProgress {
		ec.logger.WithField("volume", e.Spec.VolumeName).Debug("Skip rebuilding of replica because there is another rebuild in progress")
		return nil
	}
	for replica, addr := range e.Status.CurrentReplicaAddressMap {
		// one is enough
		if !replicaExists[replica] {
			return ec.startRebuilding(e, replica, addr)
		}
	}
	return nil
}

func doesAddressExistInEngine(e *longhorn.Engine, addr string, engineClientProxy engineapi.EngineClientProxy) (bool, error) {
	replicaURLModeMap, err := engineClientProxy.ReplicaList(e)
	if err != nil {
		return false, err
	}

	for url := range replicaURLModeMap {
		// the replica has been rebuilt or in the process already
		if addr == engineapi.GetAddressFromBackendReplicaURL(url) {
			return true, nil
		}
	}

	return false, nil
}

func (ec *EngineController) startRebuilding(e *longhorn.Engine, replica, addr string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "fail to start rebuild for %v of %v", replica, e.Name)
	}()

	log := ec.logger.WithFields(logrus.Fields{"volume": e.Spec.VolumeName, "engine": e.Name})

	engineClientProxy, err := ec.getEngineClientProxy(e, e.Status.CurrentImage)
	if err != nil {
		return err
	}
	defer engineClientProxy.Close()

	// we need to know the current status, since ReplicaAddressMap may
	// haven't been updated since last rebuild
	alreadyExists, err := doesAddressExistInEngine(e, addr, engineClientProxy)
	if err != nil {
		return err
	}
	// replica has already been added to the engine
	if alreadyExists {
		ec.logger.Debugf("replica %v address %v has been added to the engine already", replica, addr)
		return nil
	}

	replicaURL := engineapi.GetBackendReplicaURL(addr)
	go func() {
		autoCleanupSystemGeneratedSnapshot, err := ec.ds.GetSettingAsBool(types.SettingNameAutoCleanupSystemGeneratedSnapshot)
		if err != nil {
			log.WithError(err).Errorf("Failed to get %v setting", types.SettingDefinitionAutoCleanupSystemGeneratedSnapshot)
			return
		}

		engineClientProxy, err := ec.getEngineClientProxy(e, e.Status.CurrentImage)
		if err != nil {
			log.WithError(err).Errorf("Failed rebuilding of replica %v", addr)
			ec.eventRecorder.Eventf(e, v1.EventTypeWarning, EventReasonFailedRebuilding, "Failed rebuilding replica with Address %v: %v", addr, err)
			return
		}
		defer engineClientProxy.Close()

		// If enabled, call and wait for SnapshotPurge to clean up system generated snapshot before rebuilding.
		if autoCleanupSystemGeneratedSnapshot {
			if err := engineClientProxy.SnapshotPurge(e); err != nil {
				log.WithError(err).Error("Failed to start snapshot purge before rebuilding")
				ec.eventRecorder.Eventf(e, v1.EventTypeWarning, EventReasonFailedStartingSnapshotPurge, "Failed to start snapshot purge for engine %v and volume %v before rebuilding: %v", e.Name, e.Spec.VolumeName, err)
				return
			}
			logrus.Debug("Started snapshot purge before rebuilding, will wait for the purge complete")

			purgeDone := false
			endTime := time.Now().Add(time.Duration(purgeWaitIntervalInSecond) * time.Second)
			ticker := time.NewTicker(2 * EnginePollInterval)
			defer ticker.Stop()
			for !purgeDone && time.Now().Before(endTime) {
				<-ticker.C

				e, err := ec.ds.GetEngineRO(e.Name)
				if err != nil {
					if apierrors.IsNotFound(err) {
						log.Warn("Cannot continue waiting for the purge before rebuilding since engine is not found")
						return
					}
					log.WithError(err).Error("Failed to get engine and wait for the purge before rebuilding")
					continue
				}
				if e.Status.CurrentState != longhorn.InstanceStateRunning {
					log.Warnf("Cannot continue waiting for the purge before rebuilding since engine is state %v", e.Status.CurrentState)
					return
				}
				// Wait for purge complete
				purgeDone = true
				for _, purgeStatus := range e.Status.PurgeStatus {
					if purgeStatus.IsPurging {
						purgeDone = false
						break
					}
				}
			}
			if !purgeDone {
				log.Errorf("Timeout waiting for snapshot purge done before rebuilding, wait interval %v second", purgeWaitIntervalInSecond)
				ec.eventRecorder.Eventf(e, v1.EventTypeWarning, EventReasonTimeoutSnapshotPurge, "Timeout waiting for snapshot purge done before rebuilding volume %v, wait interval %v second", e.Spec.VolumeName, purgeWaitIntervalInSecond)
				return
			}
			log.Debug("Finished snapshot purge, will start rebuilding then")
		}

		// start rebuild
		if e.Spec.RequestedBackupRestore != "" {
			if e.Spec.NodeID != "" {
				ec.eventRecorder.Eventf(e, v1.EventTypeNormal, EventReasonRebuilding, "Start rebuilding replica %v with Address %v for restore engine %v and volume %v", replica, addr, e.Name, e.Spec.VolumeName)
				err = engineClientProxy.ReplicaAdd(e, replicaURL, true)
			}
		} else {
			ec.eventRecorder.Eventf(e, v1.EventTypeNormal, EventReasonRebuilding, "Start rebuilding replica %v with Address %v for normal engine %v and volume %v", replica, addr, e.Name, e.Spec.VolumeName)
			err = engineClientProxy.ReplicaAdd(e, replicaURL, false)
		}
		if err != nil {
			log.WithError(err).Errorf("Failed rebuilding of replica %v", addr)
			ec.eventRecorder.Eventf(e, v1.EventTypeWarning, EventReasonFailedRebuilding, "Failed rebuilding replica with Address %v: %v", addr, err)
			// we've sent out event to notify user. we don't want to
			// automatically handle it because it may cause chain
			// reaction to create numerous new replicas if we set
			// the replica to failed.
			// user can decide to delete it then we will try again
			if err := engineClientProxy.ReplicaRemove(e, replicaURL); err != nil {
				log.WithError(err).Errorf("Failed to remove rebuilding replica %v", addr)
				ec.eventRecorder.Eventf(e, v1.EventTypeWarning, EventReasonFailedDeleting,
					"Failed to remove rebuilding replica %v with address %v for engine %v and volume %v due to rebuilding failure: %v", replica, addr, e.Name, e.Spec.VolumeName, err)
			} else {
				log.Infof("Removed failed rebuilding replica %v", addr)
			}
			// Before we mark the Replica as Failed automatically, we want to check the Backoff to avoid recreating new
			// Replicas too quickly. If the Replica is still in the Backoff period, we will leave the Replica alone. If
			// it is past the Backoff period, we'll try to mark the Replica as Failed and increase the Backoff period
			// for the next failure.
			if !ec.backoff.IsInBackOffSinceUpdate(e.Name, time.Now()) {
				rep, err := ec.ds.GetReplica(replica)
				if err != nil {
					log.WithError(err).Errorf("Failed to get replica %v unable to mark failed rebuild", replica)
					return
				}
				rep.Spec.FailedAt = util.Now()
				rep.Spec.DesireState = longhorn.InstanceStateStopped
				if _, err := ec.ds.UpdateReplica(rep); err != nil {
					log.WithError(err).Errorf("Unable to mark failed rebuild on replica %v", replica)
					return
				}
				// Now that the Replica can actually be recreated, we can move up the Backoff.
				ec.backoff.Next(e.Name, time.Now())
				backoffTime := ec.backoff.Get(e.Name).Seconds()
				log.Infof("Marked failed rebuild on replica %v, backoff period is now %v seconds", replica, backoffTime)
				return
			}
			log.Infof("Engine is still in backoff for replica %v rebuild failure", replica)
			return
		}
		// Replica rebuild succeeded, clear Backoff.
		ec.backoff.DeleteEntry(e.Name)
		ec.eventRecorder.Eventf(e, v1.EventTypeNormal, EventReasonRebuilt,
			"Replica %v with Address %v has been rebuilt for volume %v", replica, addr, e.Spec.VolumeName)

		// If enabled, call SnapshotPurge to clean up system generated snapshot after rebuilding.
		if autoCleanupSystemGeneratedSnapshot {
			if err := engineClientProxy.SnapshotPurge(e); err != nil {
				log.WithError(err).Error("Failed to start snapshot purge after rebuilding")
				ec.eventRecorder.Eventf(e, v1.EventTypeWarning, EventReasonFailedStartingSnapshotPurge, "Failed to start snapshot purge for engine %v and volume %v after rebuilding: %v", e.Name, e.Spec.VolumeName, err)
				return
			}
			logrus.Debug("Started snapshot purge after rebuilding")
		}
	}()

	// Wait until engine confirmed that rebuild started
	if err := wait.PollImmediate(EnginePollInterval, EnginePollTimeout, func() (bool, error) {
		return doesAddressExistInEngine(e, addr, engineClientProxy)
	}); err != nil {
		return err
	}
	return nil
}

func (ec *EngineController) Upgrade(e *longhorn.Engine) (err error) {
	defer func() {
		err = errors.Wrapf(err, "cannot live upgrade image for %v", e.Name)
	}()

	log := ec.logger.WithField("volume", e.Spec.VolumeName)

	engineClientProxy, err := ec.getEngineClientProxy(e, e.Spec.EngineImage)
	if err != nil {
		return err
	}
	defer engineClientProxy.Close()

	version, err := engineClientProxy.VersionGet(e, false)
	if err != nil {
		return err
	}

	// Don't use image with different image name but same commit here. It
	// will cause live replica to be removed. Volume controller should filter those.
	if version.ClientVersion.GitCommit != version.ServerVersion.GitCommit {
		log.Debugf("Try to upgrade engine from %v to %v",
			e.Status.CurrentImage, e.Spec.EngineImage)
		if err := ec.UpgradeEngineProcess(e); err != nil {
			return err
		}
	}
	log.Infof("Engine has been upgraded from %v to %v", e.Status.CurrentImage, e.Spec.EngineImage)
	e.Status.CurrentImage = e.Spec.EngineImage
	e.Status.CurrentReplicaAddressMap = e.Spec.UpgradedReplicaAddressMap
	// reset ReplicaModeMap to reflect the new replicas
	e.Status.ReplicaModeMap = nil
	e.Status.RestoreStatus = nil
	e.Status.RebuildStatus = nil
	return nil
}

func (ec *EngineController) UpgradeEngineProcess(e *longhorn.Engine) error {
	frontend := e.Spec.Frontend
	if e.Spec.DisableFrontend {
		frontend = longhorn.VolumeFrontendEmpty
	}

	im, err := ec.ds.GetInstanceManager(e.Status.InstanceManagerName)
	if err != nil {
		return err
	}
	c, err := engineapi.NewInstanceManagerClient(im)
	if err != nil {
		return err
	}
	defer c.Close()

	engineProcess, err := c.EngineProcessUpgrade(e.Name, e.Spec.VolumeName, e.Spec.EngineImage, frontend, e.Spec.UpgradedReplicaAddressMap)
	if err != nil {
		return err
	}

	e.Status.Port = int(engineProcess.Status.PortStart)
	return nil
}

// isResponsibleFor picks a running node that has e.Status.CurrentImage deployed.
// We need e.Status.CurrentImage deployed on the node to make request to the corresponding engine process.
// Prefer picking the node e.Spec.NodeID if it meet the above requirement.
func (ec *EngineController) isResponsibleFor(e *longhorn.Engine, defaultEngineImage string) (bool, error) {
	var err error
	defer func() {
		err = errors.Wrap(err, "error while checking isResponsibleFor")
	}()

	isResponsible := isControllerResponsibleFor(ec.controllerID, ec.ds, e.Name, e.Spec.NodeID, e.Status.OwnerID)

	// The engine is not running, the owner node doesn't need to have e.Status.CurrentImage
	// Fall back to the default logic where we pick a running node to be the owner
	if e.Status.CurrentImage == "" {
		return isResponsible, nil
	}

	readyNodesWithEI, err := ec.ds.ListReadyNodesWithEngineImage(e.Status.CurrentImage)
	if err != nil {
		return false, err
	}
	// No node in the system has the e.Status.CurrentImage,
	// Fall back to the default logic where we pick a running node to be the owner
	if len(readyNodesWithEI) == 0 {
		return isResponsible, nil
	}

	preferredOwnerEngineAvailable, err := ec.ds.CheckEngineImageReadiness(e.Status.CurrentImage, e.Spec.NodeID)
	if err != nil {
		return false, err
	}
	currentOwnerEngineAvailable, err := ec.ds.CheckEngineImageReadiness(e.Status.CurrentImage, e.Status.OwnerID)
	if err != nil {
		return false, err
	}
	currentNodeEngineAvailable, err := ec.ds.CheckEngineImageReadiness(e.Status.CurrentImage, ec.controllerID)
	if err != nil {
		return false, err
	}

	isPreferredOwner := currentNodeEngineAvailable && isResponsible
	continueToBeOwner := currentNodeEngineAvailable && !preferredOwnerEngineAvailable && ec.controllerID == e.Status.OwnerID
	requiresNewOwner := currentNodeEngineAvailable && !preferredOwnerEngineAvailable && !currentOwnerEngineAvailable

	return isPreferredOwner || continueToBeOwner || requiresNewOwner, nil
}

func removeInvalidEngineOpStatus(e *longhorn.Engine) {
	tcpReplicaAddrMap := map[string]struct{}{}
	for _, addr := range e.Status.CurrentReplicaAddressMap {
		tcpReplicaAddrMap[engineapi.GetBackendReplicaURL(addr)] = struct{}{}
	}
	for tcpAddr := range e.Status.PurgeStatus {
		if _, exists := tcpReplicaAddrMap[tcpAddr]; !exists {
			delete(e.Status.PurgeStatus, tcpAddr)
		}
	}
	for tcpAddr := range e.Status.RestoreStatus {
		if _, exists := tcpReplicaAddrMap[tcpAddr]; !exists {
			delete(e.Status.RestoreStatus, tcpAddr)
		}
	}
	for tcpAddr := range e.Status.RebuildStatus {
		if _, exists := tcpReplicaAddrMap[tcpAddr]; !exists {
			delete(e.Status.RebuildStatus, tcpAddr)
		}
	}
	for tcpAddr := range e.Status.CloneStatus {
		if _, exists := tcpReplicaAddrMap[tcpAddr]; !exists {
			delete(e.Status.CloneStatus, tcpAddr)
		}
	}
}
