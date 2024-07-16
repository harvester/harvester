package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvestertype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
)

type HarvesterSettingClient func() harvestertype.SettingInterface

func (c HarvesterSettingClient) Create(s *v1beta1.Setting) (*v1beta1.Setting, error) {
	return c().Create(context.TODO(), s, metav1.CreateOptions{})
}

func (c HarvesterSettingClient) Update(s *v1beta1.Setting) (*v1beta1.Setting, error) {
	return c().Update(context.TODO(), s, metav1.UpdateOptions{})
}

func (c HarvesterSettingClient) UpdateStatus(_ *v1beta1.Setting) (*v1beta1.Setting, error) {
	panic("implement me")
}

func (c HarvesterSettingClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c HarvesterSettingClient) Get(name string, options metav1.GetOptions) (*v1beta1.Setting, error) {
	return c().Get(context.TODO(), name, options)
}

func (c HarvesterSettingClient) List(opts metav1.ListOptions) (*v1beta1.SettingList, error) {
	return c().List(context.TODO(), opts)
}

func (c HarvesterSettingClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c HarvesterSettingClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.Setting, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c HarvesterSettingClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*v1beta1.Setting, *v1beta1.SettingList], error) {
	panic("implement me")
}

type HarvesterSettingCache func() harvestertype.SettingInterface

func (c HarvesterSettingCache) Get(name string) (*v1beta1.Setting, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c HarvesterSettingCache) List(_ labels.Selector) ([]*v1beta1.Setting, error) {
	panic("implement me")
}

func (c HarvesterSettingCache) AddIndexer(_ string, _ generic.Indexer[*v1beta1.Setting]) {
	panic("implement me")
}

func (c HarvesterSettingCache) GetByIndex(_, _ string) ([]*v1beta1.Setting, error) {
	panic("implement me")
}
