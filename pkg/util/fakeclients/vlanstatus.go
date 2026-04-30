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

type VlanStatusClient func() networktype.VlanStatusInterface

func (c VlanStatusClient) Create(s *v1beta1.VlanStatus) (*v1beta1.VlanStatus, error) {
	return c().Create(context.TODO(), s, metav1.CreateOptions{})
}

func (c VlanStatusClient) Update(s *v1beta1.VlanStatus) (*v1beta1.VlanStatus, error) {
	return c().Update(context.TODO(), s, metav1.UpdateOptions{})
}

func (c VlanStatusClient) UpdateStatus(_ *v1beta1.VlanStatus) (*v1beta1.VlanStatus, error) {
	panic("implement me")
}

func (c VlanStatusClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c VlanStatusClient) Get(name string, options metav1.GetOptions) (*v1beta1.VlanStatus, error) {
	return c().Get(context.TODO(), name, options)
}

func (c VlanStatusClient) List(opts metav1.ListOptions) (*v1beta1.VlanStatusList, error) {
	return c().List(context.TODO(), opts)
}

func (c VlanStatusClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c VlanStatusClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.VlanStatus, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type VlanStatusCache func() networktype.VlanStatusInterface

func (c VlanStatusCache) Get(name string) (*v1beta1.VlanStatus, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VlanStatusCache) List(selector labels.Selector) ([]*v1beta1.VlanStatus, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*v1beta1.VlanStatus, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c VlanStatusCache) AddIndexer(_ string, _ generic.Indexer[*v1beta1.VlanStatus]) {
	panic("implement me")
}

func (c VlanStatusCache) GetByIndex(_, _ string) ([]*v1beta1.VlanStatus, error) {
	panic("implement me")
}
