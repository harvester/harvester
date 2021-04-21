package indexeres

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	kubevirtv1 "kubevirt.io/client-go/api/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/ref"
)

const (
	UserNameIndex           = "auth.harvesterhci.io/user-username-index"
	RbByRoleAndSubjectIndex = "auth.harvesterhci.io/crb-by-role-and-subject"
	DataVolumeByVMIndex     = "cdi.harvesterhci.io/datavolume-by-vm"
	VMByNetworkIndex        = "vm.harvesterhci.io/vm-by-network"
)

func RegisterScaledIndexers(scaled *config.Scaled) {
	userInformer := scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().User().Cache()
	userInformer.AddIndexer(UserNameIndex, indexUserByUsername)
	vmInformer := scaled.Management.VirtFactory.Kubevirt().V1().VirtualMachine().Cache()
	vmInformer.AddIndexer(VMByNetworkIndex, vmByNetwork)
}

func RegisterManagementIndexers(management *config.Management) {
	crbInformer := management.RbacFactory.Rbac().V1().ClusterRoleBinding().Cache()
	crbInformer.AddIndexer(RbByRoleAndSubjectIndex, rbByRoleAndSubject)
	dataVolumeInformer := management.CDIFactory.Cdi().V1beta1().DataVolume().Cache()
	dataVolumeInformer.AddIndexer(DataVolumeByVMIndex, dataVolumeByVM)
}

func indexUserByUsername(obj *harvesterv1.User) ([]string, error) {
	return []string{obj.Username}, nil
}

func rbByRoleAndSubject(obj *rbacv1.ClusterRoleBinding) ([]string, error) {
	keys := make([]string, 0, len(obj.Subjects))
	for _, s := range obj.Subjects {
		keys = append(keys, RbRoleSubjectKey(obj.RoleRef.Name, s))
	}
	return keys, nil
}

func RbRoleSubjectKey(roleName string, subject rbacv1.Subject) string {
	return roleName + "." + subject.Kind + "." + subject.Name
}

func dataVolumeByVM(obj *cdiv1beta1.DataVolume) ([]string, error) {
	annotationSchemaOwners, err := ref.GetSchemaOwnersFromAnnotation(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema owners from datavolume %s's annotation: %w", obj.Name, err)
	}
	return annotationSchemaOwners.List(kubevirtv1.VirtualMachineGroupVersionKind.GroupKind()), nil
}

func vmByNetwork(obj *kubevirtv1.VirtualMachine) ([]string, error) {
	networks := obj.Spec.Template.Spec.Networks
	networkNameList := make([]string, 0, len(networks))
	for _, network := range networks {
		if network.NetworkSource.Multus == nil {
			continue
		}
		networkNameList = append(networkNameList, network.NetworkSource.Multus.NetworkName)
	}
	return networkNameList, nil
}
