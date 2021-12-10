package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/longhorn/longhorn-instance-manager/pkg/api"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	lhinformers "github.com/longhorn/longhorn-manager/k8s/pkg/client/informers/externalversions/longhorn/v1beta1"
)

var (
	hostToContainer = v1.MountPropagationHostToContainer
)

type InstanceManagerController struct {
	*baseController

	namespace      string
	controllerID   string
	serviceAccount string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	imStoreSynced cache.InformerSynced
	pStoreSynced  cache.InformerSynced
	knStoreSynced cache.InformerSynced

	instanceManagerMonitorMutex *sync.RWMutex
	instanceManagerMonitorMap   map[string]chan struct{}

	// for unit test
	versionUpdater func(*longhorn.InstanceManager) error
}

type InstanceManagerMonitor struct {
	logger logrus.FieldLogger

	Name         string
	controllerID string

	instanceManagerUpdater  *InstanceManagerUpdater
	instanceManagerNotifier *InstanceManagerNotifier
	ds                      *datastore.DataStore
	lock                    *sync.RWMutex
	updateNotification      bool
	stopCh                  chan struct{}
	done                    bool
	// used to notify the controller that monitoring has stopped
	monitorVoluntaryStopCh chan struct{}
}

type InstanceManagerUpdater struct {
	client *engineapi.InstanceManagerClient
}

type InstanceManagerNotifier struct {
	stream *api.ProcessStream
}

func updateInstanceManagerVersion(im *longhorn.InstanceManager) error {
	cli, err := engineapi.NewInstanceManagerClient(im)
	if err != nil {
		return err
	}
	apiMinVersion, apiVersion, err := cli.VersionGet()
	if err != nil {
		return err
	}
	im.Status.APIMinVersion = apiMinVersion
	im.Status.APIVersion = apiVersion
	return nil
}

func NewInstanceManagerController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	imInformer lhinformers.InstanceManagerInformer,
	pInformer coreinformers.PodInformer,
	kubeNodeInformer coreinformers.NodeInformer,
	kubeClient clientset.Interface,
	namespace, controllerID, serviceAccount string) *InstanceManagerController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events("")})

	imc := &InstanceManagerController{
		baseController: newBaseController("longhorn-instance-manager", logger),

		namespace:      namespace,
		controllerID:   controllerID,
		serviceAccount: serviceAccount,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-instance-manager-controller"}),

		ds: ds,

		imStoreSynced: imInformer.Informer().HasSynced,
		pStoreSynced:  pInformer.Informer().HasSynced,
		knStoreSynced: kubeNodeInformer.Informer().HasSynced,

		instanceManagerMonitorMutex: &sync.RWMutex{},
		instanceManagerMonitorMap:   map[string]chan struct{}{},

		versionUpdater: updateInstanceManagerVersion,
	}

	imInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    imc.enqueueInstanceManager,
		UpdateFunc: func(old, cur interface{}) { imc.enqueueInstanceManager(cur) },
		DeleteFunc: imc.enqueueInstanceManager,
	})

	pInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: isInstanceManagerPod,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    imc.enqueueInstanceManagerPod,
			UpdateFunc: func(old, cur interface{}) { imc.enqueueInstanceManagerPod(cur) },
			DeleteFunc: imc.enqueueInstanceManagerPod,
		},
	})

	kubeNodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, cur interface{}) { imc.enqueueKubernetesNode(cur) },
		DeleteFunc: imc.enqueueKubernetesNode,
	})

	return imc
}

func isInstanceManagerPod(obj interface{}) bool {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		deletedState, ok := obj.(*cache.DeletedFinalStateUnknown)
		if !ok {
			return false
		}

		// use the last known state, to enqueue, dependent objects
		pod, ok = deletedState.Obj.(*v1.Pod)
		if !ok {
			return false
		}
	}

	for _, con := range pod.Spec.Containers {
		if con.Name == "engine-manager" || con.Name == "replica-manager" {
			return true
		}
	}
	return false
}

