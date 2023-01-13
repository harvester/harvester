package fakeclients

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	appsv1type "k8s.io/client-go/kubernetes/typed/apps/v1"
)

type DeploymentClient func(string) appsv1type.DeploymentInterface

func (c DeploymentClient) Update(deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	panic("implement me")
}
func (c DeploymentClient) Get(namespace, name string, options metav1.GetOptions) (*appsv1.Deployment, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c DeploymentClient) Create(deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	return c(deployment.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
}
func (c DeploymentClient) UpdateStatus(*appsv1.Deployment) (*appsv1.Deployment, error) {
	panic("implement me")
}
func (c DeploymentClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}
func (c DeploymentClient) List(namespace string, opts metav1.ListOptions) (*appsv1.DeploymentList, error) {
	panic("implement me")
}
func (c DeploymentClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c DeploymentClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *appsv1.Deployment, err error) {
	panic("implement me")
}
