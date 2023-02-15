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

type UpgradeLogClient func(string) harv1type.UpgradeLogInterface

func (c UpgradeLogClient) Create(upgradeLog *harvesterv1.UpgradeLog) (*harvesterv1.UpgradeLog, error) {
	return c(upgradeLog.Namespace).Create(context.TODO(), upgradeLog, metav1.CreateOptions{})
}
func (c UpgradeLogClient) Update(upgradeLog *harvesterv1.UpgradeLog) (*harvesterv1.UpgradeLog, error) {
	return c(upgradeLog.Namespace).Update(context.TODO(), upgradeLog, metav1.UpdateOptions{})
}
func (c UpgradeLogClient) UpdateStatus(upgradeLog *harvesterv1.UpgradeLog) (*harvesterv1.UpgradeLog, error) {
	panic("implement me")
}
func (c UpgradeLogClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}
func (c UpgradeLogClient) Get(namespace, name string, options metav1.GetOptions) (*harvesterv1.UpgradeLog, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c UpgradeLogClient) List(namespace string, opts metav1.ListOptions) (*harvesterv1.UpgradeLogList, error) {
	panic("implement me")
}
func (c UpgradeLogClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c UpgradeLogClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1.UpgradeLog, err error) {
	panic("implement me")
}

type UpgradeLogCache func(string) harv1type.UpgradeLogInterface

func (c UpgradeLogCache) Get(namespace, name string) (*harvesterv1.UpgradeLog, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c UpgradeLogCache) List(namespace string, selector labels.Selector) ([]*harvesterv1.UpgradeLog, error) {
	panic("implement me")
}
func (c UpgradeLogCache) AddIndexer(indexName string, indexer ctlharvesterv1.UpgradeLogIndexer) {
	panic("implement me")
}
func (c UpgradeLogCache) GetByIndex(indexName, key string) ([]*harvesterv1.UpgradeLog, error) {
	panic("implement me")
}
