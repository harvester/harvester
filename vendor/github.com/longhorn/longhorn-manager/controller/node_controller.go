package controller

import (
	"fmt"
	"reflect"
	"strings"
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
	"k8s.io/kubernetes/pkg/controller"

	monitor "github.com/longhorn/longhorn-manager/controller/monitor"
	"github.com/longhorn/longhorn-manager/datastore"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/scheduler"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
)

var (
	nodeControllerResyncPeriod = 30 * time.Second

	unknownFsid = "UNKNOWN_FSID"
)

type NodeController struct {
	*baseController

	// which namespace controller is running with
	namespace    string
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	monitor           monitor.Monitor
	collectedDiskInfo map[string]*monitor.CollectedDiskInfo

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced

	topologyLabelsChecker TopologyLabelsChecker

	scheduler *scheduler.ReplicaScheduler
}

type TopologyLabelsChecker func(kubeClient clientset.Interface, vers string) (bool, error)

func NewNodeController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	namespace, controllerID string) *NodeController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events("")})

	nc := &NodeController{
		baseController: newBaseController("longhorn-node", logger),

		namespace:    namespace,
		controllerID: controllerID,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-node-controller"}),

		ds: ds,

		collectedDiskInfo: make(map[string]*monitor.CollectedDiskInfo, 0),

		topologyLabelsChecker: util.IsKubernetesVersionAtLeast,
	}

	nc.scheduler = scheduler.NewReplicaScheduler(ds)

	// We want to check the real time usage of disk on nodes.
	// Therefore, we add a small resync for the NodeInformer here
	ds.NodeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    nc.enqueueNode,
		UpdateFunc: func(old, cur interface{}) { nc.enqueueNode(cur) },
		DeleteFunc: nc.enqueueNode,
	}, nodeControllerResyncPeriod)

	nc.cacheSyncs = append(nc.cacheSyncs, ds.NodeInformer.HasSynced)

	ds.SettingInformer.AddEventHandlerWithResyncPeriod(
		cache.FilteringResourceEventHandler{
			FilterFunc: nc.isResponsibleForSetting,
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    nc.enqueueSetting,
				UpdateFunc: func(old, cur interface{}) { nc.enqueueSetting(cur) },
			},
		}, 0)
	nc.cacheSyncs = append(nc.cacheSyncs, ds.SettingInformer.HasSynced)

	ds.ReplicaInformer.AddEventHandlerWithResyncPeriod(
		cache.FilteringResourceEventHandler{
			FilterFunc: nc.isResponsibleForReplica,
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    nc.enqueueReplica,
				UpdateFunc: func(old, cur interface{}) { nc.enqueueReplica(cur) },
				DeleteFunc: nc.enqueueReplica,
			},
		}, 0)
	nc.cacheSyncs = append(nc.cacheSyncs, ds.ReplicaInformer.HasSynced)

	ds.PodInformer.AddEventHandlerWithResyncPeriod(
		cache.FilteringResourceEventHandler{
			FilterFunc: isManagerPod,
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    nc.enqueueManagerPod,
				UpdateFunc: func(old, cur interface{}) { nc.enqueueManagerPod(cur) },
				DeleteFunc: nc.enqueueManagerPod,
			},
		}, 0)
	nc.cacheSyncs = append(nc.cacheSyncs, ds.PodInformer.HasSynced)

	ds.KubeNodeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, cur interface{}) { nc.enqueueKubernetesNode(cur) },
		DeleteFunc: nc.enqueueKubernetesNode,
	}, 0)
	nc.cacheSyncs = append(nc.cacheSyncs, ds.KubeNodeInformer.HasSynced)

	return nc
}

func (nc *NodeController) isResponsibleForSetting(obj interface{}) bool {
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

	return types.SettingName(setting.Name) == types.SettingNameStorageMinimalAvailablePercentage ||
		types.SettingName(setting.Name) == types.SettingNameBackingImageCleanupWaitInterval ||
		types.SettingName(setting.Name) == types.SettingNameOrphanAutoDeletion
}

func (nc *NodeController) isResponsibleForReplica(obj interface{}) bool {
	replica, ok := obj.(*longhorn.Replica)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return false
		}

		// use the last known state, to enqueue, dependent objects
		replica, ok = deletedState.Obj.(*longhorn.Replica)
		if !ok {
			return false
		}
	}

	return replica.Spec.NodeID == nc.controllerID
}

func isManagerPod(obj interface{}) bool {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
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
		if con.Name == "longhorn-manager" {
			return true
		}
	}
	return false
}

