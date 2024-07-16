package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	kubevirtv1api "kubevirt.io/api/core/v1"

	kubevirtv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
)

type VirtualMachineInstanceClient func(string) kubevirtv1.VirtualMachineInstanceInterface

func (c VirtualMachineInstanceClient) Update(vmi *kubevirtv1api.VirtualMachineInstance) (*kubevirtv1api.VirtualMachineInstance, error) {
	return c(vmi.Namespace).Update(context.TODO(), vmi, metav1.UpdateOptions{})
}

func (c VirtualMachineInstanceClient) Get(namespace, name string, options metav1.GetOptions) (*kubevirtv1api.VirtualMachineInstance, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c VirtualMachineInstanceClient) Create(vmi *kubevirtv1api.VirtualMachineInstance) (*kubevirtv1api.VirtualMachineInstance, error) {
	return c(vmi.Namespace).Create(context.TODO(), vmi, metav1.CreateOptions{})
}

func (c VirtualMachineInstanceClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c VirtualMachineInstanceClient) List(_ string, _ metav1.ListOptions) (*kubevirtv1api.VirtualMachineInstanceList, error) {
	panic("implement me")
}

func (c VirtualMachineInstanceClient) UpdateStatus(*kubevirtv1api.VirtualMachineInstance) (*kubevirtv1api.VirtualMachineInstance, error) {
	panic("implement me")
}

func (c VirtualMachineInstanceClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c VirtualMachineInstanceClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *kubevirtv1api.VirtualMachineInstance, err error) {
	panic("implement me")
}

func (c VirtualMachineInstanceClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*kubevirtv1api.VirtualMachineInstance, *kubevirtv1api.VirtualMachineInstanceList], error) {
	panic("implement me")
}

type VirtualMachineInstanceCache func(string) kubevirtv1.VirtualMachineInstanceInterface

func (c VirtualMachineInstanceCache) Get(namespace, name string) (*kubevirtv1api.VirtualMachineInstance, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VirtualMachineInstanceCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1api.VirtualMachineInstance, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*kubevirtv1api.VirtualMachineInstance, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c VirtualMachineInstanceCache) AddIndexer(_ string, _ generic.Indexer[*kubevirtv1api.VirtualMachineInstance]) {
	panic("implement me")
}

func (c VirtualMachineInstanceCache) GetByIndex(_, _ string) ([]*kubevirtv1api.VirtualMachineInstance, error) {
	panic("implement me")
}
