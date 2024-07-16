package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	appsv1type "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/client-go/rest"
)

type DeploymentClient func(string) appsv1type.DeploymentInterface

func (c DeploymentClient) Update(deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	return c(deployment.Namespace).Update(context.TODO(), deployment, metav1.UpdateOptions{})
}

func (c DeploymentClient) Get(namespace, name string, options metav1.GetOptions) (*appsv1.Deployment, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c DeploymentClient) Create(deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	return c(deployment.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
}

func (c DeploymentClient) UpdateStatus(*appsv1.Deployment) (*appsv1.Deployment, error) {
	panic("implement me")
}

func (c DeploymentClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c DeploymentClient) List(_ string, _ metav1.ListOptions) (*appsv1.DeploymentList, error) {
	panic("implement me")
}

func (c DeploymentClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c DeploymentClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *appsv1.Deployment, err error) {
	panic("implement me")
}

func (c DeploymentClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*appsv1.Deployment, *appsv1.DeploymentList], error) {
	panic("implement me")
}
