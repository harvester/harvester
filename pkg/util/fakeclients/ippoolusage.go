package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
)

type IPPoolUsageCache func() harv1type.IPPoolUsageInterface

func (c IPPoolUsageCache) Get(name string) (*harvesterv1.IPPoolUsage, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c IPPoolUsageCache) List(selector labels.Selector) ([]*harvesterv1.IPPoolUsage, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*harvesterv1.IPPoolUsage, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, nil
}

func (c IPPoolUsageCache) AddIndexer(_ string, _ generic.Indexer[*harvesterv1.IPPoolUsage]) {
	panic("implement me")
}

func (c IPPoolUsageCache) GetByIndex(_, _ string) ([]*harvesterv1.IPPoolUsage, error) {
	panic("implement me")
}

type IPPoolUsageClient func() harv1type.IPPoolUsageInterface

func (c IPPoolUsageClient) Update(obj *harvesterv1.IPPoolUsage) (*harvesterv1.IPPoolUsage, error) {
	return c().Update(context.TODO(), obj, metav1.UpdateOptions{})
}

func (c IPPoolUsageClient) Get(name string, options metav1.GetOptions) (*harvesterv1.IPPoolUsage, error) {
	return c().Get(context.TODO(), name, options)
}

func (c IPPoolUsageClient) Create(obj *harvesterv1.IPPoolUsage) (*harvesterv1.IPPoolUsage, error) {
	return c().Create(context.TODO(), obj, metav1.CreateOptions{})
}

func (c IPPoolUsageClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c IPPoolUsageClient) List(options metav1.ListOptions) (*harvesterv1.IPPoolUsageList, error) {
	return c().List(context.TODO(), options)
}

func (c IPPoolUsageClient) UpdateStatus(obj *harvesterv1.IPPoolUsage) (*harvesterv1.IPPoolUsage, error) {
	return c().UpdateStatus(context.TODO(), obj, metav1.UpdateOptions{})
}

func (c IPPoolUsageClient) Watch(_ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c IPPoolUsageClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1.IPPoolUsage, err error) {
	panic("implement me")
}

func (c IPPoolUsageClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*harvesterv1.IPPoolUsage, *harvesterv1.IPPoolUsageList], error) {
	panic("implement me")
}
