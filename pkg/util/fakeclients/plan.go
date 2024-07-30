package fakeclients

import (
	"context"

	upgradeapiv1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	upgradev1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/upgrade.cattle.io/v1"
)

type PlanClient func(string) upgradev1.PlanInterface

func (c PlanClient) Update(plan *upgradeapiv1.Plan) (*upgradeapiv1.Plan, error) {
	return c(plan.Namespace).Update(context.TODO(), plan, metav1.UpdateOptions{})
}
func (c PlanClient) Get(namespace, name string, options metav1.GetOptions) (*upgradeapiv1.Plan, error) {
	return c(namespace).Get(context.TODO(), name, options)
}
func (c PlanClient) Create(plan *upgradeapiv1.Plan) (*upgradeapiv1.Plan, error) {
	return c(plan.Namespace).Create(context.TODO(), plan, metav1.CreateOptions{})
}
func (c PlanClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}
func (c PlanClient) List(_ string, _ metav1.ListOptions) (*upgradeapiv1.PlanList, error) {
	panic("implement me")
}
func (c PlanClient) UpdateStatus(*upgradeapiv1.Plan) (*upgradeapiv1.Plan, error) {
	panic("implement me")
}
func (c PlanClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c PlanClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *upgradeapiv1.Plan, err error) {
	panic("implement me")
}

func (c PlanClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*upgradeapiv1.Plan, *upgradeapiv1.PlanList], error) {
	panic("implement me")
}

type PlanCache func(string) upgradev1.PlanInterface

func (c PlanCache) Get(namespace, name string) (*upgradeapiv1.Plan, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c PlanCache) List(_ string, _ labels.Selector) ([]*upgradeapiv1.Plan, error) {
	panic("implement me")
}

func (c PlanCache) AddIndexer(_ string, _ generic.Indexer[*upgradeapiv1.Plan]) {
	panic("implement me")
}

func (c PlanCache) GetByIndex(_, _ string) ([]*upgradeapiv1.Plan, error) {
	panic("implement me")
}
