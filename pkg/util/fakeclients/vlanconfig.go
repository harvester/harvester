package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"

	networkv1beta1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	networkv1beta1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/network.harvesterhci.io/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type VlanConfigCache func() networkv1beta1type.VlanConfigInterface

func (c VlanConfigCache) Get(name string) (*networkv1beta1.VlanConfig, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VlanConfigCache) List(selector labels.Selector) ([]*networkv1beta1.VlanConfig, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*networkv1beta1.VlanConfig, 0, len(list.Items))
	for _, item := range list.Items {
		obj := item
		result = append(result, &obj)
	}
	return result, err
}
func (c VlanConfigCache) AddIndexer(_ string, _ generic.Indexer[*networkv1beta1.VlanConfig]) {
	panic("implement me")
}

// GetByIndex implements the VlanConfigCache interface
func (c VlanConfigCache) GetByIndex(_, _ string) ([]*networkv1beta1.VlanConfig, error) {
	panic("implement me")
}
