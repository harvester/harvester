package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
)

type VersionCache func(string) harv1type.VersionInterface

func (c VersionCache) Get(namespace, name string) (*harvesterv1.Version, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c VersionCache) List(_ string, _ labels.Selector) ([]*harvesterv1.Version, error) {
	panic("implement me")
}
func (c VersionCache) AddIndexer(_ string, _ generic.Indexer[*harvesterv1.Version]) {
	panic("implement me")
}
func (c VersionCache) GetByIndex(_, _ string) ([]*harvesterv1.Version, error) {
	panic("implement me")
}

type VersionClient func(string) harv1type.VersionInterface

func (c VersionClient) Update(version *harvesterv1.Version) (*harvesterv1.Version, error) {
	return c(version.Namespace).Update(context.TODO(), version, metav1.UpdateOptions{})
}

func (c VersionClient) Get(namespace, name string, options metav1.GetOptions) (*harvesterv1.Version, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c VersionClient) Create(version *harvesterv1.Version) (*harvesterv1.Version, error) {
	return c(version.Namespace).Create(context.TODO(), version, metav1.CreateOptions{})
}

func (c VersionClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c VersionClient) List(namespace string, options metav1.ListOptions) (*harvesterv1.VersionList, error) {
	return c(namespace).List(context.TODO(), options)
}

func (c VersionClient) UpdateStatus(_ *harvesterv1.Version) (*harvesterv1.Version, error) {
	panic("implement me")
}

func (c VersionClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c VersionClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *harvesterv1.Version, err error) {
	panic("implement me")
}

func (c VersionClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*harvesterv1.Version, *harvesterv1.VersionList], error) {
	panic("implement me")
}
