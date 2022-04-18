package controller

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
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
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
)

type KubernetesPVController struct {
	*baseController

	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced

	// key is <PVName>, value is <VolumeName>
	pvToVolumeCache sync.Map

	// for unit test
	nowHandler func() string
}

func NewKubernetesPVController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	controllerID string) *KubernetesPVController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events("")})

	kc := &KubernetesPVController{
		baseController: newBaseController("longhorn-kubernetes-pv", logger),

		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-kubernetes-pv-controller"}),

		pvToVolumeCache: sync.Map{},

		nowHandler: util.Now,
	}

	ds.PersistentVolumeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    kc.enqueuePersistentVolume,
		UpdateFunc: func(old, cur interface{}) { kc.enqueuePersistentVolume(cur) },
		DeleteFunc: func(obj interface{}) {
			kc.enqueuePersistentVolume(obj)
			kc.enqueuePVDeletion(obj)
		},
	})
	kc.cacheSyncs = append(kc.cacheSyncs, ds.PersistentVolumeInformer.HasSynced)

	ds.PodInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    kc.enqueuePodChange,
		UpdateFunc: func(old, cur interface{}) { kc.enqueuePodChange(cur) },
		DeleteFunc: kc.enqueuePodChange,
	}, 0)
	kc.cacheSyncs = append(kc.cacheSyncs, ds.PodInformer.HasSynced)

	return kc
}

func (kc *KubernetesPVController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer kc.queue.ShutDown()

	logrus.Infof("Start kubernetes controller")
	defer logrus.Infof("Shutting down kubernetes controller")

	if !cache.WaitForNamedCacheSync("kubernetes", stopCh, kc.cacheSyncs...) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(kc.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (kc *KubernetesPVController) worker() {
	for kc.processNextWorkItem() {
	}
}

func (kc *KubernetesPVController) processNextWorkItem() bool {
	key, quit := kc.queue.Get()

	if quit {
		return false
	}
	defer kc.queue.Done(key)

	err := kc.syncKubernetesStatus(key.(string))
	kc.handleErr(err, key)

	return true
}

func (kc *KubernetesPVController) handleErr(err error, key interface{}) {
	if err == nil {
		kc.queue.Forget(key)
		return
	}

	if kc.queue.NumRequeues(key) < maxRetries {
		logrus.Warnf("Error syncing Longhorn volume kubernetes status %v: %v", key, err)
		kc.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	logrus.Warnf("Dropping Persistent Volume %v out of the queue: %v", key, err)
	kc.queue.Forget(key)
}

func (kc *KubernetesPVController) syncKubernetesStatus(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "kubernetes-controller: fail to sync %v", key)
	}()
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	ok, err := kc.cleanupForPVDeletion(name)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	pv, err := kc.ds.GetPersistentVolumeRO(name)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "Error getting Persistent Volume %s", name)
	}

	volumeName := kc.getCSIVolumeHandleFromPV(pv)
	if volumeName == "" {
		return nil
	}

	volume, err := kc.ds.GetVolume(volumeName)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			return nil
		}
		return err
	}

	if volume.Status.OwnerID != kc.controllerID {
		return nil
	}

	existingVolume := volume.DeepCopy()
	defer func() {
		// we're going to update volume assume things changes
		if err == nil && !reflect.DeepEqual(existingVolume.Status, volume.Status) {
			_, err = kc.ds.UpdateVolumeStatus(volume)
		}
		// requeue if it's conflict
		if apierrors.IsConflict(errors.Cause(err)) {
			logrus.Debugf("Requeue for volume %v due to conflict: %v", volumeName, err)
			kc.enqueueVolumeChange(volume)
			err = nil
		}
	}()

	// existing volume may be used/reused by pv
	if volume.Status.KubernetesStatus.PVName != name {
		volume.Status.KubernetesStatus = longhorn.KubernetesStatus{}
		kc.eventRecorder.Eventf(volume, v1.EventTypeNormal, EventReasonStart, "Persistent Volume %v started to use/reuse Longhorn volume %v", volume.Name, name)
	}
	ks := &volume.Status.KubernetesStatus

	lastPVStatus := ks.PVStatus

	ks.PVName = name
	ks.PVStatus = string(pv.Status.Phase)

	if pv.Spec.ClaimRef != nil {
		if pv.Status.Phase == v1.VolumeBound {
			// set for bounded PVC
			ks.PVCName = pv.Spec.ClaimRef.Name
			ks.Namespace = pv.Spec.ClaimRef.Namespace
			ks.LastPVCRefAt = ""
		} else if lastPVStatus == string(v1.VolumeBound) && ks.LastPVCRefAt == "" {
			// PVC is no longer bound with PV. indicate historic data by setting <LastPVCRefAt>
			ks.LastPVCRefAt = kc.nowHandler()
			if len(ks.WorkloadsStatus) != 0 && ks.LastPodRefAt == "" {
				ks.LastPodRefAt = kc.nowHandler()
			}
		}
	} else {
		if ks.LastPVCRefAt == "" {
			if pv.Status.Phase == v1.VolumeBound {
				return fmt.Errorf("BUG: current Persistent Volume %v is in Bound phase but has no ClaimRef field", pv.Name)
			}
			// The associated PVC is removed from the PV ClaimRef
			if ks.PVCName != "" {
				ks.LastPVCRefAt = kc.nowHandler()
				if len(ks.WorkloadsStatus) != 0 && ks.LastPodRefAt == "" {
					ks.LastPodRefAt = kc.nowHandler()
				}
			}
		}
	}

	pods, err := kc.getAssociatedPods(ks)
	if err != nil {
		return err
	}
	kc.setWorkloads(ks, pods)

	return nil
}