func (nc *NodeController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer nc.queue.ShutDown()

	logrus.Infof("Start Longhorn node controller")
	defer logrus.Infof("Shutting down Longhorn node controller")

	if !cache.WaitForNamedCacheSync("longhorn node", stopCh, nc.cacheSyncs...) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(nc.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (nc *NodeController) worker() {
	for nc.processNextWorkItem() {
	}
}

func (nc *NodeController) processNextWorkItem() bool {
	key, quit := nc.queue.Get()

	if quit {
		return false
	}
	defer nc.queue.Done(key)

	err := nc.syncNode(key.(string))
	nc.handleErr(err, key)

	return true
}

func (nc *NodeController) handleErr(err error, key interface{}) {
	if err == nil {
		nc.queue.Forget(key)
		return
	}

	if nc.queue.NumRequeues(key) < maxRetries {
		logrus.Warnf("Error syncing Longhorn node %v: %v", key, err)
		nc.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	logrus.Warnf("Dropping Longhorn node %v out of the queue: %v", key, err)
	nc.queue.Forget(key)
}

func getLoggerForNode(logger logrus.FieldLogger, n *longhorn.Node) *logrus.Entry {
	return logger.WithField("node", n.Name)
}

func (nc *NodeController) syncNode(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "fail to sync node for %v", key)
	}()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != nc.namespace {
		// Not ours, don't do anything
		return nil
	}

	node, err := nc.ds.GetNode(name)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			logrus.Errorf("Longhorn node %v has been deleted", key)
			return nil
		}
		return err
	}

	if node.DeletionTimestamp != nil {
		nc.eventRecorder.Eventf(node, v1.EventTypeWarning, EventReasonDelete, "Deleting node %v", node.Name)
		return nc.ds.RemoveFinalizerForNode(node)
	}

	existingNode := node.DeepCopy()
	defer func() {
		// we're going to update volume assume things changes
		if err == nil && !reflect.DeepEqual(existingNode.Status, node.Status) {
			_, err = nc.ds.UpdateNodeStatus(node)
		}
		// requeue if it's conflict
		if apierrors.IsConflict(errors.Cause(err)) {
			logrus.Debugf("Requeue %v due to conflict: %v", key, err)
			nc.enqueueNode(node)
			err = nil
		}
	}()

	// sync node state by manager pod
	managerPods, err := nc.ds.ListManagerPods()
	if err != nil {
		return err
	}
	nodeManagerFound := false
	for _, pod := range managerPods {
		if pod.Spec.NodeName == node.Name {
			nodeManagerFound = true
			podConditions := pod.Status.Conditions
			for _, podCondition := range podConditions {
				if podCondition.Type == v1.PodReady {
					if podCondition.Status == v1.ConditionTrue && pod.Status.Phase == v1.PodRunning {
						node.Status.Conditions = types.SetConditionAndRecord(node.Status.Conditions,
							longhorn.NodeConditionTypeReady, longhorn.ConditionStatusTrue,
							"", fmt.Sprintf("Node %v is ready", node.Name),
							nc.eventRecorder, node, v1.EventTypeNormal)
					} else {
						node.Status.Conditions = types.SetConditionAndRecord(node.Status.Conditions,
							longhorn.NodeConditionTypeReady, longhorn.ConditionStatusFalse,
							string(longhorn.NodeConditionReasonManagerPodDown),
							fmt.Sprintf("Node %v is down: the manager pod %v is not running", node.Name, pod.Name),
							nc.eventRecorder, node, v1.EventTypeWarning)
					}
					break
				}
			}
			break
		}
	}

	if !nodeManagerFound {
		node.Status.Conditions = types.SetConditionAndRecord(node.Status.Conditions,
			longhorn.NodeConditionTypeReady, longhorn.ConditionStatusFalse,
			string(longhorn.NodeConditionReasonManagerPodMissing),
			fmt.Sprintf("manager pod missing: node %v has no manager pod running on it", node.Name),
			nc.eventRecorder, node, v1.EventTypeWarning)
	}

	// sync node state with kuberentes node status
	kubeNode, err := nc.ds.GetKubernetesNode(name)
	if err != nil {
		// if kubernetes node has been removed from cluster
		if apierrors.IsNotFound(err) {
			node.Status.Conditions = types.SetConditionAndRecord(node.Status.Conditions,
				longhorn.NodeConditionTypeReady, longhorn.ConditionStatusFalse,
				string(longhorn.NodeConditionReasonKubernetesNodeGone),
				fmt.Sprintf("Kubernetes node missing: node %v has been removed from the cluster and there is no manager pod running on it", node.Name),
				nc.eventRecorder, node, v1.EventTypeWarning)
		} else {
			return err
		}
	} else {
		kubeConditions := kubeNode.Status.Conditions
		for _, con := range kubeConditions {
			switch con.Type {
			case v1.NodeReady:
				if con.Status != v1.ConditionTrue {
					node.Status.Conditions = types.SetConditionAndRecord(node.Status.Conditions,
						longhorn.NodeConditionTypeReady, longhorn.ConditionStatusFalse,
						string(longhorn.NodeConditionReasonKubernetesNodeNotReady),
						fmt.Sprintf("Kubernetes node %v not ready: %v", node.Name, con.Reason),
						nc.eventRecorder, node, v1.EventTypeWarning)
					break
				}
			case v1.NodeDiskPressure,
				v1.NodePIDPressure,
				v1.NodeMemoryPressure,
				v1.NodeNetworkUnavailable:
				if con.Status == v1.ConditionTrue {
					node.Status.Conditions = types.SetConditionAndRecord(node.Status.Conditions,
						longhorn.NodeConditionTypeReady, longhorn.ConditionStatusFalse,
						string(longhorn.NodeConditionReasonKubernetesNodePressure),
						fmt.Sprintf("Kubernetes node %v has pressure: %v, %v", node.Name, con.Reason, con.Message),
						nc.eventRecorder, node, v1.EventTypeWarning)

					break
				}
			default:
				if con.Status == v1.ConditionTrue {
					nc.eventRecorder.Eventf(node, v1.EventTypeWarning, longhorn.NodeConditionReasonUnknownNodeConditionTrue, "Unknown condition true of kubernetes node %v: condition type is %v, reason is %v, message is %v", node.Name, con.Type, con.Reason, con.Message)
				}

			}
		}

		DisableSchedulingOnCordonedNode, err :=
			nc.ds.GetSettingAsBool(types.SettingNameDisableSchedulingOnCordonedNode)
		if err != nil {
			logrus.Errorf("error getting disable scheduling on cordoned node setting: %v", err)
			return err
		}

		// Update node condition based on
		// DisableSchedulingOnCordonedNode setting and
		// k8s node status
		kubeSpec := kubeNode.Spec
		if DisableSchedulingOnCordonedNode &&
			kubeSpec.Unschedulable {
			node.Status.Conditions =
				types.SetConditionAndRecord(node.Status.Conditions,
					longhorn.NodeConditionTypeSchedulable,
					longhorn.ConditionStatusFalse,
					string(longhorn.NodeConditionReasonKubernetesNodeCordoned),
					fmt.Sprintf("Node %v is cordoned", node.Name),
					nc.eventRecorder, node,
					v1.EventTypeNormal)
		} else {
			node.Status.Conditions =
				types.SetConditionAndRecord(node.Status.Conditions,
					longhorn.NodeConditionTypeSchedulable,
					longhorn.ConditionStatusTrue,
					"",
					"",
					nc.eventRecorder, node,
					v1.EventTypeNormal)
		}

		node.Status.Region, node.Status.Zone = types.GetRegionAndZone(kubeNode.Labels)

	}

	if nc.controllerID != node.Name {
		return nil
	}

	mon, err := nc.checkMonitor(nc.controllerID)
	if err != nil {
		return err
	}

	collectedDiskInfo, err := nc.syncWithMonitor(mon, node)
	if err != nil {
		if strings.Contains(err.Error(), "mismatching disks") {
			logrus.Info(err.Error())
			return nil
		}
		return err
	}
	nc.collectedDiskInfo = collectedDiskInfo

	// sync disks status on current node
	if err := nc.syncDiskStatus(node, collectedDiskInfo); err != nil {
		return err
	}

	// sync mount propagation status on current node
	for _, pod := range managerPods {
		if pod.Spec.NodeName == node.Name {
			if err := nc.syncNodeStatus(pod, node); err != nil {
				return err
			}
		}
	}

	if err := nc.syncInstanceManagers(node); err != nil {
		return err
	}

	if err := nc.cleanUpBackingImagesInDisks(node); err != nil {
		return err
	}

	if err := nc.syncOrphans(node, collectedDiskInfo); err != nil {
		return err
	}

	return nil
}

