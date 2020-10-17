package crds

import (
	"context"

	"github.com/rancher/steve/pkg/server"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/util/crd"
)

func Setup(ctx context.Context, server *server.Server) error {
	return createCRDs(ctx, server)
}

func createCRDs(ctx context.Context, server *server.Server) error {
	factory, err := crd.NewFactoryFromClient(ctx, server.RestConfig)
	if err != nil {
		return err
	}
	return factory.
		CreateCRDsIfNotExisted(
			crd.NonNamespacedFromGV(v1alpha1.SchemeGroupVersion, "Setting"),
			crd.NonNamespacedFromGV(v1alpha1.SchemeGroupVersion, "User"),
		).
		CreateCRDsIfNotExisted(
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineImage"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "KeyPair"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineTemplate"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineTemplateVersion"),
		).
		Wait()
}