func (kc *KubernetesPVController) getCSIVolumeHandleFromPV(pv *v1.PersistentVolume) string {
	if pv == nil {
		return ""
	}
	// try to get associated Longhorn volume name
	if pv.Spec.CSI == nil || pv.Spec.CSI.VolumeHandle == "" || (pv.Spec.CSI.Driver != types.LonghornDriverName && pv.Spec.CSI.Driver != types.DepracatedDriverName) {
		return ""
	}
	return pv.Spec.CSI.VolumeHandle
}

func (kc *KubernetesPVController) enqueuePersistentVolume(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}
	kc.queue.Add(key)

}

func (kc *KubernetesPVController) enqueuePodChange(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
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

	for _, v := range pod.Spec.Volumes {
		claim := v.VolumeSource.PersistentVolumeClaim
		if claim == nil {
			continue
		}

		pvc, err := kc.ds.GetPersistentVolumeClaimRO(pod.Namespace, claim.ClaimName)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				utilruntime.HandleError(fmt.Errorf("couldn't get pvc %#v: %v", claim.ClaimName, err))
				return
			}
			continue
		}

		if pvName := pvc.Spec.VolumeName; pvName != "" {
			kc.queue.Add(pvName)
		}
	}

}

func (kc *KubernetesPVController) enqueueVolumeChange(obj interface{}) {
	volume, ok := obj.(*longhorn.Volume)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		volume, ok = deletedState.Obj.(*longhorn.Volume)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	if volume.Status.State != longhorn.VolumeStateDetached {
		return
	}
	ks := volume.Status.KubernetesStatus
	if ks.PVName != "" && ks.PVStatus == string(v1.VolumeBound) &&
		ks.LastPodRefAt == "" {
		kc.queue.Add(volume.Status.KubernetesStatus.PVName)
	}

}

func (kc *KubernetesPVController) enqueuePVDeletion(obj interface{}) {
	pv, ok := obj.(*v1.PersistentVolume)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		pv, ok = deletedState.Obj.(*v1.PersistentVolume)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	if pv.Spec.CSI != nil && pv.Spec.CSI.VolumeHandle != "" {
		kc.pvToVolumeCache.Store(pv.Name, pv.Spec.CSI.VolumeHandle)
	}

}

