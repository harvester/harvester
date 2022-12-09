package fakeclients

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	kubevirtv1api "kubevirt.io/api/core/v1"

	kubevirtv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
	kubevirtctlv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

type VirtualMachineInstanceClient func(string) kubevirtv1.VirtualMachineInstanceInterface

func (c VirtualMachineInstanceClient) Update(vmi *kubevirtv1api.VirtualMachineInstance) (*kubevirtv1api.VirtualMachineInstance, error) {
	return c(vmi.Namespace).Update(context.TODO(), vmi, metav1.UpdateOptions{})
}

func (c VirtualMachineInstanceClient) Get(namespace, name string, options metav1.GetOptions) (*kubevirtv1api.VirtualMachineInstance, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VirtualMachineInstanceClient) Create(vmi *kubevirtv1api.VirtualMachineInstance) (*kubevirtv1api.VirtualMachineInstance, error) {
	return c(vmi.Namespace).Create(context.TODO(), vmi, metav1.CreateOptions{})
}

func (c VirtualMachineInstanceClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c VirtualMachineInstanceClient) List(namespace string, opts metav1.ListOptions) (*kubevirtv1api.VirtualMachineInstanceList, error) {
	panic("implement me")
}

func (c VirtualMachineInstanceClient) UpdateStatus(*kubevirtv1api.VirtualMachineInstance) (*kubevirtv1api.VirtualMachineInstance, error) {
	panic("implement me")
}

func (c VirtualMachineInstanceClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c VirtualMachineInstanceClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *kubevirtv1api.VirtualMachineInstance, err error) {
	panic("implement me")
}

type VirtualMachineInstanceCache func(string) kubevirtv1.VirtualMachineInstanceInterface

func (c VirtualMachineInstanceCache) Get(namespace, name string) (*kubevirtv1api.VirtualMachineInstance, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VirtualMachineInstanceCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1api.VirtualMachineInstance, error) {
	panic("implement me")
}

func (c VirtualMachineInstanceCache) AddIndexer(indexName string, indexer kubevirtctlv1.VirtualMachineInstanceIndexer) {
	panic("implement me")
}

func (c VirtualMachineInstanceCache) GetByIndex(indexName, key string) ([]*kubevirtv1api.VirtualMachineInstance, error) {
	panic("implement me")
}
