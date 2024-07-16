package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	kubevirtv1 "kubevirt.io/api/core/v1"

	kubevirttype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
)

type VirtualMachineInstanceMigrationCache func(string) kubevirttype.VirtualMachineInstanceMigrationInterface

func (c VirtualMachineInstanceMigrationCache) Get(namespace, name string) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VirtualMachineInstanceMigrationCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1.VirtualMachineInstanceMigration, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*kubevirtv1.VirtualMachineInstanceMigration, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c VirtualMachineInstanceMigrationCache) AddIndexer(_ string, _ generic.Indexer[*kubevirtv1.VirtualMachineInstanceMigration]) {
	panic("implement me")
}

func (c VirtualMachineInstanceMigrationCache) GetByIndex(_, _ string) ([]*kubevirtv1.VirtualMachineInstanceMigration, error) {
	panic("implement me")
}

type VirtualMachineInstanceMigrationClient func(string) kubevirttype.VirtualMachineInstanceMigrationInterface

func (c VirtualMachineInstanceMigrationClient) Create(virtualMachine *kubevirtv1.VirtualMachineInstanceMigration) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return c(virtualMachine.Namespace).Create(context.TODO(), virtualMachine, metav1.CreateOptions{})
}

func (c VirtualMachineInstanceMigrationClient) Update(virtualMachine *kubevirtv1.VirtualMachineInstanceMigration) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return c(virtualMachine.Namespace).Update(context.TODO(), virtualMachine, metav1.UpdateOptions{})
}

func (c VirtualMachineInstanceMigrationClient) UpdateStatus(virtualMachine *kubevirtv1.VirtualMachineInstanceMigration) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return c(virtualMachine.Namespace).UpdateStatus(context.TODO(), virtualMachine, metav1.UpdateOptions{})
}

func (c VirtualMachineInstanceMigrationClient) Delete(namespace, name string, opts *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *opts)
}

func (c VirtualMachineInstanceMigrationClient) Get(namespace, name string, opts metav1.GetOptions) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c VirtualMachineInstanceMigrationClient) List(namespace string, opts metav1.ListOptions) (*kubevirtv1.VirtualMachineInstanceMigrationList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c VirtualMachineInstanceMigrationClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c VirtualMachineInstanceMigrationClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *kubevirtv1.VirtualMachineInstanceMigration, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c VirtualMachineInstanceMigrationClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*kubevirtv1.VirtualMachineInstanceMigration, *kubevirtv1.VirtualMachineInstanceMigrationList], error) {
	panic("implement me")
}