func (imc *InstanceManagerController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer imc.queue.ShutDown()

	logrus.Infof("Starting Longhorn instance manager controller")
	defer logrus.Infof("Shutting down Longhorn instance manager controller")

	if !cache.WaitForNamedCacheSync("longhorn instance manager", stopCh, imc.imStoreSynced, imc.pStoreSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(imc.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (imc *InstanceManagerController) worker() {
	for imc.processNextWorkItem() {
	}
}

func (imc *InstanceManagerController) processNextWorkItem() bool {
	key, quit := imc.queue.Get()

	if quit {
		return false
	}
	defer imc.queue.Done(key)

	err := imc.syncInstanceManager(key.(string))
	imc.handleErr(err, key)

	return true
}

func (imc *InstanceManagerController) handleErr(err error, key interface{}) {
	if err == nil {
		imc.queue.Forget(key)
		return
	}

	if imc.queue.NumRequeues(key) < maxRetries {
		logrus.Warnf("Error syncing Longhorn instance manager %v: %v", key, err)
		imc.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	logrus.Warnf("Dropping Longhorn instance manager %v out of the queue: %v", key, err)
	imc.queue.Forget(key)
}

func getLoggerForInstanceManager(logger logrus.FieldLogger, im *longhorn.InstanceManager) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"instanceManager": im.Name,
			"nodeID":          im.Spec.NodeID,
		},
	)
}

func (imc *InstanceManagerController) syncInstanceManager(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "fail to sync instance manager for %v", key)
	}()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != imc.namespace {
		return nil
	}

	im, err := imc.ds.GetInstanceManager(name)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			logrus.Infof("Longhorn instance manager %v has been deleted", key)
			return nil
		}
		return err
	}

	log := getLoggerForInstanceManager(imc.logger, im)

	if !imc.isResponsibleFor(im) {
		return nil
	}

	if im.Status.OwnerID != imc.controllerID {
		im.Status.OwnerID = imc.controllerID
		im, err = imc.ds.UpdateInstanceManagerStatus(im)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
		log.Debugf("Instance Manager Controller %v picked up %v", imc.controllerID, im.Name)
	}

	if im.DeletionTimestamp != nil {
		return nil
	}

	existingIM := im.DeepCopy()
	defer func() {
		if err == nil && !reflect.DeepEqual(existingIM.Status, im.Status) {
			_, err = imc.ds.UpdateInstanceManagerStatus(im)
		}
		if apierrors.IsConflict(errors.Cause(err)) {
			log.Debugf("Requeue %v due to conflict: %v", key, err)
			imc.enqueueInstanceManager(im)
			err = nil
		}
	}()

	pod, err := imc.ds.GetPod(im.Name)
	if err != nil {
		return errors.Wrapf(err, "cannot get pod for instance manager %v", im.Name)
	}

	isDown, err := imc.ds.IsNodeDownOrDeleted(im.Spec.NodeID)
	if err != nil {
		log.WithError(err).Warnf("cannot check IsNodeDownOrDeleted(%v) when syncInstanceManager ", im.Spec.NodeID)
	}
	if isDown {
		im.Status.CurrentState = longhorn.InstanceManagerStateUnknown
		if err := imc.cleanupInstanceManager(im); err != nil {
			return err
		}
		return nil
	}

	// Sync instance manager status with related pod
	if pod == nil {
		if im.Status.CurrentState != longhorn.InstanceManagerStateStopped {
			im.Status.CurrentState = longhorn.InstanceManagerStateError
		}
	} else if pod.DeletionTimestamp != nil {
		if im.Status.CurrentState != longhorn.InstanceManagerStateError {
			im.Status.CurrentState = longhorn.InstanceManagerStateError
		}
	} else {
		// TODO: Will remove this reference kind correcting after all Longhorn components having used the new kinds
		if len(pod.OwnerReferences) < 1 || pod.OwnerReferences[0].Kind != types.LonghornKindInstanceManager {
			pod.OwnerReferences = datastore.GetOwnerReferencesForInstanceManager(im)
			pod, err = imc.kubeClient.CoreV1().Pods(imc.namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}

		switch pod.Status.Phase {
		case v1.PodPending:
			if im.Status.CurrentState != longhorn.InstanceManagerStateStarting {
				log.Errorf("Instance Manager %v is state %v but the related pod is pending.", im.Name, im.Status.CurrentState)
				im.Status.CurrentState = longhorn.InstanceManagerStateError
			}
		case v1.PodRunning:
			// Make sure readiness probe has passed.
			for _, st := range pod.Status.ContainerStatuses {
				if !st.Ready {
					return nil
				}
			}

			if im.Status.CurrentState == longhorn.InstanceManagerStateStarting || im.Status.CurrentState == longhorn.InstanceManagerStateUnknown {
				im.Status.CurrentState = longhorn.InstanceManagerStateRunning
			} else if im.Status.CurrentState != longhorn.InstanceManagerStateRunning {
				im.Status.CurrentState = longhorn.InstanceManagerStateError
			}

			if im.Status.CurrentState == longhorn.InstanceManagerStateRunning {
				im.Status.IP = pod.Status.PodIP
			}
		default:
			im.Status.CurrentState = longhorn.InstanceManagerStateError
		}
	}

	if im.Status.CurrentState == longhorn.InstanceManagerStateRunning {
		if im.Status.APIVersion == engineapi.UnknownInstanceManagerAPIVersion {
			if err := imc.versionUpdater(im); err != nil {
				return err
			}
		}
	} else {
		im.Status.APIVersion = engineapi.UnknownInstanceManagerAPIVersion
		im.Status.APIMinVersion = engineapi.UnknownInstanceManagerAPIVersion
	}

	if err := imc.syncInstanceManagerPDB(im); err != nil {
		return err
	}

	if im.Status.CurrentState == longhorn.InstanceManagerStateRunning {
		if err := engineapi.CheckInstanceManagerCompatibilty(im.Status.APIMinVersion, im.Status.APIVersion); err != nil {
			if imc.isMonitoring(im.Name) {
				log.Infof("Instance manager controller will stop monitoring the incompatible instance manager %v", im.Name)
				imc.stopMonitoring(im)
			}
		} else {
			if !imc.isMonitoring(im.Name) {
				switch im.Spec.Type {
				case longhorn.InstanceManagerTypeEngine:
					fallthrough
				case longhorn.InstanceManagerTypeReplica:
					imc.startMonitoring(im, NewInstanceManagerUpdater(im))
				default:
					log.Errorf("BUG: instance manager %v has invalid type %v", im.Name, im.Spec.Type)
					im.Status.CurrentState = longhorn.InstanceManagerStateError
				}
			}
		}
	}

	// Restart instance manager automatically. Otherwise users need to take care of it then we need to expose API
	// for operating instance manager.
	if im.Status.CurrentState == longhorn.InstanceManagerStateError || im.Status.CurrentState == longhorn.InstanceManagerStateStopped {
		if err := imc.cleanupInstanceManager(im); err != nil {
			return err
		}
		// When user taints a node with NoExecute effect and doesn't set the corresponding toleration for Longhorn,
		// both Longhorn manager and IM pod are evicted. In this case we shouldn't repeatedly create the IM pod on that node.
		// In order to do so, we make sure that only create the IM pod if this controller is the same as the im.Spec.NodeID
		if imc.controllerID == im.Spec.NodeID {
			if err := imc.createInstanceManagerPod(im); err != nil {
				if !types.ErrorAlreadyExists(errors.Cause(err)) {
					return err
				}
				return nil
			}
			im.Status.CurrentState = longhorn.InstanceManagerStateStarting
			return nil
		}
	}

	return nil
}

