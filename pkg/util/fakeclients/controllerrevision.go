package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	k8sappsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/client-go/rest"
)

type ControllerRevisionClient func(string) appsv1.ControllerRevisionInterface

func (c ControllerRevisionClient) Update(cr *k8sappsv1.ControllerRevision) (*k8sappsv1.ControllerRevision, error) {
	return c(cr.Namespace).Update(context.TODO(), cr, metav1.UpdateOptions{})
}

func (c ControllerRevisionClient) Get(namespace, name string, _ metav1.GetOptions) (*k8sappsv1.ControllerRevision, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c ControllerRevisionClient) Create(cr *k8sappsv1.ControllerRevision) (*k8sappsv1.ControllerRevision, error) {
	return c(cr.Namespace).Create(context.TODO(), cr, metav1.CreateOptions{})
}

func (c ControllerRevisionClient) Delete(namespace, name string, _ *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c ControllerRevisionClient) List(_ string, _ metav1.ListOptions) (*k8sappsv1.ControllerRevisionList, error) {
	panic("implement me")
}

func (c ControllerRevisionClient) UpdateStatus(_ *k8sappsv1.ControllerRevision) (*k8sappsv1.ControllerRevision, error) {
	panic("implement me")
}

func (c ControllerRevisionClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c ControllerRevisionClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *k8sappsv1.ControllerRevision, err error) {
	panic("implement me")
}

func (c ControllerRevisionClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*k8sappsv1.ControllerRevision, *k8sappsv1.ControllerRevisionList], error) {
	panic("implement me")
}

type ControllerRevisionCache func(string) appsv1.ControllerRevisionInterface

func (c ControllerRevisionCache) Get(namespace, name string) (*k8sappsv1.ControllerRevision, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c ControllerRevisionCache) List(_ string, _ labels.Selector) ([]*k8sappsv1.ControllerRevision, error) {
	panic("implement me")
}

func (c ControllerRevisionCache) AddIndexer(_ string, _ generic.Indexer[*k8sappsv1.ControllerRevision]) {
	panic("implement me")
}

func (c ControllerRevisionCache) GetByIndex(_, _ string) ([]*k8sappsv1.ControllerRevision, error) {
	panic("implement me")
}
