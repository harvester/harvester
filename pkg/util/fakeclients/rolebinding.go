package fakeclients

import (
	"context"

	ctlrbacv1 "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
)

type RoleBindingClient func(string) v1.RoleBindingInterface

func (r RoleBindingClient) Create(rbac *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	return r(rbac.Namespace).Create(context.TODO(), rbac, metav1.CreateOptions{})
}

func (r RoleBindingClient) Update(rbac *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	return r(rbac.Namespace).Update(context.TODO(), rbac, metav1.UpdateOptions{})
}

func (r RoleBindingClient) UpdateStatus(rbac *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	panic("implement me")
}

func (r RoleBindingClient) Get(namespace, name string, options metav1.GetOptions) (*rbacv1.RoleBinding, error) {
	panic("implement me")
}

func (r RoleBindingClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}

func (r RoleBindingClient) List(namespace string, options metav1.ListOptions) (*rbacv1.RoleBindingList, error) {
	panic("implement me")
}

func (r RoleBindingClient) Watch(namespace string, options metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (r RoleBindingClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *rbacv1.RoleBinding, err error) {
	panic("implement me")
}

type RoleBindingCache func(string) v1.RoleBindingInterface

func (r RoleBindingCache) Get(namespace, name string) (*rbacv1.RoleBinding, error) {
	return r(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (r RoleBindingCache) List(namespace string, selector labels.Selector) ([]*rbacv1.RoleBinding, error) {
	panic("implement me")
}

func (r RoleBindingCache) AddIndexer(indexName string, indexer ctlrbacv1.RoleBindingIndexer) {
	panic("implement me")
}

func (c RoleBindingCache) GetByIndex(indexName, key string) ([]*rbacv1.RoleBinding, error) {
	panic("implement me")
}
