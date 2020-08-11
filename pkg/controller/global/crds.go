package global

import (
	"context"
	"fmt"

	"github.com/rancher/harvester/pkg/apis/vm.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/steve/pkg/server"
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
			getCRDName(v1alpha1.SchemeGroupVersion, "keyPair"),
			getCRDName(v1alpha1.SchemeGroupVersion, "Template"),
			getCRDName(v1alpha1.SchemeGroupVersion, "TemplateVersion"),
		)...).
		BatchWait()
}

func getCRDName(gv schema.GroupVersion, name string) string {
	return fmt.Sprintf("%s.%s/%s", name, gv.Group, gv.Version)
}
