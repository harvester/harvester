package fakeclients

import (
	rkev1 "github.com/rancher/rancher/pkg/apis/rke.cattle.io/v1"
	rkev1controller "github.com/rancher/rancher/pkg/generated/controllers/rke.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

type RKEControlPlaneCache func(string) rkev1controller.RKEControlPlaneClient

func (c RKEControlPlaneCache) Get(namespace, name string) (*rkev1.RKEControlPlane, error) {
	return c(namespace).Get(namespace, name, metav1.GetOptions{})
}
func (c RKEControlPlaneCache) List(namespace string, selector labels.Selector) ([]*rkev1.RKEControlPlane, error) {
	panic("implement me")
}
func (c RKEControlPlaneCache) AddIndexer(_ string, _ generic.Indexer[*rkev1.RKEControlPlane]) {
	panic("implement me")
}
func (c RKEControlPlaneCache) GetByIndex(_, _ string) ([]*rkev1.RKEControlPlane, error) {
	panic("implement me")
}

type MockRKEControlPlaneClient struct {
	RKEControlPlanes map[string]*rkev1.RKEControlPlane
}

func (m *MockRKEControlPlaneClient) Get(namespace, name string, opts metav1.GetOptions) (*rkev1.RKEControlPlane, error) {
	if cp, ok := m.RKEControlPlanes[namespace+"/"+name]; ok {
		return cp, nil
	}
	return nil, errors.NewNotFound(schema.GroupResource{Group: "rke.cattle.io", Resource: "rkecontrolplanes"}, name)
}

func (m *MockRKEControlPlaneClient) Create(*rkev1.RKEControlPlane) (*rkev1.RKEControlPlane, error) {
	panic("implement me")
}

func (m *MockRKEControlPlaneClient) Update(*rkev1.RKEControlPlane) (*rkev1.RKEControlPlane, error) {
	panic("implement me")
}

func (m *MockRKEControlPlaneClient) Delete(namespace, name string, opts *metav1.DeleteOptions) error {
	panic("implement me")
}

func (m *MockRKEControlPlaneClient) List(namespace string, opts metav1.ListOptions) (*rkev1.RKEControlPlaneList, error) {
	panic("implement me")
}

func (m *MockRKEControlPlaneClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (m *MockRKEControlPlaneClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (*rkev1.RKEControlPlane, error) {
	panic("implement me")
}

func (m *MockRKEControlPlaneClient) UpdateStatus(*rkev1.RKEControlPlane) (*rkev1.RKEControlPlane, error) {
	panic("implement me")
}

func (m *MockRKEControlPlaneClient) WithImpersonation(rest.ImpersonationConfig) (generic.ClientInterface[*rkev1.RKEControlPlane, *rkev1.RKEControlPlaneList], error) {
	panic("implement me")
}
