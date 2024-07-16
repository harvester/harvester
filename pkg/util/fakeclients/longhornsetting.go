package fakeclients

import (
	"context"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	lhtype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/longhorn.io/v1beta2"
)

type LonghornSettingClient func(string) lhtype.SettingInterface

func (c LonghornSettingClient) Create(setting *lhv1beta2.Setting) (*lhv1beta2.Setting, error) {
	return c(setting.Namespace).Create(context.TODO(), setting, metav1.CreateOptions{})
}

func (c LonghornSettingClient) Update(setting *lhv1beta2.Setting) (*lhv1beta2.Setting, error) {
	return c(setting.Namespace).Update(context.TODO(), setting, metav1.UpdateOptions{})
}

func (c LonghornSettingClient) UpdateStatus(_ *lhv1beta2.Setting) (*lhv1beta2.Setting, error) {
	panic("implement me")
}

func (c LonghornSettingClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c LonghornSettingClient) Get(namespace, name string, options metav1.GetOptions) (*lhv1beta2.Setting, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c LonghornSettingClient) List(namespace string, opts metav1.ListOptions) (*lhv1beta2.SettingList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c LonghornSettingClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c LonghornSettingClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *lhv1beta2.Setting, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}
func (c LonghornSettingClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*lhv1beta2.Setting, *lhv1beta2.SettingList], error) {
	panic("implement me")
}

type LonghornSettingCache func(string) lhtype.SettingInterface

func (c LonghornSettingCache) Get(namespace, name string) (*lhv1beta2.Setting, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c LonghornSettingCache) List(_ string, _ labels.Selector) ([]*lhv1beta2.Setting, error) {
	panic("implement me")
}

func (c LonghornSettingCache) AddIndexer(_ string, _ generic.Indexer[*lhv1beta2.Setting]) {
	panic("implement me")
}

func (c LonghornSettingCache) GetByIndex(_, _ string) ([]*lhv1beta2.Setting, error) {
	panic("implement me")
}
