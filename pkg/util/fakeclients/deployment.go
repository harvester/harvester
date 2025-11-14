package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

func (c DeploymentClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
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

type DeploymentCache func(string) appsv1type.DeploymentInterface

func (c DeploymentCache) Get(namespace, name string) (*appsv1.Deployment, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c DeploymentCache) List(namespace string, selector labels.Selector) ([]*appsv1.Deployment, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*appsv1.Deployment, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}
func (c DeploymentCache) AddIndexer(_ string, _ generic.Indexer[*appsv1.Deployment]) {
	panic("implement me")
}
func (c DeploymentCache) GetByIndex(_, _ string) ([]*appsv1.Deployment, error) {
	panic("implement me")
}
