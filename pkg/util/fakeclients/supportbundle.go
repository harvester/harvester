package fakeclients

import (
	"context"
	"time"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
)

type SupportBundleClient func(string) harv1type.SupportBundleInterface

func (c SupportBundleClient) Update(sb *harvesterv1.SupportBundle) (*harvesterv1.SupportBundle, error) {
	return c(sb.Namespace).Update(context.TODO(), sb, metav1.UpdateOptions{})
}

func (c SupportBundleClient) Get(namespace, name string, _ metav1.GetOptions) (*harvesterv1.SupportBundle, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c SupportBundleClient) Create(sb *harvesterv1.SupportBundle) (*harvesterv1.SupportBundle, error) {
	return c(sb.Namespace).Create(context.TODO(), sb, metav1.CreateOptions{})
}

func (c SupportBundleClient) Delete(namespace, name string, _ *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c SupportBundleClient) List(_ string, _ metav1.ListOptions) (*harvesterv1.SupportBundleList, error) {
	panic("implement me")
}

func (c SupportBundleClient) UpdateStatus(_ *harvesterv1.SupportBundle) (*harvesterv1.SupportBundle, error) {
	panic("implement me")
}

func (c SupportBundleClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c SupportBundleClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *harvesterv1.SupportBundle, err error) {
	panic("implement me")
}

func (c SupportBundleClient) Informer() cache.SharedIndexInformer {
	panic("implement me")
}

func (c SupportBundleClient) GroupVersionKind() schema.GroupVersionKind {
	panic("implement me")
}

func (c SupportBundleClient) AddGenericHandler(_ context.Context, _ string, _ generic.Handler) {
	panic("implement me")
}

func (c SupportBundleClient) AddGenericRemoveHandler(_ context.Context, _ string, _ generic.Handler) {
	panic("implement me")
}

func (c SupportBundleClient) Updater() generic.Updater {
	panic("implement me")
}

func (c SupportBundleClient) OnChange(_ context.Context, _ string, _ generic.ObjectHandler[*harvesterv1.SupportBundle]) {
	panic("implement me")
}

func (c SupportBundleClient) OnRemove(_ context.Context, _ string, _ generic.ObjectHandler[*harvesterv1.SupportBundle]) {
	panic("implement me")
}

func (c SupportBundleClient) Cache() generic.CacheInterface[*harvesterv1.SupportBundle] {
	panic("implement me")
}

func (c SupportBundleClient) Enqueue(_, _ string) {
	panic("implement me")
}

func (c SupportBundleClient) EnqueueAfter(_, _ string, _ time.Duration) {
	// do nothing
}

func (c SupportBundleClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*harvesterv1.SupportBundle, *harvesterv1.SupportBundleList], error) {
	panic("implement me")
}

type SupportBundleCache func(string) harv1type.SupportBundleInterface

func (c SupportBundleCache) Get(namespace, name string) (*harvesterv1.SupportBundle, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c SupportBundleCache) List(_ string, _ labels.Selector) ([]*harvesterv1.SupportBundle, error) {
	panic("implement me")
}

func (c SupportBundleCache) AddIndexer(_ string, _ generic.Indexer[*harvesterv1.SupportBundle]) {
	panic("implement me")
}

func (c SupportBundleCache) GetByIndex(_, _ string) ([]*harvesterv1.SupportBundle, error) {
	panic("implement me")
}
