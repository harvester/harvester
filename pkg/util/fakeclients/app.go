package fakeclients

import (
	"context"

	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	ctlcatalogv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	catalogv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/catalog.cattle.io/v1"
)

type AppCache func(string) catalogv1type.AppInterface

func (c AppCache) Get(namespace, name string) (*catalogv1.App, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c AppCache) List(namespace string, selector labels.Selector) ([]*catalogv1.App, error) {
	panic("implement me")
}
func (c AppCache) AddIndexer(indexName string, indexer ctlcatalogv1.AppIndexer) {
	panic("implement me")
}
func (c AppCache) GetByIndex(indexName, key string) ([]*catalogv1.App, error) {
	panic("implement me")
}
