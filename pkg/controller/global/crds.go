package global

import (
	"context"
	"fmt"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
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
			getCRDName(v1alpha1.SchemeGroupVersion, "VirtualMachineImage"),
			getCRDName(v1alpha1.SchemeGroupVersion, "KeyPair"),
			getCRDName(v1alpha1.SchemeGroupVersion, "VirtualMachineTemplate"),
			getCRDName(v1alpha1.SchemeGroupVersion, "VirtualMachineTemplateVersion"),
		)...).
		BatchWait()
}

func getCRDName(gv schema.GroupVersion, name string) string {
	return fmt.Sprintf("%s.%s/%s", name, gv.Group, gv.Version)
}