func (kc *KubernetesPVController) cleanupForPVDeletion(pvName string) (bool, error) {
	volumeName, ok := kc.pvToVolumeCache.Load(pvName)
	if !ok {
		return false, nil
	}
	volume, err := kc.ds.GetVolume(volumeName.(string))
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			kc.pvToVolumeCache.Delete(pvName)
			return true, nil
		}
		return false, errors.Wrapf(err, "failed to get volume for cleanup in cleanupForPVDeletion")
	}
	if kc.controllerID != volume.Status.OwnerID {
		kc.pvToVolumeCache.Delete(pvName)
		return true, nil
	}
	pv, err := kc.ds.GetPersistentVolumeRO(pvName)
	if err != nil && !datastore.ErrorIsNotFound(err) {
		return false, errors.Wrapf(err, "failed to get associated pv in cleanupForPVDeletion")
	}
	if datastore.ErrorIsNotFound(err) || pv.DeletionTimestamp != nil {
		ks := &volume.Status.KubernetesStatus
		if ks.PVCName != "" && ks.LastPVCRefAt == "" {
			volume.Status.KubernetesStatus.LastPVCRefAt = kc.nowHandler()
		}
		if len(ks.WorkloadsStatus) != 0 && ks.LastPodRefAt == "" {
			volume.Status.KubernetesStatus.LastPodRefAt = kc.nowHandler()
		}
		volume.Status.KubernetesStatus.PVName = ""
		volume.Status.KubernetesStatus.PVStatus = ""
		volume, err = kc.ds.UpdateVolumeStatus(volume)
		if err != nil {
			return false, errors.Wrapf(err, "failed to update volume in cleanupForPVDeletion")
		}
		kc.eventRecorder.Eventf(volume, v1.EventTypeNormal, EventReasonStop, "Persistent Volume %v stopped to use Longhorn volume %v", pvName, volume.Name)
	}
	kc.pvToVolumeCache.Delete(pvName)
	return true, nil
}

// filterPods includes only the pods where the passed predicate returns true
func filterPods(pods []*v1.Pod, predicate func(pod *v1.Pod) bool) (filtered []*v1.Pod) {
	for _, p := range pods {
		if predicate(p) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// getAssociatedPods returns all pods using this pvc in sorted order based on pod name
func (kc *KubernetesPVController) getAssociatedPods(ks *longhorn.KubernetesStatus) ([]*v1.Pod, error) {
	if ks.PVStatus != string(v1.VolumeBound) {
		return nil, nil
	}
	ps, err := kc.ds.ListPodsRO(ks.Namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list pods in getAssociatedPod")
	}

	pods := filterPods(ps, func(pod *v1.Pod) bool {
		for _, v := range pod.Spec.Volumes {
			if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == ks.PVCName {
				return true
			}
		}
		return false
	})

	// we sort the pods to normalize their order based on pod name
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].Name < pods[j].Name
	})

	return pods, nil
}

func (kc *KubernetesPVController) setWorkloads(ks *longhorn.KubernetesStatus, pods []*v1.Pod) {
	if len(pods) == 0 {
		if len(ks.WorkloadsStatus) == 0 || ks.LastPodRefAt != "" {
			return
		}
		ks.LastPodRefAt = kc.nowHandler()
		return
	}

	ks.WorkloadsStatus = []longhorn.WorkloadStatus{}
	ks.LastPodRefAt = ""
	for _, p := range pods {
		ws := longhorn.WorkloadStatus{
			PodName:   p.Name,
			PodStatus: string(p.Status.Phase),
		}
		ws.WorkloadName, ws.WorkloadType = kc.detectWorkload(p)
		ks.WorkloadsStatus = append(ks.WorkloadsStatus, ws)
	}

}

func (kc *KubernetesPVController) detectWorkload(p *v1.Pod) (string, string) {
	refs := p.GetObjectMeta().GetOwnerReferences()
	for _, ref := range refs {
		if ref.Name != "" && ref.Kind != "" {
			return ref.Name, ref.Kind
		}
	}
	return "", ""
}
