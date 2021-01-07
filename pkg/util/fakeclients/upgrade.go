package fakeclients

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	harvesterv1alpha1 "github.com/rancher/harvester/pkg/generated/clientset/versioned/typed/harvester.cattle.io/v1alpha1"
	harvesterctlv1alpha1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
)

type UpgradeClient func(string) harvesterv1alpha1.UpgradeInterface

func (c UpgradeClient) Update(upgrade *v1alpha1.Upgrade) (*v1alpha1.Upgrade, error) {
	return c(upgrade.Namespace).Update(context.TODO(), upgrade, metav1.UpdateOptions{})
}
func (c UpgradeClient) Get(namespace, name string, options metav1.GetOptions) (*v1alpha1.Upgrade, error) {
	panic("implement me")
}
func (c UpgradeClient) Create(*v1alpha1.Upgrade) (*v1alpha1.Upgrade, error) {
	panic("implement me")
}
func (c UpgradeClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}
func (c UpgradeClient) List(namespace string, opts metav1.ListOptions) (*v1alpha1.UpgradeList, error) {
	panic("implement me")
}
func (c UpgradeClient) UpdateStatus(*v1alpha1.Upgrade) (*v1alpha1.Upgrade, error) {
	panic("implement me")
}
func (c UpgradeClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c UpgradeClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Upgrade, err error) {
	panic("implement me")
}

type UpgradeCache func(string) harvesterv1alpha1.UpgradeInterface

func (c UpgradeCache) Get(namespace, name string) (*v1alpha1.Upgrade, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c UpgradeCache) List(namespace string, selector labels.Selector) ([]*v1alpha1.Upgrade, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*v1alpha1.Upgrade, 0, len(list.Items))
	for _, upgrade := range list.Items {
		result = append(result, &upgrade)
	}
	return result, err
}
func (c UpgradeCache) AddIndexer(indexName string, indexer harvesterctlv1alpha1.UpgradeIndexer) {
	panic("implement me")
}
func (c UpgradeCache) GetByIndex(indexName, key string) ([]*v1alpha1.Upgrade, error) {
	panic("implement me")
}
