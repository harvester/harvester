package fakeclients

import (
	"context"
	"time"

	"github.com/rancher/wrangler/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	kubevirtv1api "kubevirt.io/api/core/v1"

	kubevirtv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
	kubevirtctlv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

type VirtualMachineClient func(string) kubevirtv1.VirtualMachineInterface

func (c VirtualMachineClient) Update(virtualMachine *kubevirtv1api.VirtualMachine) (*kubevirtv1api.VirtualMachine, error) {
	return c(virtualMachine.Namespace).Update(context.TODO(), virtualMachine, metav1.UpdateOptions{})
}

func (c VirtualMachineClient) Get(namespace, name string, options metav1.GetOptions) (*kubevirtv1api.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VirtualMachineClient) Create(virtualMachine *kubevirtv1api.VirtualMachine) (*kubevirtv1api.VirtualMachine, error) {
	return c(virtualMachine.Namespace).Create(context.TODO(), virtualMachine, metav1.CreateOptions{})
}

func (c VirtualMachineClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c VirtualMachineClient) List(namespace string, opts metav1.ListOptions) (*kubevirtv1api.VirtualMachineList, error) {
	panic("implement me")
}

func (c VirtualMachineClient) UpdateStatus(*kubevirtv1api.VirtualMachine) (*kubevirtv1api.VirtualMachine, error) {
	panic("implement me")
}

func (c VirtualMachineClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c VirtualMachineClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *kubevirtv1api.VirtualMachine, err error) {
	panic("implement me")
}

func (c VirtualMachineClient) Informer() cache.SharedIndexInformer {
	panic("implement me")
}

func (c VirtualMachineClient) GroupVersionKind() schema.GroupVersionKind {
	panic("implement me")
}

func (c VirtualMachineClient) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	panic("implement me")
}

func (c VirtualMachineClient) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	panic("implement me")
}

func (c VirtualMachineClient) Updater() generic.Updater {
	panic("implement me")
}

func (c VirtualMachineClient) OnChange(ctx context.Context, name string, sync kubevirtctlv1.VirtualMachineHandler) {
	panic("implement me")
}

func (c VirtualMachineClient) OnRemove(ctx context.Context, name string, sync kubevirtctlv1.VirtualMachineHandler) {
	panic("implement me")
}

func (c VirtualMachineClient) Enqueue(namespace, name string) {
	panic("implement me")
}

func (c VirtualMachineClient) EnqueueAfter(namespace, name string, duration time.Duration) {
	panic("implement me")
}

func (c VirtualMachineClient) Cache() kubevirtctlv1.VirtualMachineCache {
	panic("implement me")
}

type VirtualMachineCache func(string) kubevirtv1.VirtualMachineInterface

func (c VirtualMachineCache) Get(namespace, name string) (*kubevirtv1api.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VirtualMachineCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1api.VirtualMachine, error) {
	panic("implement me")
}

func (c VirtualMachineCache) AddIndexer(indexName string, indexer kubevirtctlv1.VirtualMachineIndexer) {
	panic("implement me")
}

func (c VirtualMachineCache) GetByIndex(indexName, key string) ([]*kubevirtv1api.VirtualMachine, error) {
	panic("implement me")
}