func (nc *NodeController) enqueueNode(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	nc.queue.Add(key)
}

func (nc *NodeController) enqueueSetting(obj interface{}) {
	nodesRO, err := nc.ds.ListNodesRO()
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't list nodes: %v ", err))
		return
	}

	for _, node := range nodesRO {
		nc.enqueueNode(node)
	}
}

func (nc *NodeController) enqueueReplica(obj interface{}) {
	replica, ok := obj.(*longhorn.Replica)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		replica, ok = deletedState.Obj.(*longhorn.Replica)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	node, err := nc.ds.GetNode(replica.Spec.NodeID)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("couldn't get node %v for replica %v: %v ",
				replica.Spec.NodeID, replica.Name, err))
		}
		return
	}
	nc.enqueueNode(node)
}

func (nc *NodeController) enqueueManagerPod(obj interface{}) {
	nodesRO, err := nc.ds.ListNodesRO()
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't list nodes: %v ", err))
		return
	}
	for _, node := range nodesRO {
		nc.enqueueNode(node)
	}
}

func (nc *NodeController) enqueueKubernetesNode(obj interface{}) {
	kubernetesNode, ok := obj.(*v1.Node)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
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

	nodeRO, err := nc.ds.GetNodeRO(kubernetesNode.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("couldn't get longhorn node %v: %v ", kubernetesNode.Name, err))
		}
		return
	}
	nc.enqueueNode(nodeRO)
}

func (nc *NodeController) syncDiskStatus(node *longhorn.Node, collectedDataInfo map[string]*monitor.CollectedDiskInfo) error {
	alignDiskSpecAndStatus(node)

	notReadyDiskInfoMap, readyDiskInfoMap := nc.findNotReadyAndReadyDiskMaps(node, collectedDataInfo)

	for _, diskInfoMap := range notReadyDiskInfoMap {
		nc.updateNotReadyDiskStatusReadyCondition(node, diskInfoMap)
	}

	for fsid, diskInfoMap := range readyDiskInfoMap {
		nc.updateReadyDiskStatusReadyCondition(node, fsid, diskInfoMap)
	}

	return nc.updateDiskStatusSchedulableCondition(node)
}

