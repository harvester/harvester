package controller

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
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

	cfmStoreSynced cache.InformerSynced
}

func NewKubernetesConfigMapController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	configMapInformer coreinformers.ConfigMapInformer,
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

		cfmStoreSynced: configMapInformer.Informer().HasSynced,
	}

	configMapInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    kc.enqueueConfigMapChange,
		UpdateFunc: func(old, cur interface{}) { kc.enqueueConfigMapChange(cur) },
		DeleteFunc: kc.enqueueConfigMapChange,
	})

	return kc
}

func (kc *KubernetesConfigMapController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer kc.queue.ShutDown()

	kc.logger.Infof("Start")
	defer kc.logger.Infof("Shutting down")

	if !cache.WaitForNamedCacheSync(kc.name, stopCh, kc.cfmStoreSynced) {
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

	if err := kc.reconcileDefaultStorageClass(namespace, cfmName); err != nil {
		return err
	}

	return nil
}

func (kc *KubernetesConfigMapController) reconcileDefaultStorageClass(namespace, cfmName string) error {
	if namespace != kc.namespace || cfmName != types.DefaultStorageClassConfigMapName {
		return nil
	}

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
