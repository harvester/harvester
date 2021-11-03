package fakeclients

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	kubevirtv1api "kubevirt.io/client-go/api/v1"

	kubevirtv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
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