func (nc *NodeController) findNotReadyAndReadyDiskMaps(node *longhorn.Node, collectedDataInfo map[string]*monitor.CollectedDiskInfo) (notReadyDiskInfoMap, readyDiskInfoMap map[string]map[string]*monitor.CollectedDiskInfo) {
	notReadyDiskInfoMap = make(map[string]map[string]*monitor.CollectedDiskInfo, 0)
	readyDiskInfoMap = make(map[string]map[string]*monitor.CollectedDiskInfo, 0)
	fsidDiskGroupMap := make(map[string]map[string]*monitor.CollectedDiskInfo, 0)

	// Find out notReadyDiskInfoMap and fsidDiskGroupMap
	for diskName, diskInfo := range collectedDataInfo {
		fsid := unknownFsid
		if diskInfo.DiskStat != nil {
			fsid = diskInfo.DiskStat.Fsid
		}

		if notReadyDiskInfoMap[fsid] == nil {
			notReadyDiskInfoMap[fsid] = make(map[string]*monitor.CollectedDiskInfo, 0)
		}
		if fsidDiskGroupMap[fsid] == nil {
			fsidDiskGroupMap[fsid] = make(map[string]*monitor.CollectedDiskInfo, 0)
		}

		// The disk info collected by the node monitor will contain the condition only when the disk is NotReady
		if diskInfo.Condition != nil {
			notReadyDiskInfoMap[fsid][diskName] = diskInfo
			continue
		}

		// Move the disks with UUID config file missing error and UUID mismatching error to notReadyDiskInfoMap
		if node.Status.DiskStatus[diskName].DiskUUID != "" {
			errorMessage := ""
			if diskInfo.DiskUUID == "" {
				errorMessage = fmt.Sprintf("Disk %v(%v) on node %v is not ready: cannot find disk config file, maybe due to a mount error",
					diskName, diskInfo.Path, node.Name)
			} else if node.Status.DiskStatus[diskName].DiskUUID != diskInfo.DiskUUID {
				errorMessage = fmt.Sprintf("Disk %v(%v) on node %v is not ready: record diskUUID doesn't match the one on the disk ",
					diskName, diskInfo.Path, node.Name)
			}

			if errorMessage != "" {
				notReadyDiskInfoMap[fsid][diskName] =
					monitor.NewDiskInfo(diskInfo.Path, diskInfo.DiskUUID,
						diskInfo.NodeOrDiskEvicted, diskInfo.DiskStat,
						diskInfo.OrphanedReplicaDirectoryNames,
						string(longhorn.DiskConditionReasonDiskFilesystemChanged), errorMessage)
				continue
			}
		}

		fsidDiskGroupMap[fsid][diskName] = diskInfo
	}

	// Find the duplicated disks and move them to notReadyDiskInfoMap
	for fsid, diskInfoMap := range fsidDiskGroupMap {
		for diskName, diskInfo := range diskInfoMap {
			if readyDiskInfoMap[fsid] == nil {
				readyDiskInfoMap[fsid] = make(map[string]*monitor.CollectedDiskInfo, 0)
			}

			if nc.isFSIDDuplicatedWithExistingReadyDisk(diskName, diskInfoMap, node.Status.DiskStatus) ||
				isReadyDiskFound(readyDiskInfoMap[fsid]) {
				notReadyDiskInfoMap[fsid][diskName] =
					monitor.NewDiskInfo(diskInfo.Path, diskInfo.DiskUUID, diskInfo.NodeOrDiskEvicted, diskInfo.DiskStat,
						diskInfo.OrphanedReplicaDirectoryNames,
						string(longhorn.DiskConditionReasonDiskFilesystemChanged),
						fmt.Sprintf("Disk %v(%v) on node %v is not ready: disk has same file system ID %v as other disks %+v",
							diskName, diskInfoMap[diskName].Path, node.Name, fsid, monitor.GetDiskNamesFromDiskMap(diskInfoMap)))
				continue
			}

			node.Status.DiskStatus[diskName].DiskUUID = diskInfo.DiskUUID
			readyDiskInfoMap[fsid][diskName] = diskInfo
		}
	}

	return notReadyDiskInfoMap, readyDiskInfoMap
}

func (nc *NodeController) updateNotReadyDiskStatusReadyCondition(node *longhorn.Node, diskInfoMap map[string]*monitor.CollectedDiskInfo) {
	for diskName, info := range diskInfoMap {
		if info.Condition == nil {
			continue
		}
		node.Status.DiskStatus[diskName].Conditions =
			types.SetConditionAndRecord(node.Status.DiskStatus[diskName].Conditions,
				longhorn.DiskConditionTypeReady,
				info.Condition.Status,
				info.Condition.Reason,
				info.Condition.Message,
				nc.eventRecorder,
				node,
				v1.EventTypeWarning)
	}
}

func (nc *NodeController) updateReadyDiskStatusReadyCondition(node *longhorn.Node, fsid string, diskInfoMap map[string]*monitor.CollectedDiskInfo) {
	diskStatusMap := node.Status.DiskStatus

	for diskName, info := range diskInfoMap {
		diskStatus := diskStatusMap[diskName]

		if diskStatus.DiskUUID == info.DiskUUID {
			// on the default disks this will be updated constantly since there is always something generating new disk usage (logs, etc)
			// We also don't need byte/block precisions for this instead we can round down to the next 10/100mb
			const truncateTo = 100 * 1024 * 1024
			usableStorage := (diskInfoMap[diskName].DiskStat.StorageAvailable / truncateTo) * truncateTo
			diskStatus.StorageAvailable = usableStorage
			diskStatus.StorageMaximum = diskInfoMap[diskName].DiskStat.StorageMaximum
			diskStatusMap[diskName].Conditions = types.SetConditionAndRecord(diskStatusMap[diskName].Conditions,
				longhorn.DiskConditionTypeReady, longhorn.ConditionStatusTrue,
				"", fmt.Sprintf("Disk %v(%v) on node %v is ready", diskName, diskInfoMap[diskName].Path, node.Name),
				nc.eventRecorder, node, v1.EventTypeNormal)
		}
		diskStatusMap[diskName] = diskStatus
	}
}