func (imc *InstanceManagerController) syncInstanceManagerPDB(im *longhorn.InstanceManager) error {
	if err := imc.cleanUpPDBOfNonExistingIMPod(); err != nil {
		return err
	}

	if im.Status.CurrentState != longhorn.InstanceManagerStateRunning {
		return nil
	}

	unschedulable, err := imc.isNodeUnschedulable()
	if err != nil {
		return err
	}

	imPDB, err := imc.ds.GetPDBRO(imc.getPDBName(im))
	if err != nil && !datastore.ErrorIsNotFound(err) {
		return err
	}

	// When current node is unschedulable, it is a signal that the node is being cordoned/drained.
	if unschedulable {
		if imPDB == nil {
			return nil
		}
		canDelete, err := imc.canDeleteInstanceManagerPDB(im)
		if err != nil {
			return err
		}
		if !canDelete {
			return nil
		}
		return imc.deleteInstanceManagerPDB(im)
	}

	// Make sure that there is a PodDisruptionBudget to protect this instance manager in normal case.
	if imPDB == nil {
		return imc.createInstanceManagerPDB(im)
	}

	return nil
}

func (imc *InstanceManagerController) cleanUpPDBOfNonExistingIMPod() error {
	ims, err := imc.ds.ListInstanceManagers()
	if err != nil {
		if !datastore.ErrorIsNotFound(err) {
			return err
		}
		ims = make(map[string]*longhorn.InstanceManager)
	}

	imPDBs, err := imc.ds.ListPDBs()
	if err != nil {
		if !datastore.ErrorIsNotFound(err) {
			return err
		}
		imPDBs = make(map[string]*policyv1beta1.PodDisruptionBudget)
	}

	for pdbName, pdb := range imPDBs {
		if pdb.Spec.Selector == nil || pdb.Spec.Selector.MatchLabels == nil {
			continue
		}
		labelValue, ok := pdb.Spec.Selector.MatchLabels[types.GetLonghornLabelComponentKey()]
		if !ok {
			continue
		}
		if labelValue != types.LonghornLabelInstanceManager {
			continue
		}
		if _, ok := ims[getIMNameFromPDBName(pdbName)]; ok {
			continue
		}
		if err := imc.ds.DeletePDB(pdbName); err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func (imc *InstanceManagerController) isNodeUnschedulable() (bool, error) {
	kubeNode, err := imc.ds.GetKubernetesNode(imc.controllerID)
	if err != nil {
		return false, err
	}
	return kubeNode.Spec.Unschedulable, nil
}

func (imc *InstanceManagerController) deleteInstanceManagerPDB(im *longhorn.InstanceManager) error {
	err := imc.ds.DeletePDB(imc.getPDBName(im))
	if err != nil && !datastore.ErrorIsNotFound(err) {
		return err
	}
	return nil
}

func (imc *InstanceManagerController) canDeleteInstanceManagerPDB(im *longhorn.InstanceManager) (bool, error) {
	// If there is no engine instance process inside the engine instance manager,
	// it means that all volumes are detached.
	// We can delete the PodDisruptionBudget for the engine instance manager.
	if im.Spec.Type == longhorn.InstanceManagerTypeEngine {
		if len(im.Status.Instances) == 0 {
			return true, nil
		}
		return false, nil
	}

	// Make sure that the instance manager is of type replica
	if im.Spec.Type != longhorn.InstanceManagerTypeReplica {
		return false, fmt.Errorf("the instance manager %v has invalid type: %v ", im.Name, im.Spec.Type)
	}

	// Must wait for all volumes detached from the current node first.
	// This also means that we must wait until the the PDB of engine instance manager on the current node is deleted
	allVolumeDetached, err := imc.areAllVolumesDetachedFromCurrentNode()
	if err != nil {
		return false, err
	}
	if !allVolumeDetached {
		return false, nil
	}

	allowDrainingNodeWithLastReplica, err := imc.ds.GetSettingAsBool(types.SettingNameAllowNodeDrainWithLastHealthyReplica)
	if err != nil {
		return false, err
	}
	if allowDrainingNodeWithLastReplica {
		return true, nil
	}

	replicasOnCurrentNode, err := imc.ds.ListReplicasByNodeRO(imc.controllerID)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			return true, nil
		}
		return false, err
	}

	// For each replica process in the current node,
	// find out whether there is a healthy replica of the same volume on another node
	for _, replica := range replicasOnCurrentNode {
		vol, err := imc.ds.GetVolume(replica.Spec.VolumeName)
		if err != nil {
			return false, err
		}

		replicas, err := imc.ds.ListVolumeReplicas(vol.Name)
		if err != nil {
			return false, err
		}

		hasHealthyReplicaOnAnotherNode := false
		isUnusedReplicaOnCurrentNode := false
		for _, r := range replicas {
			if r.Spec.HealthyAt != "" && r.Spec.FailedAt == "" && r.Spec.NodeID != imc.controllerID {
				hasHealthyReplicaOnAnotherNode = true
				break
			}
			// If a replica has never been started, there is no data stored in this replica, and
			// retaining it makes no sense for HA.
			// Hence Longhorn doesn't need to block the PDB removal for the replica.
			// This case typically happens on a newly created volume that hasn't been attached to any node.
			// https://github.com/longhorn/longhorn/issues/2673
			if r.Spec.HealthyAt == "" && r.Spec.FailedAt == "" && r.Spec.NodeID == imc.controllerID {
				isUnusedReplicaOnCurrentNode = true
				break
			}
		}

		if !hasHealthyReplicaOnAnotherNode && !isUnusedReplicaOnCurrentNode {
			return false, nil
		}
	}

	return true, nil
}

