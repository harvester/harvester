/*
package sql provides an Informer and Indexer that uses SQLite as a store, instead of an in-memory store like a map.
*/

package informer

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/rancher/steve/pkg/sqlcache/db"
	"github.com/rancher/steve/pkg/sqlcache/partition"
	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
	sqlStore "github.com/rancher/steve/pkg/sqlcache/store"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

var defaultRefreshTime = 5 * time.Second

// Informer is a SQLite-backed cache.SharedIndexInformer that can execute queries on listprocessor structs
type Informer struct {
	cache.SharedIndexInformer
	ByOptionsLister
}

type WatchOptions struct {
	ResourceVersion string
	Filter          WatchFilter
}

type WatchFilter struct {
	ID        string
	Selector  labels.Selector
	Namespace string
}

type ByOptionsLister interface {
	ListByOptions(ctx context.Context, lo *sqltypes.ListOptions, partitions []partition.Partition, namespace string) (*unstructured.UnstructuredList, int, string, error)
	Watch(ctx context.Context, options WatchOptions, eventsCh chan<- watch.Event) error
}

// this is set to a var so that it can be overridden by test code for mocking purposes
var newInformer = cache.NewSharedIndexInformer

// NewInformer returns a new SQLite-backed Informer for the type specified by schema in unstructured.Unstructured form
// using the specified client
func NewInformer(ctx context.Context, client dynamic.ResourceInterface, fields [][]string, transform cache.TransformFunc, gvk schema.GroupVersionKind, db db.Client, shouldEncrypt bool, namespaced bool, watchable bool, maxEventsCount int) (*Informer, error) {
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		return client.Watch(ctx, options)
	}
	if !watchable {
		watchFunc = func(options metav1.ListOptions) (watch.Interface, error) {
			ctx, cancel := context.WithCancel(ctx)
			return newSyntheticWatcher(ctx, cancel).watch(client, options, defaultRefreshTime)
		}
	}
	listWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			a, err := client.List(ctx, options)
			if err != nil {
				return nil, err
			}
			// We want the list to be consistent when there are going to be relists
			sort.SliceStable(a.Items, func(i int, j int) bool {
				var err error
				rvI, err1 := strconv.Atoi(a.Items[i].GetResourceVersion())
				err = errors.Join(err, err1)
				rvJ, err2 := strconv.Atoi(a.Items[j].GetResourceVersion())
				err = errors.Join(err, err2)
				if err != nil {
					logrus.Debug("ResourceVersion not a number, falling back to string comparison")
					return a.Items[i].GetResourceVersion() < a.Items[j].GetResourceVersion()
				}
				return rvI < rvJ
			})
			return a, err
		},
		WatchFunc: watchFunc,
	}

	example := &unstructured.Unstructured{}
	example.SetGroupVersionKind(gvk)

	// TL;DR: this disables the Informer periodic resync - but this is inconsequential
	//
	// Long version: Informers use a Reflector to pull data from a ListWatcher and push it into a DeltaFIFO.
	// Concurrently, they pop data off the DeltaFIFO to fire registered handlers, and also to keep an updated
	// copy of the known state of all objects (in an Indexer).
	// The resync period option here is passed from Informer to Reflector to periodically (re)-push all known
	// objects to the DeltaFIFO. That causes the periodic (re-)firing all registered handlers.
	// In this case we are not registering any handlers to this particular informer, so re-syncing is a no-op.
	// We therefore just disable it right away.
	resyncPeriod := time.Duration(0)

	sii := newInformer(listWatcher, example, resyncPeriod, cache.Indexers{})
	if transform != nil {
		if err := sii.SetTransform(transform); err != nil {
			return nil, err
		}
	}

	name := informerNameFromGVK(gvk)

	s, err := sqlStore.NewStore(ctx, example, cache.DeletionHandlingMetaNamespaceKeyFunc, db, shouldEncrypt, name)
	if err != nil {
		return nil, err
	}

	opts := ListOptionIndexerOptions{
		Fields:             fields,
		IsNamespaced:       namespaced,
		MaximumEventsCount: maxEventsCount,
	}
	loi, err := NewListOptionIndexer(ctx, s, opts)
	if err != nil {
		return nil, err
	}

	// HACK: replace the default informer's indexer with the SQL based one
	UnsafeSet(sii, "indexer", loi)

	return &Informer{
		SharedIndexInformer: sii,
		ByOptionsLister:     loi,
	}, nil
}

// ListByOptions returns objects according to the specified list options and partitions.
// Specifically:
//   - an unstructured list of resources belonging to any of the specified partitions
//   - the total number of resources (returned list might be a subset depending on pagination options in lo)
//   - a continue token, if there are more pages after the returned one
//   - an error instead of all of the above if anything went wrong
func (i *Informer) ListByOptions(ctx context.Context, lo *sqltypes.ListOptions, partitions []partition.Partition, namespace string) (*unstructured.UnstructuredList, int, string, error) {
	return i.ByOptionsLister.ListByOptions(ctx, lo, partitions, namespace)
}

// SetSyntheticWatchableInterval - call this function to override the default interval time of 5 seconds
func SetSyntheticWatchableInterval(interval time.Duration) {
	defaultRefreshTime = interval
}

func informerNameFromGVK(gvk schema.GroupVersionKind) string {
	return gvk.Group + "_" + gvk.Version + "_" + gvk.Kind
}
