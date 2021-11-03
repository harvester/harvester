package fakeclients

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

type VersionCache func(string) harv1type.VersionInterface

func (c VersionCache) Get(namespace, name string) (*harvesterv1.Version, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c VersionCache) List(namespace string, selector labels.Selector) ([]*harvesterv1.Version, error) {
	panic("implement me")
}
func (c VersionCache) AddIndexer(indexName string, indexer ctlharvesterv1.VersionIndexer) {
	panic("implement me")
}
func (c VersionCache) GetByIndex(indexName, key string) ([]*harvesterv1.Version, error) {
	panic("implement me")
}
