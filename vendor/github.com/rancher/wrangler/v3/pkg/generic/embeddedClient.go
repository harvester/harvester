package generic

import (
	"context"

	"github.com/rancher/lasso/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

// embeddedClient is the interface for the lasso Client used by the controller.
type embeddedClient interface {
	Create(ctx context.Context, namespace string, obj runtime.Object, result runtime.Object, opts metav1.CreateOptions) (err error)
	Delete(ctx context.Context, namespace string, name string, opts metav1.DeleteOptions) error
	Get(ctx context.Context, namespace string, name string, result runtime.Object, options metav1.GetOptions) (err error)
	List(ctx context.Context, namespace string, result runtime.Object, opts metav1.ListOptions) (err error)
	Patch(ctx context.Context, namespace string, name string, pt types.PatchType, data []byte, result runtime.Object, opts metav1.PatchOptions, subresources ...string) (err error)
	Update(ctx context.Context, namespace string, obj runtime.Object, result runtime.Object, opts metav1.UpdateOptions) (err error)
	UpdateStatus(ctx context.Context, namespace string, obj runtime.Object, result runtime.Object, opts metav1.UpdateOptions) (err error)
	Watch(ctx context.Context, namespace string, opts metav1.ListOptions) (watch.Interface, error)
	WithImpersonation(impersonate rest.ImpersonationConfig) (*client.Client, error)
}