func (nc *NodeController) updateDiskStatusSchedulableCondition(node *longhorn.Node) error {
	log := getLoggerForNode(nc.logger, node)

	diskStatusMap := node.Status.DiskStatus

	// update Schedulable condition
	minimalAvailablePercentage, err := nc.ds.GetSettingAsInt(types.SettingNameStorageMinimalAvailablePercentage)
	if err != nil {
		return err
	}

	for diskName, disk := range node.Spec.Disks {
		diskStatus := diskStatusMap[diskName]

		if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeReady).Status != longhorn.ConditionStatusTrue {
			diskStatus.StorageScheduled = 0
			diskStatus.ScheduledReplica = map[string]int64{}
			diskStatus.Conditions = types.SetConditionAndRecord(diskStatus.Conditions,
				longhorn.DiskConditionTypeSchedulable, longhorn.ConditionStatusFalse,
				string(longhorn.DiskConditionReasonDiskNotReady),
				fmt.Sprintf("the disk %v(%v) on the node %v is not ready", diskName, disk.Path, node.Name),
				nc.eventRecorder, node, v1.EventTypeWarning)
		} else {
			// sync backing image managers
			list, err := nc.ds.ListBackingImageManagersByDiskUUID(diskStatus.DiskUUID)
			if err != nil {
				return err
			}
			for _, bim := range list {
				if bim.Spec.NodeID != node.Name || bim.Spec.DiskPath != disk.Path {
					log.Infof("Node Controller: Node & disk info in backing image manager %v need to be updated", bim.Name)
					bim.Spec.NodeID = node.Name
					bim.Spec.DiskPath = disk.Path
					if _, err := nc.ds.UpdateBackingImageManager(bim); err != nil {
						log.WithError(err).Errorf("Failed to update node & disk info for backing image manager %v when syncing disk %v(%v), will enqueue then resync node", bim.Name, diskName, diskStatus.DiskUUID)
						nc.enqueueNode(node)
						continue
					}
				}
			}

			// sync replicas as well as calculate storage scheduled
			replicas, err := nc.ds.ListReplicasByDiskUUID(diskStatus.DiskUUID)
			if err != nil {
				return err
			}
			scheduledReplica := map[string]int64{}
			storageScheduled := int64(0)
			for _, replica := range replicas {
				if replica.Spec.NodeID != node.Name || replica.Spec.DiskPath != disk.Path {
					replica.Spec.NodeID = node.Name
					replica.Spec.DiskPath = disk.Path
					if _, err := nc.ds.UpdateReplica(replica); err != nil {
						log.Errorf("Failed to update node & disk info for replica %v when syncing disk %v(%v), will enqueue then resync node", replica.Name, diskName, diskStatus.DiskUUID)
						nc.enqueueNode(node)
						continue
					}
				}
				storageScheduled += replica.Spec.VolumeSize
				scheduledReplica[replica.Name] = replica.Spec.VolumeSize
			}
			diskStatus.StorageScheduled = storageScheduled
			diskStatus.ScheduledReplica = scheduledReplica
			// check disk pressure
			info, err := nc.scheduler.GetDiskSchedulingInfo(disk, diskStatus)
			if err != nil {
				return err
			}
			if !nc.scheduler.IsSchedulableToDisk(0, 0, info) {
				diskStatus.Conditions = types.SetConditionAndRecord(diskStatus.Conditions,
					longhorn.DiskConditionTypeSchedulable, longhorn.ConditionStatusFalse,
					string(longhorn.DiskConditionReasonDiskPressure),
					fmt.Sprintf("the disk %v(%v) on the node %v has %v available, but requires reserved %v, minimal %v%s to schedule more replicas",
						diskName, disk.Path, node.Name, diskStatus.StorageAvailable, disk.StorageReserved, minimalAvailablePercentage, "%"),
					nc.eventRecorder, node, v1.EventTypeWarning)
			} else {
				diskStatus.Conditions = types.SetConditionAndRecord(diskStatus.Conditions,
					longhorn.DiskConditionTypeSchedulable, longhorn.ConditionStatusTrue,
					"", fmt.Sprintf("Disk %v(%v) on node %v is schedulable", diskName, disk.Path, node.Name),
					nc.eventRecorder, node, v1.EventTypeNormal)
			}
		}

		diskStatusMap[diskName] = diskStatus
	}

	return nil
}

func (nc *NodeController) syncNodeStatus(pod *v1.Pod, node *longhorn.Node) error {
	// sync bidirectional mount propagation for node status to check whether the node could deploy CSI driver
	for _, mount := range pod.Spec.Containers[0].VolumeMounts {
		if mount.Name == types.LonghornSystemKey {
			mountPropagationStr := ""
			if mount.MountPropagation == nil {
				mountPropagationStr = "nil"
			} else {
				mountPropagationStr = string(*mount.MountPropagation)
			}
			if mount.MountPropagation == nil || *mount.MountPropagation != v1.MountPropagationBidirectional {
				node.Status.Conditions = types.SetCondition(node.Status.Conditions, longhorn.NodeConditionTypeMountPropagation, longhorn.ConditionStatusFalse,
					string(longhorn.NodeConditionReasonNoMountPropagationSupport),
					fmt.Sprintf("The MountPropagation value %s is not detected from pod %s, node %s", mountPropagationStr, pod.Name, pod.Spec.NodeName))
			} else {
				node.Status.Conditions = types.SetCondition(node.Status.Conditions, longhorn.NodeConditionTypeMountPropagation, longhorn.ConditionStatusTrue, "", "")
			}
			break
		}
	}

	return nil
}

