package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

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
