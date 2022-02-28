package indexeres

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/ref"
)

const (
	UserNameIndex           = "auth.harvesterhci.io/user-username-index"
	RbByRoleAndSubjectIndex = "auth.harvesterhci.io/crb-by-role-and-subject"
	PVCByVMIndex            = "harvesterhci.io/pvc-by-vm-index"
	VMByNetworkIndex        = "vm.harvesterhci.io/vm-by-network"
)

func RegisterScaledIndexers(scaled *config.Scaled) {
	vmInformer := scaled.Management.VirtFactory.Kubevirt().V1().VirtualMachine().Cache()
	vmInformer.AddIndexer(VMByNetworkIndex, VMByNetwork)
}

func RegisterManagementIndexers(management *config.Management) {
	crbInformer := management.RbacFactory.Rbac().V1().ClusterRoleBinding().Cache()
	crbInformer.AddIndexer(RbByRoleAndSubjectIndex, rbByRoleAndSubject)
	pvcInformer := management.CoreFactory.Core().V1().PersistentVolumeClaim().Cache()
	pvcInformer.AddIndexer(PVCByVMIndex, pvcByVM)
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

func pvcByVM(obj *corev1.PersistentVolumeClaim) ([]string, error) {
	annotationSchemaOwners, err := ref.GetSchemaOwnersFromAnnotation(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema owners from PVC %s's annotation: %w", obj.Name, err)
	}
	return annotationSchemaOwners.List(kubevirtv1.VirtualMachineGroupVersionKind.GroupKind()), nil
}

func VMByNetwork(obj *kubevirtv1.VirtualMachine) ([]string, error) {
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
