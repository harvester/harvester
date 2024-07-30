package fakeclients

import (
	"context"

	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	catalogv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/catalog.cattle.io/v1"
)

type AppCache func(string) catalogv1type.AppInterface

func (c AppCache) Get(namespace, name string) (*catalogv1.App, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c AppCache) List(_ string, _ labels.Selector) ([]*catalogv1.App, error) {
	panic("implement me")
}
func (c AppCache) AddIndexer(_ string, _ generic.Indexer[*catalogv1.App]) {
	panic("implement me")
}
func (c AppCache) GetByIndex(_, _ string) ([]*catalogv1.App, error) {
	panic("implement me")
}

type AppClient func(string) catalogv1type.AppInterface

func (c AppClient) Update(app *catalogv1.App) (*catalogv1.App, error) {
	return c(app.Namespace).Update(context.TODO(), app, metav1.UpdateOptions{})
}

func (c AppClient) Get(namespace, name string, options metav1.GetOptions) (*catalogv1.App, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c AppClient) Create(app *catalogv1.App) (*catalogv1.App, error) {
	return c(app.Namespace).Create(context.TODO(), app, metav1.CreateOptions{})
}

func (c AppClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c AppClient) List(_ string, _ metav1.ListOptions) (*catalogv1.AppList, error) {
	panic("implement me")
}

func (c AppClient) UpdateStatus(*catalogv1.App) (*catalogv1.App, error) {
	panic("implement me")
}

func (c AppClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c AppClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *catalogv1.App, err error) {
	panic("implement me")
}
