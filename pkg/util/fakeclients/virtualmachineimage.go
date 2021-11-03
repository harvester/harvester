package fakeclients

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/tests/framework/fuzz"
)

type VirtualMachineImageClient func(string) harv1type.VirtualMachineImageInterface

func (c VirtualMachineImageClient) Update(virtualMachineImage *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	return c(virtualMachineImage.Namespace).Update(context.TODO(), virtualMachineImage, metav1.UpdateOptions{})
}
func (c VirtualMachineImageClient) Get(namespace, name string, options metav1.GetOptions) (*harvesterv1.VirtualMachineImage, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c VirtualMachineImageClient) Create(virtualMachineImage *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if virtualMachineImage.GenerateName != "" {
		virtualMachineImage.Name = fmt.Sprintf("%s%s", virtualMachineImage.GenerateName, fuzz.String(5))
	}
	return c(virtualMachineImage.Namespace).Create(context.TODO(), virtualMachineImage, metav1.CreateOptions{})
}
func (c VirtualMachineImageClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}
func (c VirtualMachineImageClient) List(namespace string, opts metav1.ListOptions) (*harvesterv1.VirtualMachineImageList, error) {
	panic("implement me")
}
func (c VirtualMachineImageClient) UpdateStatus(*harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	panic("implement me")
}
func (c VirtualMachineImageClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c VirtualMachineImageClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1.VirtualMachineImage, err error) {
	panic("implement me")
}

type VirtualMachineImageCache func(string) harv1type.VirtualMachineImageInterface

func (c VirtualMachineImageCache) Get(namespace, name string) (*harvesterv1.VirtualMachineImage, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c VirtualMachineImageCache) List(namespace string, selector labels.Selector) ([]*harvesterv1.VirtualMachineImage, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*harvesterv1.VirtualMachineImage, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}
func (c VirtualMachineImageCache) AddIndexer(indexName string, indexer ctlharvesterv1.VirtualMachineImageIndexer) {
	panic("implement me")
}
func (c VirtualMachineImageCache) GetByIndex(indexName, key string) ([]*harvesterv1.VirtualMachineImage, error) {
	panic("implement me")
}