func (imc *InstanceManagerController) areAllVolumesDetachedFromCurrentNode() (bool, error) {
	engineIMs, err := imc.ds.ListInstanceManagersByNode(imc.controllerID, longhorn.InstanceManagerTypeEngine)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	for _, engineIM := range engineIMs {
		if len(engineIM.Status.Instances) > 0 {
			return false, nil
		}
	}

	return true, nil
}

func (imc *InstanceManagerController) createInstanceManagerPDB(im *longhorn.InstanceManager) error {
	instanceManagerPDB := imc.generateInstanceManagerPDBManifest(im)
	if _, err := imc.ds.CreatePDB(instanceManagerPDB); err != nil {
		if apierrors.IsAlreadyExists(err) {
			imc.logger.Warn("PDB %s is already exists", instanceManagerPDB.GetName())
			return nil
		}
		return err
	}
	return nil
}

func (imc *InstanceManagerController) generateInstanceManagerPDBManifest(im *longhorn.InstanceManager) *policyv1beta1.PodDisruptionBudget {
	return &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      imc.getPDBName(im),
			Namespace: imc.namespace,
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: types.GetInstanceManagerLabels(imc.controllerID, im.Spec.Image, im.Spec.Type),
			},
			MinAvailable: &intstr.IntOrString{IntVal: 1},
		},
	}
}

