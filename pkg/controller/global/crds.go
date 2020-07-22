package global

import (
	"context"
	"fmt"

	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/vm/pkg/apis/vm.cattle.io/v1alpha1"
	pkgcontext "github.com/rancher/vm/pkg/context"
	"github.com/rancher/wrangler/pkg/crd"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func createCRDs(ctx context.Context, _ *pkgcontext.Scaled, server *server.Server) error {
	factory, err := crd.NewFactoryFromClient(server.RestConfig)
	if err != nil {
		return err
	}
	return factory.
		BatchCreateCRDs(ctx, crd.NonNamespacedTypes(
			getCRDName(v1alpha1.SchemeGroupVersion, "Setting"),
			getCRDName(v1alpha1.SchemeGroupVersion, "Image"),
		)...).
		BatchCreateCRDs(ctx, crd.NamespacedTypes()...).
		BatchWait()
}

func getCRDName(gv schema.GroupVersion, name string) string {
	return fmt.Sprintf("%s.%s/%s", name, gv.Group, gv.Version)
}
