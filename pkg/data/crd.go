package data

import (
	"context"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	rancherv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"k8s.io/client-go/rest"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util/crd"
)

func createCRDs(ctx context.Context, restConfig *rest.Config) error {
	factory, err := crd.NewFactoryFromClient(ctx, restConfig)
	if err != nil {
		return err
	}
	return factory.
		BatchCreateCRDsIfNotExisted(
			crd.NonNamespacedFromGV(harvesterv1.SchemeGroupVersion, "Setting", harvesterv1.Setting{}),
			crd.NonNamespacedFromGV(harvesterv1.SchemeGroupVersion, "User", harvesterv1.User{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "APIService", rancherv3.APIService{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "Setting", rancherv3.Setting{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "User", rancherv3.User{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "UserAttribute", rancherv3.UserAttribute{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "Group", rancherv3.Group{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "GroupMember", rancherv3.GroupMember{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "Token", rancherv3.Token{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "NodeDriver", rancherv3.NodeDriver{}),
		).
		BatchCreateCRDsIfNotExisted(
			crd.FromGV(harvesterv1.SchemeGroupVersion, "KeyPair", harvesterv1.KeyPair{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "Upgrade", harvesterv1.Upgrade{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineImage", harvesterv1.VirtualMachineImage{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineTemplate", harvesterv1.VirtualMachineTemplate{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineTemplateVersion", harvesterv1.VirtualMachineTemplateVersion{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineBackup", harvesterv1.VirtualMachineBackup{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineBackupContent", harvesterv1.VirtualMachineBackupContent{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineRestore", harvesterv1.VirtualMachineRestore{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "Preference", harvesterv1.Preference{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "SupportBundle", harvesterv1.SupportBundle{}),
			crd.FromGV(longhornv1.SchemeGroupVersion, "Volume", longhornv1.Volume{}),
			crd.FromGV(longhornv1.SchemeGroupVersion, "Setting", longhornv1.Setting{}),
		).
		BatchWait()
}
