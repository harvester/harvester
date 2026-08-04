package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	managementcattlev3 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/management.cattle.io/v3"
	rancherv3api "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
)

type RancherSettingClient func() managementcattlev3.SettingInterface

func (c RancherSettingClient) Create(s *rancherv3api.Setting) (*rancherv3api.Setting, error) {
	return c().Create(context.TODO(), s, metav1.CreateOptions{})
}

func (c RancherSettingClient) Update(s *rancherv3api.Setting) (*rancherv3api.Setting, error) {
	return c().Update(context.TODO(), s, metav1.UpdateOptions{})
}

func (c RancherSettingClient) UpdateStatus(_ *rancherv3api.Setting) (*rancherv3api.Setting, error) {
	panic("implement me")
}

func (c RancherSettingClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c RancherSettingClient) Get(name string, options metav1.GetOptions) (*rancherv3api.Setting, error) {
	return c().Get(context.TODO(), name, options)
}

func (c RancherSettingClient) List(opts metav1.ListOptions) (*rancherv3api.SettingList, error) {
	return c().List(context.TODO(), opts)
}

func (c RancherSettingClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c RancherSettingClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *rancherv3api.Setting, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c RancherSettingClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*rancherv3api.Setting, *rancherv3api.SettingList], error) {
	panic("implement me")
}

type RancherSettingCache func() managementcattlev3.SettingInterface

func (c RancherSettingCache) Get(name string) (*rancherv3api.Setting, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c RancherSettingCache) List(_ labels.Selector) ([]*rancherv3api.Setting, error) {
	panic("implement me")
}

func (c RancherSettingCache) AddIndexer(_ string, _ generic.Indexer[*rancherv3api.Setting]) {
	panic("implement me")
}

func (c RancherSettingCache) GetByIndex(_, _ string) ([]*rancherv3api.Setting, error) {
	panic("implement me")
}
