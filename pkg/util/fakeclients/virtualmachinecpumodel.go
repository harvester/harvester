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
	typeharv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
)

type VirtualMachineCPUModelClient func() typeharv1.VirtualMachineCPUModelInterface

func (c VirtualMachineCPUModelClient) Create(obj *harvesterv1.VirtualMachineCPUModel) (*harvesterv1.VirtualMachineCPUModel, error) {
	return c().Create(context.TODO(), obj, metav1.CreateOptions{})
}

func (c VirtualMachineCPUModelClient) Update(obj *harvesterv1.VirtualMachineCPUModel) (*harvesterv1.VirtualMachineCPUModel, error) {
	return c().Update(context.TODO(), obj, metav1.UpdateOptions{})
}

func (c VirtualMachineCPUModelClient) UpdateStatus(obj *harvesterv1.VirtualMachineCPUModel) (*harvesterv1.VirtualMachineCPUModel, error) {
	return c().UpdateStatus(context.TODO(), obj, metav1.UpdateOptions{})
}

func (c VirtualMachineCPUModelClient) Delete(name string, opts *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *opts)
}

func (c VirtualMachineCPUModelClient) Get(name string, opts metav1.GetOptions) (*harvesterv1.VirtualMachineCPUModel, error) {
	return c().Get(context.TODO(), name, opts)
}

func (c VirtualMachineCPUModelClient) List(opts metav1.ListOptions) (*harvesterv1.VirtualMachineCPUModelList, error) {
	return c().List(context.TODO(), opts)
}

func (c VirtualMachineCPUModelClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c VirtualMachineCPUModelClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1.VirtualMachineCPUModel, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c VirtualMachineCPUModelClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*harvesterv1.VirtualMachineCPUModel, *harvesterv1.VirtualMachineCPUModelList], error) {
	panic("implement me")
}

type VirtualMachineCPUModelController func() typeharv1.VirtualMachineCPUModelInterface

func (c VirtualMachineCPUModelController) Enqueue(name string) {
	// No-op for testing
}

type VirtualMachineCPUModelCache func() typeharv1.VirtualMachineCPUModelInterface

func (c VirtualMachineCPUModelCache) Get(name string) (*harvesterv1.VirtualMachineCPUModel, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VirtualMachineCPUModelCache) List(_ labels.Selector) ([]*harvesterv1.VirtualMachineCPUModel, error) {
	panic("implement me")
}

func (c VirtualMachineCPUModelCache) AddIndexer(_ string, _ generic.Indexer[*harvesterv1.VirtualMachineCPUModel]) {
	panic("implement me")
}

func (c VirtualMachineCPUModelCache) GetByIndex(_, _ string) ([]*harvesterv1.VirtualMachineCPUModel, error) {
	panic("implement me")
}
