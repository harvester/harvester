package global

import (
	"context"
	"fmt"

	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/vm/pkg/apis/vm.cattle.io/v1alpha1"
	"github.com/rancher/vm/pkg/config"
	"github.com/rancher/wrangler/pkg/crd"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func createCRDs(ctx context.Context, _ *config.Scaled, server *server.Server) error {
	factory, err := crd.NewFactoryFromClient(server.RestConfig)
	if err != nil {
		return err
	}
	return factory.
		BatchCreateCRDs(ctx, crd.NonNamespacedTypes(
			getCRDName(v1alpha1.SchemeGroupVersion, "Setting"),
		)...).
		BatchCreateCRDs(ctx, crd.NamespacedTypes(
			getCRDName(v1alpha1.SchemeGroupVersion, "Image"),
		)...).
		BatchWait()
}

func getCRDName(gv schema.GroupVersion, name string) string {
	return fmt.Sprintf("%s.%s/%s", name, gv.Group, gv.Version)
}
