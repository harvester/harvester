package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"

	networkv1beta1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	networkv1beta1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/network.harvesterhci.io/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type ClusterNetworkCache func() networkv1beta1type.ClusterNetworkInterface

func (c ClusterNetworkCache) Get(name string) (*networkv1beta1.ClusterNetwork, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c ClusterNetworkCache) List(selector labels.Selector) ([]*networkv1beta1.ClusterNetwork, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*networkv1beta1.ClusterNetwork, 0, len(list.Items))
	for _, item := range list.Items {
		obj := item
		result = append(result, &obj)
	}
	return result, err
}

func (c ClusterNetworkCache) AddIndexer(_ string, _ generic.Indexer[*networkv1beta1.ClusterNetwork]) {
	panic("implement me")
}

func (c ClusterNetworkCache) GetByIndex(_, _ string) ([]*networkv1beta1.ClusterNetwork, error) {
	panic("implement me")
}