func (imc *InstanceManagerController) getPDBName(im *longhorn.InstanceManager) string {
	return getPDBNameFromIMName(im.Name)
}

func getPDBNameFromIMName(imName string) string {
	return imName
}

func getIMNameFromPDBName(pdbName string) string {
	return pdbName
}

func (imc *InstanceManagerController) enqueueInstanceManager(instanceManager interface{}) {
	key, err := controller.KeyFunc(instanceManager)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", instanceManager, err))
		return
	}

	imc.queue.AddRateLimited(key)
}

func (imc *InstanceManagerController) enqueueInstanceManagerPod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		deletedState, ok := obj.(*cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		pod, ok = deletedState.Obj.(*v1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	im, err := imc.ds.GetInstanceManager(pod.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Warnf("Can't find instance manager for pod %v, may be deleted", pod.Name)
			return
		}
		utilruntime.HandleError(fmt.Errorf("couldn't get instance manager: %v", err))
		return
	}
	imc.enqueueInstanceManager(im)
}

func (imc *InstanceManagerController) enqueueKubernetesNode(obj interface{}) {
	kubernetesNode, ok := obj.(*v1.Node)
	if !ok {
		deletedState, ok := obj.(*cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		kubernetesNode, ok = deletedState.Obj.(*v1.Node)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	node, err := imc.ds.GetNode(kubernetesNode.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// there is no Longhorn node created for the Kubernetes
			// node (e.g. controller/etcd node). Skip it
			return
		}
		utilruntime.HandleError(fmt.Errorf("Couldn't get node %v: %v ", kubernetesNode.Name, err))
		return
	}

	for _, imType := range []longhorn.InstanceManagerType{longhorn.InstanceManagerTypeEngine, longhorn.InstanceManagerTypeReplica} {
		ims, err := imc.ds.ListInstanceManagersByNode(node.Name, imType)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrus.Warnf("Can't find instance manager for node %v, may be deleted", node.Name)
				return
			}
			utilruntime.HandleError(fmt.Errorf("couldn't get instance manager: %v", err))
			return
		}

		for _, im := range ims {
			imc.enqueueInstanceManager(im)
		}
	}
}

func (imc *InstanceManagerController) cleanupInstanceManager(im *longhorn.InstanceManager) error {
	log := getLoggerForInstanceManager(imc.logger, im)
	im.Status.IP = ""
	if len(im.Status.Instances) > 0 {
		im.Status.Instances = map[string]longhorn.InstanceProcess{}
	}
	if imc.isMonitoring(im.Name) {
		imc.stopMonitoring(im)
	}

	pod, err := imc.ds.GetPod(im.Name)
	if err != nil {
		return err
	}
	if pod != nil && pod.DeletionTimestamp == nil {
		if err := imc.ds.DeletePod(pod.Name); err != nil {
			return err
		}
		log.Warnf("Deleted instance manager pod %v because the instance manager has CurrentState %v", pod.Name, im.Status.CurrentState)
	}

	return nil
}

func (imc *InstanceManagerController) createInstanceManagerPod(im *longhorn.InstanceManager) error {
	tolerations, err := imc.ds.GetSettingTaintToleration()
	if err != nil {
		return errors.Wrapf(err, "failed to get taint toleration setting before creating instance manager pod")
	}
	nodeSelector, err := imc.ds.GetSettingSystemManagedComponentsNodeSelector()
	if err != nil {
		return errors.Wrapf(err, "failed to get node selector setting before creating instance manager pod")
	}

	registrySecretSetting, err := imc.ds.GetSetting(types.SettingNameRegistrySecret)
	if err != nil {
		return errors.Wrapf(err, "failed to get registry secret setting before creating instance manager pod")
	}
	registrySecret := registrySecretSetting.Value

	var podSpec *v1.Pod
	switch im.Spec.Type {
	case longhorn.InstanceManagerTypeEngine:
		podSpec, err = imc.createEngineManagerPodSpec(im, tolerations, registrySecret, nodeSelector)
	case longhorn.InstanceManagerTypeReplica:
		podSpec, err = imc.createReplicaManagerPodSpec(im, tolerations, registrySecret, nodeSelector)
	}
	if err != nil {
		return err
	}
	pod, err := imc.ds.CreatePod(podSpec)
	if err != nil {
		return errors.Wrapf(err, "failed to create pod for instance manager %v", im.Name)
	}
	logrus.Infof("Created instance manager pod %v for instance manager %v", pod.Name, im.Name)

	return nil
}

