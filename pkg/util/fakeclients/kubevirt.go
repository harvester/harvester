package fakeclients

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	// kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/rancher/wrangler/v3/pkg/generic"

	kubevirtv1api "kubevirt.io/api/core/v1"

	kubevirtv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
)

// type KubevirtClient func(string) kubevirtctrl.KubeVirtClient

// func (c LoggingClient) Create(logging *loggingv1.Logging) (*loggingv1.Logging, error) {
// 	return c().Create(context.TODO(), logging, metav1.CreateOptions{})
// }

type KubeVirtCache func(string) kubevirtv1.KubeVirtInterface

func (c KubeVirtCache) Get(namespace, name string) (*kubevirtv1api.KubeVirt, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c KubeVirtCache) List(_ string, _ labels.Selector) ([]*kubevirtv1api.KubeVirt, error) {
	panic("implement me")
}
func (c KubeVirtCache) AddIndexer(_ string, _ generic.Indexer[*kubevirtv1api.KubeVirt]) {
	panic("implement me")
}
func (c KubeVirtCache) GetByIndex(_, _ string) ([]*kubevirtv1api.KubeVirt, error) {
	panic("implement me")
}
