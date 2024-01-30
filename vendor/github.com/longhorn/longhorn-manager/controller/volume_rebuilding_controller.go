package controller

import (
	"fmt"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/longhorn/longhorn-manager/constant"
	"github.com/longhorn/longhorn-manager/datastore"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type VolumeRebuildingController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds         *datastore.DataStore
	cacheSyncs []cache.InformerSynced
}

func NewVolumeRebuildingController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	controllerID string,
	namespace string,
) *VolumeRebuildingController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)

	vbc := &VolumeRebuildingController{
		baseController: newBaseController("longhorn-volume-rebuilding", logger),

		namespace:    namespace,
		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "longhorn-volume-rebuilding-controller"}),
	}

	ds.VolumeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    vbc.enqueueVolume,
		UpdateFunc: func(old, cur interface{}) { vbc.enqueueVolume(cur) },
		DeleteFunc: vbc.enqueueVolume,
	}, 0)
	vbc.cacheSyncs = append(vbc.cacheSyncs, ds.VolumeInformer.HasSynced)

	return vbc
}

func (vbc *VolumeRebuildingController) enqueueVolume(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to get key for object %#v: %v", obj, err))
		return
	}

	vbc.queue.Add(key)
}

func (vbc *VolumeRebuildingController) enqueueVolumeAfter(obj interface{}, duration time.Duration) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("enqueueVolumeAfter: failed to get key for object %#v: %v", obj, err))
		return
	}

	vbc.queue.AddAfter(key, duration)
}

func (vbc *VolumeRebuildingController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer vbc.queue.ShutDown()

	vbc.logger.Infof("Start Longhorn rebuilding controller")
	defer vbc.logger.Infof("Shutting down Longhorn rebuilding controller")

	if !cache.WaitForNamedCacheSync(vbc.name, stopCh, vbc.cacheSyncs...) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(vbc.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (vbc *VolumeRebuildingController) worker() {
	for vbc.processNextWorkItem() {
	}
}

func (vbc *VolumeRebuildingController) processNextWorkItem() bool {
	key, quit := vbc.queue.Get()
	if quit {
		return false
	}
	defer vbc.queue.Done(key)
	err := vbc.syncHandler(key.(string))
	vbc.handleErr(err, key)
	return true
}

func (vbc *VolumeRebuildingController) handleErr(err error, key interface{}) {
	if err == nil {
		vbc.queue.Forget(key)
		return
	}

	log := vbc.logger.WithField("Volume", key)
	handleReconcileErrorLogging(log, err, "Failed to sync Longhorn volume")
	vbc.queue.AddRateLimited(key)
}

func (vbc *VolumeRebuildingController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: failed to sync volume %v", vbc.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != vbc.namespace {
		return nil
	}
	return vbc.reconcile(name)
}

func (vbc *VolumeRebuildingController) reconcile(volName string) (err error) {
	vol, err := vbc.ds.GetVolume(volName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	if !vbc.isResponsibleFor(vol) {
		return nil
	}

	va, err := vbc.ds.GetLHVolumeAttachmentByVolumeName(volName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		vbc.enqueueVolumeAfter(vol, constant.LonghornVolumeAttachmentNotFoundRetryPeriod)
		return nil
	}
	existingVA := va.DeepCopy()
	defer func() {
		if err != nil {
			return
		}
		if reflect.DeepEqual(existingVA.Spec, va.Spec) {
			return
		}

		if _, err = vbc.ds.UpdateLHVolumeAttachment(va); err != nil {
			return
		}
	}()

	rebuildingAttachmentTicketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeVolumeRebuildingController, volName)

	if vol.Status.OfflineReplicaRebuildingRequired {
		createOrUpdateAttachmentTicket(va, rebuildingAttachmentTicketID, vol.Status.OwnerID, longhorn.TrueValue, longhorn.AttacherTypeVolumeRebuildingController)
	} else {
		delete(va.Spec.AttachmentTickets, rebuildingAttachmentTicketID)
	}

	return nil
}

func (vbc *VolumeRebuildingController) isResponsibleFor(vol *longhorn.Volume) bool {
	return vbc.controllerID == vol.Status.OwnerID
}
