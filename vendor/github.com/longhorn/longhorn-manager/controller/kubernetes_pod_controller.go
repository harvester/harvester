package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	controllerAgentName = "longhorn-kubernetes-pod-controller"
)

type KubernetesPodController struct {
	*baseController

	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced
}

func NewKubernetesPodController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	controllerID string) *KubernetesPodController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	kc := &KubernetesPodController{
		baseController: newBaseController("longhorn-kubernetes-pod", logger),

		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-kubernetes-pod-controller"}),
	}

	ds.PodInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    kc.enqueuePodChange,
		UpdateFunc: func(old, cur interface{}) { kc.enqueuePodChange(cur) },
		DeleteFunc: kc.enqueuePodChange,
	})
	kc.cacheSyncs = append(kc.cacheSyncs, ds.PodInformer.HasSynced)

	return kc
}

func (kc *KubernetesPodController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer kc.queue.ShutDown()

	kc.logger.Infof("Start %v", controllerAgentName)
	defer kc.logger.Infof("Shutting down %v", controllerAgentName)

	if !cache.WaitForNamedCacheSync(controllerAgentName, stopCh, kc.cacheSyncs...) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(kc.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (kc *KubernetesPodController) worker() {
	for kc.processNextWorkItem() {
	}
}

func (kc *KubernetesPodController) processNextWorkItem() bool {
	key, quit := kc.queue.Get()
	if quit {
		return false
	}
	defer kc.queue.Done(key)
	err := kc.syncHandler(key.(string))
	kc.handleErr(err, key)
	return true
}

func (kc *KubernetesPodController) handleErr(err error, key interface{}) {
	if err == nil {
		kc.queue.Forget(key)
		return
	}

	if kc.queue.NumRequeues(key) < maxRetries {
		kc.logger.WithError(err).Warnf("%v: Error syncing Longhorn kubernetes pod %v", controllerAgentName, key)
		kc.queue.AddRateLimited(key)
		return
	}

	kc.logger.WithError(err).Warnf("%v: Dropping Longhorn kubernetes pod %v out of the queue", controllerAgentName, key)
	kc.queue.Forget(key)
	utilruntime.HandleError(err)
}

func (kc *KubernetesPodController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: fail to sync %v", controllerAgentName, key)
	}()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	pod, err := kc.ds.GetPodRO(namespace, name)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "Error getting Pod: %s", name)
	}
	nodeID := pod.Spec.NodeName
	if nodeID == "" {
		kc.logger.WithField("pod", pod.Name).Trace("skipping pod check since pod is not scheduled yet")
		return nil
	}
	if err := kc.handlePodDeletionIfNodeDown(pod, nodeID, namespace); err != nil {
		return err
	}

	if err := kc.handlePodDeletionIfVolumeRequestRemount(pod); err != nil {
		return err
	}

	return nil
}

