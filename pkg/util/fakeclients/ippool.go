package fakeclients

import (
	"context"

	cnitype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/whereabouts.cni.cncf.io/v1alpha1"
	v1alpha1 "github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

type IPPoolClient func() cnitype.IPPoolInterface

func (c IPPoolClient) Create(obj *v1alpha1.IPPool) (*v1alpha1.IPPool, error) {
	panic("implement me")
}

func (c IPPoolClient) Update(obj *v1alpha1.IPPool) (*v1alpha1.IPPool, error) {
	panic("implement me")
}

func (c IPPoolClient) UpdateStatus(obj *v1alpha1.IPPool) (*v1alpha1.IPPool, error) {
	panic("implement me")
}

func (c IPPoolClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c IPPoolClient) Get(namespace, name string, options metav1.GetOptions) (*v1alpha1.IPPool, error) {
	panic("implement me")
}

func (c IPPoolClient) List(namespace string, opts metav1.ListOptions) (*v1alpha1.IPPoolList, error) {
	panic("implement me")
}

func (c IPPoolClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c IPPoolClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.IPPool, err error) {
	panic("implement me")
}

func (c IPPoolClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*v1alpha1.IPPool, *v1alpha1.IPPoolList], error) {
	panic("implement me")
}
