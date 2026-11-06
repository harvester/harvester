package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	"github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	networktype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/network.harvesterhci.io/v1beta1"
)

type HostNetworkConfigCache func() networktype.HostNetworkConfigInterface

func (c HostNetworkConfigCache) Get(name string) (*v1beta1.HostNetworkConfig, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c HostNetworkConfigCache) List(selector labels.Selector) ([]*v1beta1.HostNetworkConfig, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*v1beta1.HostNetworkConfig, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c HostNetworkConfigCache) AddIndexer(_ string, _ generic.Indexer[*v1beta1.HostNetworkConfig]) {
	panic("implement me")
}

func (c HostNetworkConfigCache) GetByIndex(_, _ string) ([]*v1beta1.HostNetworkConfig, error) {
	panic("implement me")
}

type HostNetworkConfigClient func() networktype.HostNetworkConfigInterface

func (c HostNetworkConfigClient) Create(s *v1beta1.HostNetworkConfig) (*v1beta1.HostNetworkConfig, error) {
	return c().Create(context.TODO(), s, metav1.CreateOptions{})
}

func (c HostNetworkConfigClient) Update(s *v1beta1.HostNetworkConfig) (*v1beta1.HostNetworkConfig, error) {
	return c().Update(context.TODO(), s, metav1.UpdateOptions{})
}

func (c HostNetworkConfigClient) UpdateStatus(s *v1beta1.HostNetworkConfig) (*v1beta1.HostNetworkConfig, error) {
	return c().UpdateStatus(context.TODO(), s, metav1.UpdateOptions{})
}

func (c HostNetworkConfigClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c HostNetworkConfigClient) Get(name string, options metav1.GetOptions) (*v1beta1.HostNetworkConfig, error) {
	return c().Get(context.TODO(), name, options)
}

func (c HostNetworkConfigClient) List(opts metav1.ListOptions) (*v1beta1.HostNetworkConfigList, error) {
	return c().List(context.TODO(), opts)
}

func (c HostNetworkConfigClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c HostNetworkConfigClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (*v1beta1.HostNetworkConfig, error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c HostNetworkConfigClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*v1beta1.HostNetworkConfig, *v1beta1.HostNetworkConfigList], error) {
	panic("implement me")
}
