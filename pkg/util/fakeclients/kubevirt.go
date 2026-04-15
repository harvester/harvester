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
	kubevirtv1api "kubevirt.io/api/core/v1"

	kubevirtv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
)

type KubeVirtClient func(string) kubevirtv1.KubeVirtInterface

func (c KubeVirtClient) Update(kubevirt *kubevirtv1api.KubeVirt) (*kubevirtv1api.KubeVirt, error) {
	return c(kubevirt.Namespace).Update(context.TODO(), kubevirt, metav1.UpdateOptions{})
}

func (c KubeVirtClient) Get(namespace, name string, options metav1.GetOptions) (*kubevirtv1api.KubeVirt, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c KubeVirtClient) Create(kubevirt *kubevirtv1api.KubeVirt) (*kubevirtv1api.KubeVirt, error) {
	return c(kubevirt.Namespace).Create(context.TODO(), kubevirt, metav1.CreateOptions{})
}

func (c KubeVirtClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c KubeVirtClient) List(_ string, _ metav1.ListOptions) (*kubevirtv1api.KubeVirtList, error) {
	panic("implement me")
}

func (c KubeVirtClient) UpdateStatus(_ *kubevirtv1api.KubeVirt) (*kubevirtv1api.KubeVirt, error) {
	panic("implement me")
}

func (c KubeVirtClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c KubeVirtClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *kubevirtv1api.KubeVirt, err error) {
	panic("implement me")
}

func (c KubeVirtClient) Informer() cache.SharedIndexInformer {
	panic("implement me")
}

func (c KubeVirtClient) GroupVersionKind() schema.GroupVersionKind {
	panic("implement me")
}

func (c KubeVirtClient) AddGenericHandler(_ context.Context, _ string, _ generic.Handler) {
	panic("implement me")
}

func (c KubeVirtClient) AddGenericRemoveHandler(_ context.Context, _ string, _ generic.Handler) {
	panic("implement me")
}

func (c KubeVirtClient) Updater() generic.Updater {
	panic("implement me")
}

func (c KubeVirtClient) OnChange(_ context.Context, _ string, _ generic.ObjectHandler[*kubevirtv1api.KubeVirt]) {
	panic("implement me")
}

func (c KubeVirtClient) OnRemove(_ context.Context, _ string, _ generic.ObjectHandler[*kubevirtv1api.KubeVirt]) {
	panic("implement me")
}

func (c KubeVirtClient) Enqueue(_, _ string) {
	panic("implement me")
}

func (c KubeVirtClient) EnqueueAfter(_, _ string, _ time.Duration) {
	panic("implement me")
}

func (c KubeVirtClient) Cache() generic.CacheInterface[*kubevirtv1api.KubeVirt] {
	panic("implement me")
}

func (c KubeVirtClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*kubevirtv1api.KubeVirt, *kubevirtv1api.KubeVirtList], error) {
	panic("implement me")
}

type KubeVirtCache func(string) kubevirtv1.KubeVirtInterface

func (c KubeVirtCache) Get(namespace, name string) (*kubevirtv1api.KubeVirt, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c KubeVirtCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1api.KubeVirt, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*kubevirtv1api.KubeVirt, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c KubeVirtCache) AddIndexer(_ string, _ generic.Indexer[*kubevirtv1api.KubeVirt]) {
	panic("implement me")
}

func (c KubeVirtCache) GetByIndex(_, _ string) ([]*kubevirtv1api.KubeVirt, error) {
	panic("implement me")
}