func (imc *InstanceManagerController) createGenericManagerPodSpec(im *longhorn.InstanceManager, tolerations []v1.Toleration, registrySecret string, nodeSelector map[string]string) (*v1.Pod, error) {
	tolerationsByte, err := json.Marshal(tolerations)
	if err != nil {
		return nil, err
	}

	priorityClass, err := imc.ds.GetSetting(types.SettingNamePriorityClass)
	if err != nil {
		return nil, err
	}

	imagePullPolicy, err := imc.ds.GetSettingImagePullPolicy()
	if err != nil {
		return nil, err
	}

	privileged := true
	podSpec := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            im.Name,
			Namespace:       imc.namespace,
			OwnerReferences: datastore.GetOwnerReferencesForInstanceManager(im),
			Annotations:     map[string]string{types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix): string(tolerationsByte)},
		},
		Spec: v1.PodSpec{
			ServiceAccountName: imc.serviceAccount,
			Tolerations:        util.GetDistinctTolerations(tolerations),
			NodeSelector:       nodeSelector,
			PriorityClassName:  priorityClass.Value,
			Containers: []v1.Container{
				{
					Image:           im.Spec.Image,
					ImagePullPolicy: imagePullPolicy,
					LivenessProbe: &v1.Probe{
						Handler: v1.Handler{
							TCPSocket: &v1.TCPSocketAction{
								Port: intstr.FromInt(engineapi.InstanceManagerDefaultPort),
							},
						},
						InitialDelaySeconds: datastore.PodProbeInitialDelay,
						TimeoutSeconds:      datastore.PodProbeTimeoutSeconds,
						PeriodSeconds:       datastore.PodProbePeriodSeconds,
						FailureThreshold:    datastore.PodLivenessProbeFailureThreshold,
					},
					SecurityContext: &v1.SecurityContext{
						Privileged: &privileged,
					},
				},
			},
			NodeName:      im.Spec.NodeID,
			RestartPolicy: v1.RestartPolicyNever,
		},
	}

	if registrySecret != "" {
		podSpec.Spec.ImagePullSecrets = []v1.LocalObjectReference{
			{
				Name: registrySecret,
			},
		}
	}

	// Apply resource requirements to newly created Instance Manager Pods.
	cpuResourceReq, err := GetInstanceManagerCPURequirement(imc.ds, im.Name)
	if err != nil {
		return nil, err
	}
	// Do nothing for the CPU requests if the value is 0.
	if cpuResourceReq != nil {
		podSpec.Spec.Containers[0].Resources = *cpuResourceReq
	}

	return podSpec, nil
}

func (imc *InstanceManagerController) createEngineManagerPodSpec(im *longhorn.InstanceManager, tolerations []v1.Toleration, registrySecret string, nodeSelector map[string]string) (*v1.Pod, error) {
	podSpec, err := imc.createGenericManagerPodSpec(im, tolerations, registrySecret, nodeSelector)
	if err != nil {
		return nil, err
	}

	podSpec.ObjectMeta.Labels = types.GetInstanceManagerLabels(imc.controllerID, im.Spec.Image, longhorn.InstanceManagerTypeEngine)
	podSpec.Spec.Containers[0].Name = "engine-manager"
	podSpec.Spec.Containers[0].Args = []string{
		"engine-manager", "--debug", "daemon", "--listen", "0.0.0.0:8500",
	}
	podSpec.Spec.Containers[0].VolumeMounts = []v1.VolumeMount{
		{
			MountPath: "/host/dev",
			Name:      "dev",
		},
		{
			MountPath: "/host/proc",
			Name:      "proc",
		},
		{
			MountPath:        types.EngineBinaryDirectoryInContainer,
			Name:             "engine-binaries",
			MountPropagation: &hostToContainer,
		},
	}
	podSpec.Spec.Volumes = []v1.Volume{
		{
			Name: "dev",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/dev",
				},
			},
		},
		{
			Name: "proc",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/proc",
				},
			},
		},
		{
			Name: "engine-binaries",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: types.EngineBinaryDirectoryOnHost,
				},
			},
		},
	}
	return podSpec, nil
}

