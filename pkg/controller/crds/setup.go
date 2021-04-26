package crds

import (
	"context"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	rancherv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	wcrd "github.com/rancher/wrangler/pkg/crd"
	"k8s.io/client-go/rest"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util/crd"
)

func Setup(ctx context.Context, restConfig *rest.Config) error {
	return createCRDs(ctx, restConfig)
}

func createCRDs(ctx context.Context, restConfig *rest.Config) error {
	factory, err := crd.NewFactoryFromClient(ctx, restConfig)
	if err != nil {
		return err
	}
	return factory.
		BatchCreateCRDsIfNotExisted(
			crd.NonNamespacedFromGV(harvesterv1.SchemeGroupVersion, "Setting"),
			crd.NonNamespacedFromGV(harvesterv1.SchemeGroupVersion, "User"),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "Setting"),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "User"),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "UserAttribute"),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "Token"),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "NodeDriver"),
		).
		BatchCreateCRDsIfNotExisted(
			crd.FromGV(harvesterv1.SchemeGroupVersion, "KeyPair"),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "Upgrade"),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineImage"),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineTemplate"),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineTemplateVersion"),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineBackup"),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineBackupContent"),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineRestore"),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "Preference"),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "SupportBundle"),
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