func (nc *NodeController) syncInstanceManagers(node *longhorn.Node) error {
	defaultInstanceManagerImage, err := nc.ds.GetSettingValueExisted(types.SettingNameDefaultInstanceManagerImage)
	if err != nil {
		return err
	}

	imTypes := []longhorn.InstanceManagerType{longhorn.InstanceManagerTypeEngine}

	// Clean up all replica managers if there is no disk on the node
	if len(node.Spec.Disks) == 0 {
		rmMap, err := nc.ds.ListInstanceManagersByNode(node.Name, longhorn.InstanceManagerTypeReplica)
		if err != nil {
			return err
		}
		for _, rm := range rmMap {
			logrus.Debugf("Prepare to clean up the replica manager %v since there is no available disk on node %v", rm.Name, node.Name)
			if err := nc.ds.DeleteInstanceManager(rm.Name); err != nil {
				return err
			}
		}
	} else {
		imTypes = append(imTypes, longhorn.InstanceManagerTypeReplica)
	}

	for _, imType := range imTypes {
		defaultInstanceManagerCreated := false
		imMap, err := nc.ds.ListInstanceManagersByNode(node.Name, imType)
		if err != nil {
			return err
		}
		for _, im := range imMap {
			if im.Labels[types.GetLonghornLabelKey(types.LonghornLabelNode)] != im.Spec.NodeID {
				return fmt.Errorf("BUG: Instance manager %v NodeID %v is not consistent with the label %v=%v",
					im.Name, im.Spec.NodeID, types.GetLonghornLabelKey(types.LonghornLabelNode), im.Labels[types.GetLonghornLabelKey(types.LonghornLabelNode)])
			}
			cleanupRequired := true
			if im.Spec.Image == defaultInstanceManagerImage {
				// Create default instance manager if needed.
				defaultInstanceManagerCreated = true
				cleanupRequired = false
			} else {
				// Clean up old instance managers if there is no running instance.
				if im.Status.CurrentState == longhorn.InstanceManagerStateRunning && im.DeletionTimestamp == nil {
					for _, instance := range im.Status.Instances {
						if instance.Status.State == longhorn.InstanceStateRunning || instance.Status.State == longhorn.InstanceStateStarting {
							cleanupRequired = false
							break
						}
					}
				}
			}
			if cleanupRequired {
				logrus.Debugf("Prepare to clean up the redundant instance manager %v when there is no running/starting instance", im.Name)
				if err := nc.ds.DeleteInstanceManager(im.Name); err != nil {
					return err
				}
			}
		}
		if !defaultInstanceManagerCreated {
			imName, err := types.GetInstanceManagerName(imType)
			if err != nil {
				return err
			}
			logrus.Debugf("Prepare to create default instance manager %v, node: %v, default instance manager image: %v, type: %v",
				imName, node.Name, defaultInstanceManagerImage, imType)
			if _, err := nc.createInstanceManager(node, imName, defaultInstanceManagerImage, imType); err != nil {
				return err
			}
		}
	}
	return nil
}

func (nc *NodeController) createInstanceManager(node *longhorn.Node, imName, image string, imType longhorn.InstanceManagerType) (*longhorn.InstanceManager, error) {
	instanceManager := &longhorn.InstanceManager{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          types.GetInstanceManagerLabels(node.Name, image, imType),
			Name:            imName,
			OwnerReferences: datastore.GetOwnerReferencesForNode(node),
		},
		Spec: longhorn.InstanceManagerSpec{
			Image:  image,
			NodeID: node.Name,
			Type:   imType,
		},
	}

	return nc.ds.CreateInstanceManager(instanceManager)
}

func (nc *NodeController) cleanUpBackingImagesInDisks(node *longhorn.Node) error {
	settingValue, err := nc.ds.GetSettingAsInt(types.SettingNameBackingImageCleanupWaitInterval)
	if err != nil {
		logrus.Errorf("failed to get Setting BackingImageCleanupWaitInterval, won't do cleanup for backing images: %v", err)
		return nil
	}
	waitInterval := time.Duration(settingValue) * time.Minute

	backingImages, err := nc.ds.ListBackingImages()
	if err != nil {
		return err
	}
	for _, bi := range backingImages {
		log := getLoggerForBackingImage(nc.logger, bi).WithField("node", node.Name)
		bids, err := nc.ds.GetBackingImageDataSource(bi.Name)
		if err != nil && !apierrors.IsNotFound(err) {
			log.WithError(err).Error("Failed to get the backing image data source when cleaning up the images in disks")
			continue
		}
		existingBackingImage := bi.DeepCopy()
		BackingImageDiskFileCleanup(node, bi, bids, waitInterval, 1)
		if !reflect.DeepEqual(existingBackingImage.Spec, bi.Spec) {
			if _, err := nc.ds.UpdateBackingImage(bi); err != nil {
				log.WithError(err).Error("Failed to update backing image when cleaning up the images in disks")
				// Requeue the node but do not fail the whole sync function
				nc.enqueueNode(node)
				continue
			}
		}
	}

	return nil
}

func BackingImageDiskFileCleanup(node *longhorn.Node, bi *longhorn.BackingImage, bids *longhorn.BackingImageDataSource, waitInterval time.Duration, haRequirement int) {
	if bi.Spec.Disks == nil || bi.Status.DiskLastRefAtMap == nil {
		return
	}

	if haRequirement < 1 {
		haRequirement = 1
	}

	var readyDiskFileCount, handlingDiskFileCount, failedDiskFileCount int
	for diskUUID := range bi.Spec.Disks {
		// Consider non-existing files as pending/handling backing image files.
		fileStatus, exists := bi.Status.DiskFileStatusMap[diskUUID]
		if !exists {
			handlingDiskFileCount++
			continue
		}
		switch fileStatus.State {
		case longhorn.BackingImageStateReadyForTransfer, longhorn.BackingImageStateReady:
			readyDiskFileCount++
		case longhorn.BackingImageStateFailed:
			failedDiskFileCount++
		default:
			handlingDiskFileCount++
		}
	}

	for _, diskStatus := range node.Status.DiskStatus {
		diskUUID := diskStatus.DiskUUID
		if _, exists := bi.Spec.Disks[diskUUID]; !exists {
			continue
		}
		isFirstFile := bids != nil && !bids.Spec.FileTransferred && diskUUID == bids.Spec.DiskUUID
		if isFirstFile {
			continue
		}
		lastRefAtStr, exists := bi.Status.DiskLastRefAtMap[diskUUID]
		if !exists {
			continue
		}
		lastRefAt, err := util.ParseTime(lastRefAtStr)
		if err != nil {
			logrus.Errorf("Unable to parse LastRefAt timestamp %v for backing image %v", lastRefAtStr, bi.Name)
			continue
		}
		if !time.Now().After(lastRefAt.Add(waitInterval)) {
			continue
		}

		// The cleanup strategy:
		//  1. If there are enough ready files for a backing image, it's fine to do cleanup.
		//  2. If there are no enough ready files, try to retain handling(state empty/pending/starting/in-progress/unknown) files to guarantee the HA requirement.
		//  3. If there are no enough ready & handling files, try to retain failed files to guarantee the HA requirement.
		//  4. If there are no enough files including failed ones, skip cleanup.
		fileStatus, exists := bi.Status.DiskFileStatusMap[diskUUID]
		if !exists {
			fileStatus = &longhorn.BackingImageDiskFileStatus{
				State: "",
			}
		}
		switch fileStatus.State {
		case longhorn.BackingImageStateFailed:
			if haRequirement >= readyDiskFileCount+handlingDiskFileCount+failedDiskFileCount {
				continue
			}
			failedDiskFileCount--
		case longhorn.BackingImageStateReadyForTransfer, longhorn.BackingImageStateReady:
			if haRequirement >= readyDiskFileCount {
				continue
			}
			readyDiskFileCount--
		default:
			if haRequirement >= readyDiskFileCount+handlingDiskFileCount {
				continue
			}
			handlingDiskFileCount--
		}

		logrus.Debugf("Start to cleanup the unused file in disk %v for backing image %v", diskUUID, bi.Name)
		delete(bi.Spec.Disks, diskUUID)
	}
}

