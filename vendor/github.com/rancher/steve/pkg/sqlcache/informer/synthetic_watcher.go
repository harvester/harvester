package informer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

type SyntheticWatcher struct {
	resultChan   chan watch.Event
	stopChan     chan struct{}
	doneChan     chan struct{}
	stopChanLock sync.Mutex
	context      context.Context
	cancelFunc   context.CancelFunc
	gvk          schema.GroupVersionKind
}

func newSyntheticWatcher(context context.Context, cancel context.CancelFunc, gvk schema.GroupVersionKind) *SyntheticWatcher {
	return &SyntheticWatcher{
		stopChan:   make(chan struct{}),
		doneChan:   make(chan struct{}),
		resultChan: make(chan watch.Event, 0),
		context:    context,
		cancelFunc: cancel,
		gvk:        gvk,
	}
}

func (rw *SyntheticWatcher) watch(client dynamic.ResourceInterface, options metav1.ListOptions, interval time.Duration) (*SyntheticWatcher, error) {
	go rw.receive(client, options, interval)
	return rw, nil
}

type objectHolder struct {
	version            string
	unstructuredObject *unstructured.Unstructured
}

// receive periodically calls client.List(), and converts the returned items into Watch Events
func (rw *SyntheticWatcher) receive(client dynamic.ResourceInterface, options metav1.ListOptions, interval time.Duration) {
	go func() {
		defer close(rw.doneChan)
		defer close(rw.resultChan)
		defer rw.cancelFunc()
		previousState := make(map[string]objectHolder)
		ticker := time.NewTicker(interval)

		initialSyncSent := false

		for {
			select {
			case <-ticker.C:
				// Clear Watch-specific fields set by the WatchList flow since
				// the synthetic watcher is only used for non watchable resources.
				options.SendInitialEvents = nil
				options.ResourceVersionMatch = ""
				options.AllowWatchBookmarks = false
				list, err := client.List(rw.context, options)
				if err != nil {
					logrus.Errorf("synthetic watcher: client.List => error: %s", err)
					continue
				}
				newObjects := make(map[string]objectHolder)
				for _, uItem := range list.Items {
					namespace := uItem.GetNamespace()
					name := uItem.GetName()
					key := name
					if namespace != "" {
						key = fmt.Sprintf("%s/%s", namespace, name)
					}
					version := uItem.GetResourceVersion()
					newObjects[key] = objectHolder{version: version, unstructuredObject: &uItem}
				}
				// Now determine whether items were added, deleted, or modified
				currentState := make(map[string]objectHolder)
				for key, newObject := range newObjects {
					currentState[key] = newObject
					if oldItem, ok := previousState[key]; !ok {
						w, err := createWatchEvent(watch.Added, newObject.unstructuredObject)
						if err != nil {
							logrus.Errorf("can't convert unstructured obj into runtime: %s", err)
							continue
						}
						rw.resultChan <- w
					} else {
						delete(previousState, key)
						if isUpdatedObject(oldItem, newObject) {
							w, err := createWatchEvent(watch.Modified, oldItem.unstructuredObject)
							if err != nil {
								logrus.Errorf("can't convert unstructured obj into runtime: %s", err)
								continue
							}
							rw.resultChan <- w
						}
					}
				}
				// And anything left  in the previousState didn't show up in currentState and can be deleted.
				for _, item := range previousState {
					w, err := createWatchEvent(watch.Deleted, item.unstructuredObject)
					if err != nil {
						continue
					}
					rw.resultChan <- w
				}
				previousState = currentState

				if !initialSyncSent {
					sendInitialSyncBookmark(list.GetResourceVersion(), rw.gvk, rw.resultChan)
					initialSyncSent = true
					logrus.Debugf("synthetic watcher: sent initial events end bookmark for %v", rw.gvk)
				}

			case <-rw.stopChan:
				rw.cancelFunc()
				return

			case <-rw.context.Done():
				return
			}
		}
	}()
}

// sendInitialSyncBookmark constructs and sends a synthetic watch.Bookmark event.
// This satisfies the client-go Reflector's requirement for the KEP-3157 WatchList protocol,
// signaling that the initial stream of events has finished and stopping the timeout ticker.
func sendInitialSyncBookmark(resourceVersion string, gvk schema.GroupVersionKind, resultChan chan watch.Event) {
	bookmarkObj := &unstructured.Unstructured{}
	bookmarkObj.SetAnnotations(map[string]string{
		metav1.InitialEventsAnnotationKey: "true",
	})
	bookmarkObj.SetResourceVersion(resourceVersion)
	bookmarkObj.SetGroupVersionKind(gvk)

	resultChan <- watch.Event{
		Type:   watch.Bookmark,
		Object: bookmarkObj,
	}
}

func createWatchEvent(event watch.EventType, u *unstructured.Unstructured) (watch.Event, error) {
	return watch.Event{Type: event, Object: u}, nil
}

// isUpdatedObject compares two objectHolder instances to determine if the underlying object has changed. It checks the resource version, and for NodeMetrics, it also checks the timestamp field
func isUpdatedObject(oldItem, newItem objectHolder) bool {
	return oldItem.version != newItem.version ||
		(newItem.unstructuredObject.Object["kind"] == "NodeMetrics" && newItem.unstructuredObject.Object["timestamp"] != oldItem.unstructuredObject.Object["timestamp"])
}

// ResultChan implements [k8s.io/apimachinery/pkg/watch].Interface.
func (rw *SyntheticWatcher) ResultChan() <-chan watch.Event {
	return rw.resultChan
}

// Stop implements [k8s.io/apimachinery/pkg/watch].Interface.
func (rw *SyntheticWatcher) Stop() {
	rw.stopChanLock.Lock()
	defer rw.stopChanLock.Unlock()

	// Prevent closing an already closed channel to prevent a panic
	select {
	case <-rw.stopChan:
	default:
		close(rw.stopChan)
	}
}

func (rw *SyntheticWatcher) Done() <-chan struct{} {
	return rw.doneChan
}
