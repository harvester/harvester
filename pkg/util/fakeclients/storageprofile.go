package fakeclients

import (
	"context"

	cdiv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/cdi.kubevirt.io/v1beta1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

type StorageProfileClient func() cdiv1type.StorageProfileInterface

func (c StorageProfileClient) Create(s *cdiv1.StorageProfile) (*cdiv1.StorageProfile, error) {
	return c().Create(context.TODO(), s, metav1.CreateOptions{})
}

func (c StorageProfileClient) Update(s *cdiv1.StorageProfile) (*cdiv1.StorageProfile, error) {
	return c().Update(context.TODO(), s, metav1.UpdateOptions{})
}

func (c StorageProfileClient) UpdateStatus(_ *cdiv1.StorageProfile) (*cdiv1.StorageProfile, error) {
	panic("implement me")
}

func (c StorageProfileClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c StorageProfileClient) Get(name string, options metav1.GetOptions) (*cdiv1.StorageProfile, error) {
	return c().Get(context.TODO(), name, options)
}

func (c StorageProfileClient) List(opts metav1.ListOptions) (*cdiv1.StorageProfileList, error) {
	return c().List(context.TODO(), opts)
}

func (c StorageProfileClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c StorageProfileClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *cdiv1.StorageProfile, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type StorageProfileCache func() cdiv1type.StorageProfileInterface

func (c StorageProfileCache) Get(name string) (*cdiv1.StorageProfile, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c StorageProfileCache) List(selector labels.Selector) ([]*cdiv1.StorageProfile, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*cdiv1.StorageProfile, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c StorageProfileClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*cdiv1.StorageProfile, *cdiv1.StorageProfileList], error) {
	panic("implement me")
}

func (c StorageProfileCache) AddIndexer(_ string, _ generic.Indexer[*cdiv1.StorageProfile]) {
	panic("implement me")
}

func (c StorageProfileCache) GetByIndex(indexName, key string) ([]*cdiv1.StorageProfile, error) {
	panic("implement me")
}
