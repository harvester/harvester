package virtualmachinebackup

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type BackupMutator struct {
	types.DefaultMutator
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache
}

func NewMutator(vmBackupCache ctlharvesterv1.VirtualMachineBackupCache) *BackupMutator {
	return &BackupMutator{vmBackupCache: vmBackupCache}
}

func (m *BackupMutator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"virtualmachinebackups"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.VirtualMachineBackup{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (m *BackupMutator) Create(_ *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	newVMBackup := newObj.(*v1beta1.VirtualMachineBackup)
	vmBackup, err := m.vmBackupCache.Get(newVMBackup.Namespace, newVMBackup.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return types.PatchOps{}, nil
		}
		return types.PatchOps{}, err
	}
	if newVMBackup.Spec.Type == vmBackup.Spec.Type {
		return types.PatchOps{}, werror.NewBadRequest(
			fmt.Sprintf("The %s %s is already existent. Please use another name for it.", vmBackup.Name, vmBackup.Spec.Type))
	}
	// https://github.com/harvester/harvester/issues/5855
	// To better handle error message about duplicated vm backup and snapshot,
	// we have to do it in mutator. After mutator, there is a schema check and
	// we can't handle the error message in it.
	return types.PatchOps{}, werror.NewBadRequest(
		fmt.Sprintf("VM Backup and Snapshot use same CRD - VirtualMachineBackup. The %s %s is already existent. Please use another name for it.", vmBackup.Name, vmBackup.Spec.Type))
}
