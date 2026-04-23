package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	networktype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/network.harvesterhci.io/v1beta1"
)

type VlanConfigClient func() networktype.VlanConfigInterface

func (c VlanConfigClient) Create(s *v1beta1.VlanConfig) (*v1beta1.VlanConfig, error) {
	return c().Create(context.TODO(), s, metav1.CreateOptions{})
}

func (c VlanConfigClient) Update(s *v1beta1.VlanConfig) (*v1beta1.VlanConfig, error) {
	return c().Update(context.TODO(), s, metav1.UpdateOptions{})
}

func (c VlanConfigClient) UpdateStatus(_ *v1beta1.VlanConfig) (*v1beta1.VlanConfig, error) {
	panic("implement me")
}

func (c VlanConfigClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c VlanConfigClient) Get(name string, options metav1.GetOptions) (*v1beta1.VlanConfig, error) {
	return c().Get(context.TODO(), name, options)
}

func (c VlanConfigClient) List(opts metav1.ListOptions) (*v1beta1.VlanConfigList, error) {
	return c().List(context.TODO(), opts)
}

func (c VlanConfigClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c VlanConfigClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.VlanConfig, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type VlanConfigCache func() networktype.VlanConfigInterface

func (c VlanConfigCache) Get(name string) (*v1beta1.VlanConfig, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VlanConfigCache) List(selector labels.Selector) ([]*v1beta1.VlanConfig, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*v1beta1.VlanConfig, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c VlanConfigCache) AddIndexer(_ string, _ generic.Indexer[*v1beta1.VlanConfig]) {
	panic("implement me")
}

func (c VlanConfigCache) GetByIndex(_, _ string) ([]*v1beta1.VlanConfig, error) {
	panic("implement me")
}
