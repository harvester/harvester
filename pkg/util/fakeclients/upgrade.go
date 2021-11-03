package fakeclients

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

type UpgradeClient func(string) harv1type.UpgradeInterface

func (c UpgradeClient) Update(upgrade *harvesterv1.Upgrade) (*harvesterv1.Upgrade, error) {
	return c(upgrade.Namespace).Update(context.TODO(), upgrade, metav1.UpdateOptions{})
}
func (c UpgradeClient) Get(namespace, name string, options metav1.GetOptions) (*harvesterv1.Upgrade, error) {
	panic("implement me")
}
func (c UpgradeClient) Create(*harvesterv1.Upgrade) (*harvesterv1.Upgrade, error) {
	panic("implement me")
}
func (c UpgradeClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}
func (c UpgradeClient) List(namespace string, opts metav1.ListOptions) (*harvesterv1.UpgradeList, error) {
	panic("implement me")
}
func (c UpgradeClient) UpdateStatus(*harvesterv1.Upgrade) (*harvesterv1.Upgrade, error) {
	panic("implement me")
}
func (c UpgradeClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c UpgradeClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1.Upgrade, err error) {
	panic("implement me")
}

type UpgradeCache func(string) harv1type.UpgradeInterface

func (c UpgradeCache) Get(namespace, name string) (*harvesterv1.Upgrade, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c UpgradeCache) List(namespace string, selector labels.Selector) ([]*harvesterv1.Upgrade, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*harvesterv1.Upgrade, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}
func (c UpgradeCache) AddIndexer(indexName string, indexer ctlharvesterv1.UpgradeIndexer) {
	panic("implement me")
}
func (c UpgradeCache) GetByIndex(indexName, key string) ([]*harvesterv1.Upgrade, error) {
	panic("implement me")
}
