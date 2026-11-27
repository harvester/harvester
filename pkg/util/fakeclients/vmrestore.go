package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvestertype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
)

type VMRestoreClient func(string) harvestertype.VirtualMachineRestoreInterface

func (c VMRestoreClient) Create(vmr *harvesterv1beta1.VirtualMachineRestore) (*harvesterv1beta1.VirtualMachineRestore, error) {
	return c(vmr.Namespace).Create(context.TODO(), vmr, metav1.CreateOptions{})
}

func (c VMRestoreClient) Update(vmr *harvesterv1beta1.VirtualMachineRestore) (*harvesterv1beta1.VirtualMachineRestore, error) {
	return c(vmr.Namespace).Update(context.TODO(), vmr, metav1.UpdateOptions{})
}

func (c VMRestoreClient) UpdateStatus(vmr *harvesterv1beta1.VirtualMachineRestore) (*harvesterv1beta1.VirtualMachineRestore, error) {
	return c(vmr.Namespace).UpdateStatus(context.TODO(), vmr, metav1.UpdateOptions{})
}

func (c VMRestoreClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	if options == nil {
		options = &metav1.DeleteOptions{}
	}
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c VMRestoreClient) Get(namespace, name string, options metav1.GetOptions) (*harvesterv1beta1.VirtualMachineRestore, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c VMRestoreClient) List(namespace string, opts metav1.ListOptions) (*harvesterv1beta1.VirtualMachineRestoreList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c VMRestoreClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c VMRestoreClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1beta1.VirtualMachineRestore, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c VMRestoreClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*harvesterv1beta1.VirtualMachineRestore, *harvesterv1beta1.VirtualMachineRestoreList], error) {
	panic("implement me")
}
