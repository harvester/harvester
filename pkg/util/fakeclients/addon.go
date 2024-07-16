package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
)

type AddonCache func(string) harv1type.AddonInterface

func (c AddonCache) Get(namespace, name string) (*harvesterv1.Addon, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c AddonCache) List(namespace string, selector labels.Selector) ([]*harvesterv1.Addon, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*harvesterv1.Addon, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}
func (c AddonCache) AddIndexer(_ string, _ generic.Indexer[*harvesterv1.Addon]) {
	panic("implement me")
}
func (c AddonCache) GetByIndex(_, _ string) ([]*harvesterv1.Addon, error) {
	panic("implement me")
}
