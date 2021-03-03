package crds

import (
	"context"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	"github.com/rancher/steve/pkg/server"
	wcrd "github.com/rancher/wrangler/pkg/crd"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/util/crd"
)

func Setup(ctx context.Context, server *server.Server, controllers *server.Controllers, options config.Options) error {
	return createCRDs(ctx, server)
}

func createCRDs(ctx context.Context, server *server.Server) error {
	factory, err := crd.NewFactoryFromClient(ctx, server.RESTConfig)
	if err != nil {
		return err
	}
	return factory.
		BatchCreateCRDsIfNotExisted(
			crd.NonNamespacedFromGV(v1alpha1.SchemeGroupVersion, "Setting"),
			crd.NonNamespacedFromGV(v1alpha1.SchemeGroupVersion, "User"),
		).
		BatchCreateCRDsIfNotExisted(
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineImage"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "KeyPair"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineTemplate"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineTemplateVersion"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineBackup"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineBackupContent"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "VirtualMachineRestore"),
			crd.FromGV(v1alpha1.SchemeGroupVersion, "Preference"),
			crd.FromGV(longhornv1.SchemeGroupVersion, "Volume"),
			crd.FromGV(longhornv1.SchemeGroupVersion, "Setting"),
			createNetworkAttachmentDefinitionCRD(),
		).
		BatchWait()
}

func createNetworkAttachmentDefinitionCRD() wcrd.CRD {
	nad := crd.FromGV(cniv1.SchemeGroupVersion, "NetworkAttachmentDefinition")
	nad.PluralName = "network-attachment-definitions"
	nad.SingularName = "network-attachment-definition"
	return nad
}
