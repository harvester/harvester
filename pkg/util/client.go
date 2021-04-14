package util

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"kubevirt.io/kubevirt/pkg/virt-operator/resource/generate/rbac"
)

func VirtClientUpdateVmi(ctx context.Context, client rest.Interface, managementNamespace, namespace, name string, obj runtime.Object) error {
	return client.Put().
		Namespace(namespace).
		SetHeader("Impersonate-User", fmt.Sprintf("system:serviceaccount:%s:%s", managementNamespace, rbac.ControllerServiceAccountName)).
		Resource("virtualmachineinstances").
		Name(name).
		Body(obj).
		Do(ctx).
		Error()
}