// handlePodDeletionIfNodeDown determines whether we are allowed to forcefully delete a pod
// from a failed node based on the users chosen NodeDownPodDeletionPolicy.
// This is necessary because Kubernetes never forcefully deletes pods on a down node,
// the pods are stuck in terminating state forever and Longhorn volumes are not released.
// We provide an option for users to help them automatically force delete terminating pods
// of StatefulSet/Deployment on the downed node. By force deleting, k8s will detach Longhorn volumes
// and spin up replacement pods on a new node.
//
// Force delete a pod when all of the below conditions are meet:
// 1. NodeDownPodDeletionPolicy is different than DoNothing
// 2. pod belongs to a StatefulSet/Deployment depend on NodeDownPodDeletionPolicy
// 3. node containing the pod is down
// 4. the pod is terminating and the DeletionTimestamp has passed.
// 5. pod has a PV with provisioner driver.longhorn.io
func (kc *KubernetesPodController) handlePodDeletionIfNodeDown(pod *v1.Pod, nodeID string, namespace string) error {
	deletionPolicy := types.NodeDownPodDeletionPolicyDoNothing
	if deletionSetting, err := kc.ds.GetSettingValueExisted(types.SettingNameNodeDownPodDeletionPolicy); err == nil {
		deletionPolicy = types.NodeDownPodDeletionPolicy(deletionSetting)
	}

	shouldDelete := (deletionPolicy == types.NodeDownPodDeletionPolicyDeleteStatefulSetPod && isOwnedByStatefulSet(pod)) ||
		(deletionPolicy == types.NodeDownPodDeletionPolicyDeleteDeploymentPod && isOwnedByDeployment(pod)) ||
		(deletionPolicy == types.NodeDownPodDeletionPolicyDeleteBothStatefulsetAndDeploymentPod && (isOwnedByStatefulSet(pod) || isOwnedByDeployment(pod)))

	if !shouldDelete {
		return nil
	}

	isNodeDown, err := kc.ds.IsNodeDownOrDeleted(nodeID)
	if err != nil {
		return errors.Wrapf(err, "failed to evaluate Node %v for pod %v in handlePodDeletionIfNodeDown", nodeID, pod.Name)
	}
	if !isNodeDown {
		return nil
	}

	if pod.DeletionTimestamp == nil {
		return nil
	}

	// make sure the volumeattachments of the pods are gone first
	// ref: https://github.com/longhorn/longhorn/issues/2947
	vas, err := kc.getVolumeAttachmentsOfPod(pod)
	if err != nil {
		return err
	}
	for _, va := range vas {
		if va.DeletionTimestamp == nil {
			err := kc.kubeClient.StorageV1().VolumeAttachments().Delete(context.TODO(), va.Name, metav1.DeleteOptions{})
			if err != nil {
				if datastore.ErrorIsNotFound(err) {
					continue
				}
				return err
			}
			kc.logger.Infof("%v: deleted volume attachment %v for pod %v on downed node %v", controllerAgentName, va.Name, pod.Name, nodeID)
		}
		// wait the volumeattachment object to be deleted
		kc.logger.Infof("%v: wait for volume attachment %v for pod %v on downed node %v to be deleted", controllerAgentName, va.Name, pod.Name, nodeID)
		return nil
	}

	if pod.DeletionTimestamp.After(time.Now()) {
		return nil
	}

	gracePeriod := int64(0)
	err = kc.kubeClient.CoreV1().Pods(namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to forcefully delete Pod %v on the downed Node %v in handlePodDeletionIfNodeDown", pod.Name, nodeID)
	}
	kc.logger.Infof("%v: Forcefully deleted pod %v on downed node %v", controllerAgentName, pod.Name, nodeID)

	return nil
}

func (kc *KubernetesPodController) getVolumeAttachmentsOfPod(pod *v1.Pod) ([]*storagev1.VolumeAttachment, error) {
	res := []*storagev1.VolumeAttachment{}
	vas, err := kc.ds.ListVolumeAttachmentsRO()
	if err != nil {
		return nil, err
	}

	pvs := make(map[string]bool)

	for _, vol := range pod.Spec.Volumes {
		if vol.VolumeSource.PersistentVolumeClaim == nil {
			continue
		}

		pvc, err := kc.ds.GetPersistentVolumeClaimRO(pod.Namespace, vol.VolumeSource.PersistentVolumeClaim.ClaimName)
		if err != nil {
			if datastore.ErrorIsNotFound(err) {
				continue
			}
			return nil, err
		}
		pvs[pvc.Spec.VolumeName] = true
	}

	for _, va := range vas {
		if va.Spec.NodeName != pod.Spec.NodeName {
			continue
		}
		if va.Spec.Attacher != types.LonghornDriverName {
			continue
		}
		if va.Spec.Source.PersistentVolumeName == nil {
			continue
		}
		if _, ok := pvs[*va.Spec.Source.PersistentVolumeName]; !ok {
			continue
		}
		res = append(res, va)
	}

	return res, nil
}

