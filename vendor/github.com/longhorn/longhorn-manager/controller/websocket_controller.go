package controller

import (
	"sync"

	"github.com/sirupsen/logrus"

	"k8s.io/client-go/tools/cache"

	"github.com/longhorn/longhorn-manager/datastore"
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
	cacheSyncs []cache.InformerSynced

	watchers    []*Watcher
	watcherLock sync.Mutex
}

func NewWebsocketController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
) *WebsocketController {

	wc := &WebsocketController{
		baseController: newBaseController("longhorn-websocket", logger),
	}

	ds.VolumeInformer.AddEventHandler(wc.notifyWatchersHandler("volume"))
	wc.cacheSyncs = append(wc.cacheSyncs, ds.VolumeInformer.HasSynced)
	ds.EngineInformer.AddEventHandler(wc.notifyWatchersHandler("engine"))
	wc.cacheSyncs = append(wc.cacheSyncs, ds.EngineInformer.HasSynced)
	ds.ReplicaInformer.AddEventHandler(wc.notifyWatchersHandler("replica"))
	wc.cacheSyncs = append(wc.cacheSyncs, ds.ReplicaInformer.HasSynced)
	ds.SettingInformer.AddEventHandler(wc.notifyWatchersHandler("setting"))
	wc.cacheSyncs = append(wc.cacheSyncs, ds.SettingInformer.HasSynced)
	ds.EngineImageInformer.AddEventHandler(wc.notifyWatchersHandler("engineImage"))
	wc.cacheSyncs = append(wc.cacheSyncs, ds.EngineImageInformer.HasSynced)
	ds.BackingImageInformer.AddEventHandler(wc.notifyWatchersHandler("backingImage"))
	wc.cacheSyncs = append(wc.cacheSyncs, ds.BackingImageInformer.HasSynced)
	ds.NodeInformer.AddEventHandler(wc.notifyWatchersHandler("node"))
	wc.cacheSyncs = append(wc.cacheSyncs, ds.NodeInformer.HasSynced)
	ds.BackupTargetInformer.AddEventHandler(wc.notifyWatchersHandler("backupTarget"))
	wc.cacheSyncs = append(wc.cacheSyncs, ds.BackupTargetInformer.HasSynced)
	ds.BackupVolumeInformer.AddEventHandler(wc.notifyWatchersHandler("backupVolume"))
	wc.cacheSyncs = append(wc.cacheSyncs, ds.BackupVolumeInformer.HasSynced)
	ds.BackupInformer.AddEventHandler(wc.notifyWatchersHandler("backup"))
	wc.cacheSyncs = append(wc.cacheSyncs, ds.BackupInformer.HasSynced)
	ds.RecurringJobInformer.AddEventHandler(wc.notifyWatchersHandler("recurringJob"))
	wc.cacheSyncs = append(wc.cacheSyncs, ds.RecurringJobInformer.HasSynced)

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

	if !cache.WaitForNamedCacheSync("longhorn websocket", stopCh, wc.cacheSyncs...) {
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
