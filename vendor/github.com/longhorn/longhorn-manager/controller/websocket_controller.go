package controller

import (
	"sync"

	"github.com/sirupsen/logrus"

	"k8s.io/client-go/tools/cache"

	lhinformers "github.com/longhorn/longhorn-manager/k8s/pkg/client/informers/externalversions/longhorn/v1beta1"
)

type SimpleResourceEventHandler struct{ ChangeFunc func() }

func (s SimpleResourceEventHandler) OnAdd(obj interface{})               { s.ChangeFunc() }
func (s SimpleResourceEventHandler) OnUpdate(oldObj, newObj interface{}) { s.ChangeFunc() }
func (s SimpleResourceEventHandler) OnDelete(obj interface{})            { s.ChangeFunc() }

type Watcher struct {
	eventChan  chan struct{}
	resources  []string
	controller *WebsocketController
}

func (w *Watcher) Events() <-chan struct{} {
	return w.eventChan
}

func (w *Watcher) Close() {
	close(w.eventChan)
}

type WebsocketController struct {
	*baseController
	volumeSynced       cache.InformerSynced
	engineSynced       cache.InformerSynced
	replicaSynced      cache.InformerSynced
	settingSynced      cache.InformerSynced
	engineImageSynced  cache.InformerSynced
	backingImageSynced cache.InformerSynced
	nodeSynced         cache.InformerSynced
	backupTargetSynced cache.InformerSynced
	backupVolumeSynced cache.InformerSynced
	backupSynced       cache.InformerSynced
	recurringJobSynced cache.InformerSynced

	watchers    []*Watcher
	watcherLock sync.Mutex
}

func NewWebsocketController(
	logger logrus.FieldLogger,
	volumeInformer lhinformers.VolumeInformer,
	engineInformer lhinformers.EngineInformer,
	replicaInformer lhinformers.ReplicaInformer,
	settingInformer lhinformers.SettingInformer,
	engineImageInformer lhinformers.EngineImageInformer,
	backingImageInformer lhinformers.BackingImageInformer,
	nodeInformer lhinformers.NodeInformer,
	backupTargetInformer lhinformers.BackupTargetInformer,
	backupVolumeInformer lhinformers.BackupVolumeInformer,
	backupInformer lhinformers.BackupInformer,
	recurringJobInformer lhinformers.RecurringJobInformer,
) *WebsocketController {

	wc := &WebsocketController{
		baseController:     newBaseController("longhorn-websocket", logger),
		volumeSynced:       volumeInformer.Informer().HasSynced,
		engineSynced:       engineInformer.Informer().HasSynced,
		replicaSynced:      replicaInformer.Informer().HasSynced,
		settingSynced:      settingInformer.Informer().HasSynced,
		engineImageSynced:  engineImageInformer.Informer().HasSynced,
		backingImageSynced: backingImageInformer.Informer().HasSynced,
		nodeSynced:         nodeInformer.Informer().HasSynced,
		backupTargetSynced: backupTargetInformer.Informer().HasSynced,
		backupVolumeSynced: backupVolumeInformer.Informer().HasSynced,
		backupSynced:       backupInformer.Informer().HasSynced,
		recurringJobSynced: recurringJobInformer.Informer().HasSynced,
	}

	volumeInformer.Informer().AddEventHandler(wc.notifyWatchersHandler("volume"))
	engineInformer.Informer().AddEventHandler(wc.notifyWatchersHandler("engine"))
	replicaInformer.Informer().AddEventHandler(wc.notifyWatchersHandler("replica"))
	settingInformer.Informer().AddEventHandler(wc.notifyWatchersHandler("setting"))
	engineImageInformer.Informer().AddEventHandler(wc.notifyWatchersHandler("engineImage"))
	backingImageInformer.Informer().AddEventHandler(wc.notifyWatchersHandler("backingImage"))
	nodeInformer.Informer().AddEventHandler(wc.notifyWatchersHandler("node"))
	backupTargetInformer.Informer().AddEventHandler(wc.notifyWatchersHandler("backupTarget"))
	backupVolumeInformer.Informer().AddEventHandler(wc.notifyWatchersHandler("backupVolume"))
	backupInformer.Informer().AddEventHandler(wc.notifyWatchersHandler("backup"))
	recurringJobInformer.Informer().AddEventHandler(wc.notifyWatchersHandler("recurringJob"))

	return wc
}

func (wc *WebsocketController) NewWatcher(resources ...string) *Watcher {
	wc.watcherLock.Lock()
	defer wc.watcherLock.Unlock()

	w := &Watcher{
		eventChan:  make(chan struct{}, 2),
		resources:  resources,
		controller: wc,
	}
	wc.watchers = append(wc.watchers, w)
	return w
}

func (wc *WebsocketController) Run(stopCh <-chan struct{}) {
	defer wc.Close()

	wc.logger.Infof("Start Longhorn websocket controller")
	defer wc.logger.Infof("Shutting down Longhorn websocket controller")

	if !cache.WaitForNamedCacheSync("longhorn websocket", stopCh,
		wc.volumeSynced, wc.engineSynced, wc.replicaSynced,
		wc.settingSynced, wc.engineImageSynced, wc.backingImageSynced, wc.nodeSynced,
		wc.backupTargetSynced, wc.backupVolumeSynced, wc.backupSynced) {
		return
	}

	<-stopCh
}

func (wc *WebsocketController) Close() {
	wc.watcherLock.Lock()
	defer wc.watcherLock.Unlock()

	for _, w := range wc.watchers {
		w.Close()
	}
	wc.watchers = wc.watchers[:0]
}

func (wc *WebsocketController) notifyWatchersHandler(resource string) cache.ResourceEventHandler {
	return SimpleResourceEventHandler{
		ChangeFunc: wc.notifyWatchersFunc(resource),
	}
}

func (wc *WebsocketController) notifyWatchersFunc(resource string) func() {
	return func() {
		wc.watcherLock.Lock()
		defer wc.watcherLock.Unlock()
		for _, w := range wc.watchers {
			for _, r := range w.resources {
				if r == resource {
					select {
					case w.eventChan <- struct{}{}:
					default:
					}
					break
				}
			}
		}
	}
}