func (nc *NodeController) checkMonitor(nodeName string) (monitor.Monitor, error) {
	if nodeName == "" {
		return nil, nil
	}

	if nc.monitor != nil {
		return nc.monitor, nil
	}

	monitor, err := monitor.NewNodeMonitor(nc.logger, nc.ds, nodeName, nc.enqueueNodeForMonitor)
	if err != nil {
		return nil, err
	}

	nc.monitor = monitor

	return monitor, nil
}

func (nc *NodeController) enqueueNodeForMonitor(key string) {
	nc.queue.Add(key)
}

func (nc *NodeController) syncOrphans(node *longhorn.Node, collectedDataInfo map[string]*monitor.CollectedDiskInfo) error {
	for diskName, diskInfo := range collectedDataInfo {
		newOrphanedReplicaDirectoryNames, missingOrphanedReplicaDirectoryNames :=
			nc.getNewAndMissingOrphanedReplicaDirectoryNames(diskName, diskInfo.DiskUUID, diskInfo.Path, diskInfo.OrphanedReplicaDirectoryNames)

		if err := nc.createOrphans(node, diskName, diskInfo, newOrphanedReplicaDirectoryNames); err != nil {
			return errors.Wrapf(err, "failed to create orphans for disk %v", diskName)
		}
		if err := nc.deleteOrphans(node, diskName, diskInfo, missingOrphanedReplicaDirectoryNames); err != nil {
			return errors.Wrapf(err, "failed to delete orphans for disk %v", diskName)
		}
	}

	return nil
}

func (nc *NodeController) getNewAndMissingOrphanedReplicaDirectoryNames(diskName, diskUUID, diskPath string, replicaDirectoryNames map[string]string) (map[string]string, map[string]string) {
	newOrphanedReplicaDirectoryNames := map[string]string{}
	missingOrphanedReplicaDirectoryNames := map[string]string{}

	// Find out the new/missing orphaned directories by checking with orphan CRs
	orphans, err := nc.ds.ListOrphansByNode(nc.controllerID)
	if err != nil {
		logrus.Errorf("unable to list orphans for node %v since %v", nc.controllerID, err.Error())
		return map[string]string{}, map[string]string{}
	}

	for dirName := range replicaDirectoryNames {
		orphanName := types.GetOrphanChecksumNameForOrphanedDirectory(nc.controllerID, diskName, diskPath, diskUUID, dirName)
		if _, ok := orphans[orphanName]; !ok {
			newOrphanedReplicaDirectoryNames[dirName] = ""
		}
	}

	for _, orphan := range orphans {
		if orphan.Spec.Parameters[longhorn.OrphanDiskName] != diskName ||
			orphan.Spec.Parameters[longhorn.OrphanDiskUUID] != diskUUID ||
			orphan.Spec.Parameters[longhorn.OrphanDiskPath] != diskPath {
			continue
		}

		dirName := orphan.Spec.Parameters[longhorn.OrphanDataName]
		if _, ok := replicaDirectoryNames[dirName]; !ok {
			missingOrphanedReplicaDirectoryNames[dirName] = ""
		}
	}

	return newOrphanedReplicaDirectoryNames, missingOrphanedReplicaDirectoryNames
}

func (nc *NodeController) deleteOrphans(node *longhorn.Node, diskName string, diskInfo *monitor.CollectedDiskInfo, missingOrphanedReplicaDirectoryNames map[string]string) error {
	autoDeletionEnabled, err := nc.ds.GetSettingAsBool(types.SettingNameOrphanAutoDeletion)
	if err != nil {
		return errors.Wrapf(err, "failed to get %v setting", types.SettingNameOrphanAutoDeletion)
	}

	for dirName := range missingOrphanedReplicaDirectoryNames {
		orphanName := types.GetOrphanChecksumNameForOrphanedDirectory(node.Name, diskName, diskInfo.Path, diskInfo.DiskUUID, dirName)
		if err := nc.ds.DeleteOrphan(orphanName); err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete orphan %v", orphanName)
		}
	}

	orphans, err := nc.ds.ListOrphans()
	if err != nil {
		return errors.Wrap(err, "unable to list orphans")
	}

	for _, orphan := range orphans {
		if orphan.Status.OwnerID != nc.controllerID {
			continue
		}

		dataCleanableCondition := types.GetCondition(orphan.Status.Conditions, longhorn.OrphanConditionTypeDataCleanable)
		if dataCleanableCondition.Status == longhorn.ConditionStatusUnknown {
			continue
		}

		if autoDeletionEnabled || dataCleanableCondition.Status == longhorn.ConditionStatusFalse {
			if err := nc.ds.DeleteOrphan(orphan.Name); err != nil && !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete orphan %v", orphan.Name)
			}
		}
	}
	return nil
}

