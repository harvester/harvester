package fakeclients

import (
	"context"

	upgradeapiv1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	upgradev1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/upgrade.cattle.io/v1"
	upgradectlv1 "github.com/harvester/harvester/pkg/generated/controllers/upgrade.cattle.io/v1"
)

type PlanClient func(string) upgradev1.PlanInterface

func (c PlanClient) Update(plan *upgradeapiv1.Plan) (*upgradeapiv1.Plan, error) {
	return c(plan.Namespace).Update(context.TODO(), plan, metav1.UpdateOptions{})
}
func (c PlanClient) Get(namespace, name string, options metav1.GetOptions) (*upgradeapiv1.Plan, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c PlanClient) Create(plan *upgradeapiv1.Plan) (*upgradeapiv1.Plan, error) {
	return c(plan.Namespace).Create(context.TODO(), plan, metav1.CreateOptions{})
}
func (c PlanClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}
func (c PlanClient) List(namespace string, opts metav1.ListOptions) (*upgradeapiv1.PlanList, error) {
	panic("implement me")
}
func (c PlanClient) UpdateStatus(*upgradeapiv1.Plan) (*upgradeapiv1.Plan, error) {
	panic("implement me")
}
func (c PlanClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c PlanClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *upgradeapiv1.Plan, err error) {
	panic("implement me")
}

type PlanCache func(string) upgradev1.PlanInterface

func (c PlanCache) Get(namespace, name string) (*upgradeapiv1.Plan, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c PlanCache) List(namespace string, selector labels.Selector) ([]*upgradeapiv1.Plan, error) {
	panic("implement me")
}

func (c PlanCache) AddIndexer(indexName string, indexer upgradectlv1.PlanIndexer) {
	panic("implement me")
}

func (c PlanCache) GetByIndex(indexName, key string) ([]*upgradeapiv1.Plan, error) {
	panic("implement me")
}
