package controller

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"
)

const (
	lastAppliedStorageConfigLabelKeySuffix = "last-applied-configmap"
)

type KubernetesConfigMapController struct {
	*baseController

	namespace    string
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced
}

func NewKubernetesConfigMapController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	controllerID string,
	namespace string) *KubernetesConfigMapController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	kc := &KubernetesConfigMapController{
		baseController: newBaseController("longhorn-kubernetes-configmap-controller", logger),

		namespace:    namespace,
		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-kubernetes-configmap-controller"}),
	}

	ds.ConfigMapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    kc.enqueueConfigMapChange,
		UpdateFunc: func(old, cur interface{}) { kc.enqueueConfigMapChange(cur) },
		DeleteFunc: kc.enqueueConfigMapChange,
	})

	ds.StorageClassInformer.AddEventHandlerWithResyncPeriod(
		cache.FilteringResourceEventHandler{
			FilterFunc: isLonghornStorageClass,
			Handler: cache.ResourceEventHandlerFuncs{
				UpdateFunc: func(old, cur interface{}) { kc.enqueueConfigMapForStorageClassChange(cur) },
				DeleteFunc: kc.enqueueConfigMapForStorageClassChange,
			},
		}, 0)

	kc.cacheSyncs = append(kc.cacheSyncs, ds.ConfigMapInformer.HasSynced, ds.StorageClassInformer.HasSynced)

	return kc
}

func (kc *KubernetesConfigMapController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer kc.queue.ShutDown()

	kc.logger.Infof("Start")
	defer kc.logger.Infof("Shutting down")

	if !cache.WaitForNamedCacheSync(kc.name, stopCh, kc.cacheSyncs...) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(kc.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (kc *KubernetesConfigMapController) worker() {
	for kc.processNextWorkItem() {
	}
}

func (kc *KubernetesConfigMapController) processNextWorkItem() bool {
	key, quit := kc.queue.Get()
	if quit {
		return false
	}
	defer kc.queue.Done(key)
	err := kc.syncHandler(key.(string))
	kc.handleErr(err, key)
	return true
}

func (kc *KubernetesConfigMapController) handleErr(err error, key interface{}) {
	if err == nil {
		kc.queue.Forget(key)
		return
	}

	if kc.queue.NumRequeues(key) < maxRetries {
		kc.logger.WithError(err).Warnf("Error syncing ConfigMap %v", key)
		kc.queue.AddRateLimited(key)
		return
	}

	kc.logger.WithError(err).Warnf("Dropping ConfigMap %v out of the queue", key)
	kc.queue.Forget(key)
	utilruntime.HandleError(err)
}

func (kc *KubernetesConfigMapController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: fail to sync %v", kc.name, key)
	}()

	namespace, cfmName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	if err := kc.reconcile(namespace, cfmName); err != nil {
		return err
	}

	return nil
}

func (kc *KubernetesConfigMapController) reconcile(namespace, cfmName string) error {
	if namespace != kc.namespace {
		return nil
	}

	switch cfmName {
	case types.DefaultStorageClassConfigMapName:
		storageCFM, err := kc.ds.GetConfigMap(kc.namespace, types.DefaultStorageClassConfigMapName)
		if err != nil {
			return err
		}

		storageclassYAML, ok := storageCFM.Data["storageclass.yaml"]
		if !ok {
			return fmt.Errorf("cannot find storageclass.yaml inside the default StorageClass ConfigMap")
		}

		existingSC, err := kc.ds.GetStorageClassRO(types.DefaultStorageClassName)
		if err != nil && !datastore.ErrorIsNotFound(err) {
			return err
		}

		if !needToUpdateStorageClass(storageclassYAML, existingSC) {
			return nil
		}

		storageclass, err := buildStorageClassManifestFromYAMLString(storageclassYAML)
		if err != nil {
			return err
		}

		err = kc.ds.DeleteStorageClass(types.DefaultStorageClassName)
		if err != nil && !datastore.ErrorIsNotFound(err) {
			return err
		}

		storageclass, err = kc.ds.CreateStorageClass(storageclass)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				return nil
			}
			return err
		}

		kc.logger.Infof("Updated the default Longhorn StorageClass: %v", storageclass)
	case types.DefaultDefaultSettingConfigMapName:
		if err := kc.ds.UpdateCustomizedSettings(nil); err != nil {
			return errors.Wrap(err, "failed to update built-in settings with customized values")
		}
	}

	return nil
}

func buildStorageClassManifestFromYAMLString(storageclassYAML string) (*storagev1.StorageClass, error) {
	decode := scheme.Codecs.UniversalDeserializer().Decode
	obj, _, err := decode([]byte(storageclassYAML), nil, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error while decoding YAML string")
	}

	storageclass, ok := obj.(*storagev1.StorageClass)
	if !ok {
		return nil, fmt.Errorf("invalid storageclass YAML string: %v", storageclassYAML)
	}

	if storageclass.Annotations == nil {
		storageclass.Annotations = make(map[string]string)
	}
	storageclass.Annotations[types.GetLonghornLabelKey(lastAppliedStorageConfigLabelKeySuffix)] = storageclassYAML

	return storageclass, nil
}

func needToUpdateStorageClass(storageclassYAML string, existingSC *storagev1.StorageClass) bool {
	// If the default StorageClass doesn't exist, need to create it
	if existingSC == nil {
		return true
	}

	lastAppliedConfiguration, ok := existingSC.Annotations[types.GetLonghornLabelKey(lastAppliedStorageConfigLabelKeySuffix)]
	if !ok { //First time creation using the default StorageClass ConfigMap
		return true
	}

	return lastAppliedConfiguration != storageclassYAML
}

func (kc *KubernetesConfigMapController) enqueueConfigMapChange(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	kc.queue.Add(key)
}

func (kc *KubernetesConfigMapController) enqueueConfigMapForStorageClassChange(obj interface{}) {
	kc.queue.Add(kc.namespace + "/" + types.DefaultStorageClassConfigMapName)
}

func isLonghornStorageClass(obj interface{}) bool {
	sc, isSC := obj.(*storagev1.StorageClass)
	if !isSC {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return false
		}

		sc, ok = deletedState.Obj.(*storagev1.StorageClass)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained non StorageClass object: %#v", deletedState.Obj))
			return false
		}
	}

	return sc.Name == types.DefaultStorageClassName && sc.Provisioner == types.LonghornDriverName
}
