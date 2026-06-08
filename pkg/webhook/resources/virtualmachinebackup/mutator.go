package virtualmachinebackup

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type BackupMutator struct {
	types.DefaultMutator
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache
	vmbr          common.VMBackupReader
}

func NewMutator(vmBackupCache ctlharvesterv1.VirtualMachineBackupCache) *BackupMutator {
	return &BackupMutator{
		vmBackupCache: vmBackupCache,
		vmbr:          common.NewVMBackupReader(),
	}
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
	newVb := newObj.(*v1beta1.VirtualMachineBackup)
	existingVb, err := m.vmBackupCache.Get(m.vmbr.GetNamespace(newVb), m.vmbr.GetName(newVb))
	if apierrors.IsNotFound(err) {
		return types.PatchOps{}, nil
	}
	if err != nil {
		return types.PatchOps{}, err
	}

	existingType := m.vmbr.GetType(existingVb)
	newType := m.vmbr.GetType(newVb)
	if existingType == newType {
		return types.PatchOps{}, werror.NewBadRequest(
			fmt.Sprintf("%s %q already exists", newType, m.vmbr.GetName(newVb)))
	}

	// Backup and snapshot share the same CRD (VirtualMachineBackup), so names
	// collide across types. Catch this here because the schema check after the
	// mutator would surface a less helpful error.
	// ref: https://github.com/harvester/harvester/issues/5855
	return types.PatchOps{}, werror.NewBadRequest(
		fmt.Sprintf("name %q is already used by a %s (backup and snapshot share names)", m.vmbr.GetName(newVb), existingType))
}