func (imc *InstanceManagerController) createReplicaManagerPodSpec(im *longhorn.InstanceManager, tolerations []v1.Toleration, registrySecret string, nodeSelector map[string]string) (*v1.Pod, error) {
	podSpec, err := imc.createGenericManagerPodSpec(im, tolerations, registrySecret, nodeSelector)
	if err != nil {
		return nil, err
	}

	podSpec.ObjectMeta.Labels = types.GetInstanceManagerLabels(imc.controllerID, im.Spec.Image, longhorn.InstanceManagerTypeReplica)
	podSpec.Spec.Containers[0].Name = "replica-manager"
	podSpec.Spec.Containers[0].Args = []string{
		"longhorn-instance-manager", "--debug", "daemon", "--listen", "0.0.0.0:8500",
	}
	podSpec.Spec.Containers[0].VolumeMounts = []v1.VolumeMount{
		{
			MountPath:        "/host",
			Name:             "host",
			MountPropagation: &hostToContainer,
		},
	}
	podSpec.Spec.Volumes = []v1.Volume{
		{
			Name: "host",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/",
				},
			},
		},
	}

	service, err := net.LookupCNAME("longhorn-backend")

	if err == nil {
		podSpec.Spec.Containers[0].Env = []v1.EnvVar{
			{
				Name:  "HOST_SUFFIX",
				Value: strings.TrimPrefix(service, "longhorn-backend"),
			},
		}
	} else {
		logrus.Errorf("error getting fully qualified domain name for longhorn services: %v", err)
	}
	return podSpec, nil
}

func NewInstanceManagerUpdater(im *longhorn.InstanceManager) *InstanceManagerUpdater {
	c, _ := engineapi.NewInstanceManagerClient(im)
	return &InstanceManagerUpdater{
		client: c,
	}
}

func (updater *InstanceManagerUpdater) Poll() (map[string]longhorn.InstanceProcess, error) {
	return updater.client.ProcessList()
}

func (updater *InstanceManagerUpdater) GetNotifier() (*InstanceManagerNotifier, error) {
	watch, err := updater.client.ProcessWatch()
	if err != nil {
		return nil, err
	}
	return NewInstanceManagerNotifier(watch), nil
}

func NewInstanceManagerNotifier(stream *api.ProcessStream) *InstanceManagerNotifier {
	return &InstanceManagerNotifier{
		stream: stream,
	}
}

func (notifier *InstanceManagerNotifier) Recv() (struct{}, error) {
	if _, err := notifier.stream.Recv(); err != nil {
		return struct{}{}, err
	}

	return struct{}{}, nil
}

func (notifier *InstanceManagerNotifier) Close() {
	notifier.stream.Close()
	return
}

func (imc *InstanceManagerController) startMonitoring(im *longhorn.InstanceManager, instanceManagerUpdater *InstanceManagerUpdater) {
	if im.Status.IP == "" {
		// IP should be set
		logrus.Errorf("IP of instance manager %v was not set before monitoring", im.Name)
		return
	}

	// TODO: this function will error out in unit tests. Need to find a way to skip this part for unit tests.
	instanceManagerNotifier, err := instanceManagerUpdater.GetNotifier()
	if err != nil {
		logrus.Errorf("Failed to get the notifier of instance manager %v before monitoring: %v", im.Name, err)
		return
	}

	stopCh := make(chan struct{}, 1)
	monitorVoluntaryStopCh := make(chan struct{})
	monitor := &InstanceManagerMonitor{
		logger:                  imc.logger.WithField("instance manager", im.Name),
		Name:                    im.Name,
		controllerID:            imc.controllerID,
		ds:                      imc.ds,
		instanceManagerUpdater:  instanceManagerUpdater,
		instanceManagerNotifier: instanceManagerNotifier,
		lock:                    &sync.RWMutex{},
		stopCh:                  stopCh,
		done:                    false,
		monitorVoluntaryStopCh:  monitorVoluntaryStopCh,
		// notify monitor to update the instance map
		updateNotification: false,
	}

	imc.instanceManagerMonitorMutex.Lock()
	defer imc.instanceManagerMonitorMutex.Unlock()

	if _, ok := imc.instanceManagerMonitorMap[im.Name]; ok {
		logrus.Warnf("BUG: Monitoring for %v already exists", im.Name)
		return
	}
	imc.instanceManagerMonitorMap[im.Name] = stopCh

	go monitor.Run()

	go func() {
		<-monitorVoluntaryStopCh
		imc.instanceManagerMonitorMutex.Lock()
		delete(imc.instanceManagerMonitorMap, im.Name)
		imc.logger.WithField("instance manager", im.Name).Debug("removed the engine from imc.instanceManagerMonitorMap")
		imc.instanceManagerMonitorMutex.Unlock()
	}()
}

