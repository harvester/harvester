package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

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