func (nc *NodeController) createOrphans(node *longhorn.Node, diskName string, diskInfo *monitor.CollectedDiskInfo, newOrphanedReplicaDirectoryNames map[string]string) error {
	for dirName := range newOrphanedReplicaDirectoryNames {
		if err := nc.createOrphan(node, diskName, dirName, diskInfo); err != nil && !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "unable to create orphan for orphaned replica directory %v in disk %v on node %v",
				dirName, node.Spec.Disks[diskName].Path, node.Name)
		}
	}
	return nil
}

func (nc *NodeController) createOrphan(node *longhorn.Node, diskName, replicaDirectoryName string, diskInfo *monitor.CollectedDiskInfo) error {
	name := types.GetOrphanChecksumNameForOrphanedDirectory(node.Name, diskName, diskInfo.Path, diskInfo.DiskUUID, replicaDirectoryName)

	_, err := nc.ds.GetOrphanRO(name)
	if err == nil || (err != nil && !apierrors.IsNotFound(err)) {
		return err
	}

	orphan := &longhorn.Orphan{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: longhorn.OrphanSpec{
			NodeID: node.Name,
			Type:   longhorn.OrphanTypeReplica,
			Parameters: map[string]string{
				longhorn.OrphanDataName: replicaDirectoryName,
				longhorn.OrphanDiskName: diskName,
				longhorn.OrphanDiskUUID: node.Status.DiskStatus[diskName].DiskUUID,
				longhorn.OrphanDiskPath: node.Spec.Disks[diskName].Path,
			},
		},
	}

	_, err = nc.ds.CreateOrphan(orphan)

	return err
}

func (nc *NodeController) syncWithMonitor(mon monitor.Monitor, node *longhorn.Node) (map[string]*monitor.CollectedDiskInfo, error) {
	v, err := mon.GetCollectedData()
	if err != nil {
		return map[string]*monitor.CollectedDiskInfo{}, err
	}

	collectedDiskInfo := v.(map[string]*monitor.CollectedDiskInfo)
	if matched := isDiskMatched(node, collectedDiskInfo); !matched {
		return map[string]*monitor.CollectedDiskInfo{},
			errors.New("mismatching disks in node resource object and monitor collected data")
	}

	return collectedDiskInfo, nil
}

// Check all disks in the same filesystem ID are in ready status
func (nc *NodeController) isFSIDDuplicatedWithExistingReadyDisk(diskName string, diskInfo map[string]*monitor.CollectedDiskInfo, diskStatusMap map[string]*longhorn.DiskStatus) bool {
	if len(diskInfo) > 1 {
		for otherName := range diskInfo {
			diskReady := types.GetCondition(diskStatusMap[otherName].Conditions, longhorn.DiskConditionTypeReady)

			if otherName != diskName && diskReady.Status == longhorn.ConditionStatusTrue {
				return true
			}
		}
	}

	return false
}

func alignDiskSpecAndStatus(node *longhorn.Node) {
	if node.Status.DiskStatus == nil {
		node.Status.DiskStatus = map[string]*longhorn.DiskStatus{}
	}

	for diskName := range node.Spec.Disks {
		if node.Status.DiskStatus[diskName] == nil {
			node.Status.DiskStatus[diskName] = &longhorn.DiskStatus{}
		}
		diskStatus := node.Status.DiskStatus[diskName]
		if diskStatus.Conditions == nil {
			diskStatus.Conditions = []longhorn.Condition{}
		}
		if diskStatus.ScheduledReplica == nil {
			diskStatus.ScheduledReplica = map[string]int64{}
		}
		// When condition are not ready, the old storage data should be cleaned.
		diskStatus.StorageMaximum = 0
		diskStatus.StorageAvailable = 0
		node.Status.DiskStatus[diskName] = diskStatus
	}

	for diskName := range node.Status.DiskStatus {
		if _, exists := node.Spec.Disks[diskName]; !exists {
			delete(node.Status.DiskStatus, diskName)
		}
	}
}

func isReadyDiskFound(diskInfoMap map[string]*monitor.CollectedDiskInfo) bool {
	return len(diskInfoMap) > 0
}

func isDiskMatched(node *longhorn.Node, collectedDiskInfo map[string]*monitor.CollectedDiskInfo) bool {
	if len(node.Spec.Disks) != len(collectedDiskInfo) {
		logrus.Warnf("number of node disks %v and collected disk info %v are not equal",
			len(node.Spec.Disks), len(collectedDiskInfo))
		return false
	}

	for diskName, diskInfo := range collectedDiskInfo {
		disk, ok := node.Spec.Disks[diskName]
		if !ok {
			logrus.Warnf("disk %v is not found in node %v", diskName, node.Name)
			return false
		}

		nodeOrDiskEvicted := node.Spec.EvictionRequested || disk.EvictionRequested
		if nodeOrDiskEvicted != diskInfo.NodeOrDiskEvicted ||
			disk.Path != diskInfo.Path {
			logrus.Warnf("disk data %v is mismatched with collected data %v for disk %v", disk, diskInfo, diskName)
			return false
		}
	}

	return true
}