// handlePodDeletionIfVolumeRequestRemount will delete the pod which is using a volume that has requested remount.
// By deleting the consuming pod, Kubernetes will recreated them, reattaches, and remounts the volume.
func (kc *KubernetesPodController) handlePodDeletionIfVolumeRequestRemount(pod *v1.Pod) error {
	// Only handle pod that is on the same node as this manager
	if pod.Spec.NodeName != kc.controllerID {
		return nil
	}

	autoDeletePodWhenVolumeDetachedUnexpectedly, err := kc.ds.GetSettingAsBool(types.SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly)
	if err != nil {
		return err
	}
	if !autoDeletePodWhenVolumeDetachedUnexpectedly {
		return nil
	}

	// Only delete pod which has controller to make sure that the pod will be recreated by its controller
	if metav1.GetControllerOf(pod) == nil {
		return nil
	}

	volumeList, err := kc.getAssociatedVolumes(pod)
	if err != nil {
		return err
	}

	// Only delete pod which has startTime < vol.Status.RemountRequestAt AND timeNow > vol.Status.RemountRequestAt + delayDuration
	// The delayDuration is to make sure that we don't repeatedly delete the pod too fast
	// when vol.Status.RemountRequestAt is updated too quickly by volumeController
	if pod.Status.StartTime == nil {
		return nil
	}

	// Avoid repeat deletion
	if pod.DeletionTimestamp != nil {
		return nil
	}

	podStartTime := pod.Status.StartTime.Time
	for _, vol := range volumeList {
		if vol.Status.RemountRequestedAt == "" {
			continue
		}
		remountRequestedAt, err := time.Parse(time.RFC3339, vol.Status.RemountRequestedAt)
		if err != nil {
			return err
		}

		timeNow := time.Now()
		delayDuration := time.Duration(int64(5)) * time.Second

		if podStartTime.Before(remountRequestedAt) && timeNow.After(remountRequestedAt.Add(delayDuration)) {
			gracePeriod := int64(30)
			err := kc.kubeClient.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.GetName(), metav1.DeleteOptions{
				GracePeriodSeconds: &gracePeriod,
			})
			if err != nil && !datastore.ErrorIsNotFound(err) {
				return err
			}
			kc.logger.Infof("Deleted pod %v so that Kubernetes will handle remounting volume %v", pod.GetName(), vol.GetName())
			return nil
		}

	}

	return nil
}

func isOwnedByStatefulSet(pod *v1.Pod) bool {
	if ownerRef := metav1.GetControllerOf(pod); ownerRef != nil {
		return ownerRef.Kind == types.KubernetesStatefulSet
	}
	return false
}

func isOwnedByDeployment(pod *v1.Pod) bool {
	if ownerRef := metav1.GetControllerOf(pod); ownerRef != nil {
		return ownerRef.Kind == types.KubernetesReplicaSet
	}
	return false
}

// enqueuePodChange determines if the pod requires processing based on whether the pod has a PV created by us (driver.longhorn.io)
func (kc *KubernetesPodController) enqueuePodChange(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

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
		if v.VolumeSource.PersistentVolumeClaim == nil {
			continue
		}

		pvc, err := kc.ds.GetPersistentVolumeClaimRO(pod.Namespace, v.VolumeSource.PersistentVolumeClaim.ClaimName)
		if datastore.ErrorIsNotFound(err) {
			continue
		}
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", pvc, err))
			return
		}

		pv, err := kc.getAssociatedPersistentVolume(pvc)
		if datastore.ErrorIsNotFound(err) {
			continue
		}
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("error getting Persistent Volume for PVC: %v", pvc))
			return
		}

		if pv.Spec.CSI != nil && pv.Spec.CSI.Driver == types.LonghornDriverName {
			kc.queue.Add(key)
			break
		}
	}
}

func (kc *KubernetesPodController) getAssociatedPersistentVolume(pvc *v1.PersistentVolumeClaim) (*v1.PersistentVolume, error) {
	pvName := pvc.Spec.VolumeName
	return kc.ds.GetPersistentVolumeRO(pvName)
}

func (kc *KubernetesPodController) getAssociatedVolumes(pod *v1.Pod) ([]*longhorn.Volume, error) {
	var volumeList []*longhorn.Volume
	for _, v := range pod.Spec.Volumes {
		if v.VolumeSource.PersistentVolumeClaim == nil {
			continue
		}

		pvc, err := kc.ds.GetPersistentVolumeClaimRO(pod.Namespace, v.VolumeSource.PersistentVolumeClaim.ClaimName)
		if datastore.ErrorIsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}

		pv, err := kc.getAssociatedPersistentVolume(pvc)
		if datastore.ErrorIsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}

		if pv.Spec.CSI != nil && pv.Spec.CSI.Driver == types.LonghornDriverName {
			vol, err := kc.ds.GetVolume(pv.GetName())
			if err != nil {
				return nil, err
			}
			volumeList = append(volumeList, vol)
		}
	}

	return volumeList, nil
}
