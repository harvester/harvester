package crds

import (
	"context"

	"github.com/rancher/steve/pkg/server"
	kubevirtv1 "kubevirt.io/client-go/api/v1alpha3"

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
		).
		CreateCRDsIfNotExisted(
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineImage"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "KeyPair"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineTemplate"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineTemplateVersion"),
		).
		CreateCRDsIfNotExisted(
			crd.FromGV(kubevirtv1.SchemeGroupVersion, "VirtualMachine"),
			crd.FromGV(kubevirtv1.SchemeGroupVersion, "VirtualMachineInstance"),
		).
		Wait()
}