func (imc *InstanceManagerController) stopMonitoring(im *longhorn.InstanceManager) {
	imc.instanceManagerMonitorMutex.Lock()
	defer imc.instanceManagerMonitorMutex.Unlock()

	stopCh, ok := imc.instanceManagerMonitorMap[im.Name]
	if !ok {
		logrus.Warnf("instance manager %v: stop monitoring called when there is no monitoring", im.Name)
		return
	}

	select {
	case <-stopCh:
		// stopCh channel is already closed
	default:
		close(stopCh)
	}

	return
}

func (imc *InstanceManagerController) isMonitoring(imName string) bool {
	imc.instanceManagerMonitorMutex.RLock()
	defer imc.instanceManagerMonitorMutex.RUnlock()

	_, ok := imc.instanceManagerMonitorMap[imName]
	return ok
}

func (m *InstanceManagerMonitor) Run() {
	m.logger.Debugf("Start monitoring instance manager %v", m.Name)
	defer func() {
		m.logger.Debugf("Stop monitoring instance manager %v", m.Name)
		m.instanceManagerNotifier.Close()
		m.StopMonitorWithLock()
		close(m.monitorVoluntaryStopCh)
	}()

	go func() {
		continuousFailureCount := 0
		for {
			if continuousFailureCount >= engineapi.MaxMonitorRetryCount {
				m.logger.Errorf("instance manager monitor streaming continuously errors receiving items for %v times, will stop the monitor itself", engineapi.MaxMonitorRetryCount)
				m.StopMonitorWithLock()
			}

			if m.CheckMonitorStoppedWithLock() {
				return
			}

			if _, err := m.instanceManagerNotifier.Recv(); err != nil {
				m.logger.Errorf("error receiving next item in engine watch: %v", err)
				continuousFailureCount++
				time.Sleep(engineapi.MinPollCount * engineapi.PollInterval)
			} else {
				m.lock.Lock()
				m.updateNotification = true
				m.lock.Unlock()
			}
		}
	}()

	timer := 0
	ticker := time.NewTicker(engineapi.MinPollCount * engineapi.PollInterval)
	defer ticker.Stop()
	tick := ticker.C
	for {
		select {
		case <-tick:
			if m.CheckMonitorStoppedWithLock() {
				return
			}

			needUpdate := false

			m.lock.Lock()
			timer++
			if timer >= engineapi.MaxPollCount || m.updateNotification {
				needUpdate = true
				m.updateNotification = false
				timer = 0
			}
			m.lock.Unlock()

			if !needUpdate {
				continue
			}
			if needStop := m.pollAndUpdateInstanceMap(); needStop {
				return
			}
		case <-m.stopCh:
			return
		}
	}
}

func (m *InstanceManagerMonitor) pollAndUpdateInstanceMap() (needStop bool) {
	im, err := m.ds.GetInstanceManager(m.Name)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			m.logger.Info("stop monitoring because the instance manager no longer exists")
			return true
		}
		utilruntime.HandleError(errors.Wrapf(err, "fail to get instance manager %v for monitoring", m.Name))
		return false
	}

	if im.Status.OwnerID != m.controllerID {
		m.logger.Infof("stop monitoring the instance manager on this node (%v) because the instance manager has new ownerID %v", m.controllerID, im.Status.OwnerID)
		return true
	}

	resp, err := m.instanceManagerUpdater.Poll()
	if err != nil {
		utilruntime.HandleError(errors.Wrapf(err, "failed to poll instance info to update instance manager %v", m.Name))
		return false
	}

	if reflect.DeepEqual(im.Status.Instances, resp) {
		return false
	}
	im.Status.Instances = resp
	if _, err := m.ds.UpdateInstanceManagerStatus(im); err != nil {
		utilruntime.HandleError(errors.Wrapf(err, "failed to update instance map for instance manager %v", m.Name))
		return false
	}

	return false
}

func (m *InstanceManagerMonitor) CheckMonitorStoppedWithLock() bool {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.done
}

func (m *InstanceManagerMonitor) StopMonitorWithLock() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.done = true
}

func (imc *InstanceManagerController) isResponsibleFor(im *longhorn.InstanceManager) bool {
	return isControllerResponsibleFor(imc.controllerID, imc.ds, im.Name, im.Spec.NodeID, im.Status.OwnerID)
}
