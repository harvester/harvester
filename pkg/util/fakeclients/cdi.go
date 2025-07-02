package fakeclients

import (
	"context"

	cdiv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/cdi.kubevirt.io/v1beta1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

type CDICache func() cdiv1type.CDIInterface

func (c CDICache) Get(name string) (*cdiv1.CDI, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c CDICache) List(selector labels.Selector) ([]*cdiv1.CDI, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*cdiv1.CDI, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c CDICache) AddIndexer(_ string, _ generic.Indexer[*cdiv1.CDI]) {
	panic("implement me")
}

func (c CDICache) GetByIndex(_, _ string) ([]*cdiv1.CDI, error) {
	panic("implement me")
}

type CDIClient func() cdiv1type.CDIInterface

func (c CDIClient) Update(obj *cdiv1.CDI) (*cdiv1.CDI, error) {
	return c().Update(context.TODO(), obj, metav1.UpdateOptions{})
}

func (c CDIClient) Get(name string, options metav1.GetOptions) (*cdiv1.CDI, error) {
	return c().Get(context.TODO(), name, options)
}

func (c CDIClient) Create(obj *cdiv1.CDI) (*cdiv1.CDI, error) {
	return c().Create(context.TODO(), obj, metav1.CreateOptions{})
}

func (c CDIClient) Delete(_ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c CDIClient) List(metav1.ListOptions) (*cdiv1.CDIList, error) {
	panic("implement me")
}

func (c CDIClient) UpdateStatus(*cdiv1.CDI) (*cdiv1.CDI, error) {
	panic("implement me")
}

func (c CDIClient) Watch(metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c CDIClient) Patch(_ string, _ types.PatchType, _ []byte, _ ...string) (result *cdiv1.CDI, err error) {
	panic("implement me")
}

func (c CDIClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*cdiv1.CDI, *cdiv1.CDIList], error) {
	panic("implement me")
}
