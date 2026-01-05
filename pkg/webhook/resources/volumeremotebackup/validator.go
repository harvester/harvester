package volumeremotebackup

import (
	"fmt"

	ctlv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/volumeremotebackup/common"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	fieldType   = "spec.type"
	fieldSource = "spec.source"
	fieldFrom   = "spec.from"
)

type remoteBackupValidator struct {
	types.DefaultValidator
	pvcCache ctlv1.PersistentVolumeClaimCache
	bo       common.BackupOperator
}

func NewBackupValidator(
	pvcCache ctlv1.PersistentVolumeClaimCache,
	remoteBackupClient ctlharvesterv1.VolumeRemoteBackupClient,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
) types.Validator {
	return &remoteBackupValidator{
		pvcCache: pvcCache,
		bo:       common.NewBackupOperator(remoteBackupClient, pvcCache, scCache, settingCache),
	}
}

func (v *remoteBackupValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.VolumeRemoteBackupResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.VolumeRemoteBackup{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *remoteBackupValidator) Create(request *types.Request, newObj runtime.Object) error {
	vrb := newObj.(*v1beta1.VolumeRemoteBackup)

	if v.bo.GetSource(vrb) == "" {
		return werror.NewInvalidError("spec.source is required", fieldSource)
	}

	// Check if PVC exists in the same namespace
	_, err := v.pvcCache.Get(v.bo.GetNamespace(vrb), v.bo.GetSource(vrb))
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get PVC %s/%s: %v", v.bo.GetNamespace(vrb), v.bo.GetSource(vrb), err))
	}

	return nil
}

func (v *remoteBackupValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldVrb := oldObj.(*v1beta1.VolumeRemoteBackup)
	newVrb := newObj.(*v1beta1.VolumeRemoteBackup)

	if v.bo.GetType(oldVrb) != v.bo.GetType(newVrb) {
		return werror.NewInvalidError("spec.type cannot be changed", fieldType)
	}

	if v.bo.GetSource(oldVrb) != v.bo.GetSource(newVrb) {
		return werror.NewInvalidError("spec.source cannot be changed", fieldSource)
	}

	return nil
}

type remoteRestoreValidator struct {
	types.DefaultValidator
	remoteBackupCache ctlharvesterv1.VolumeRemoteBackupCache
	ro                common.RestoreOperator
}

func NewRestoreValidator(
	remoteRestoreClient ctlharvesterv1.VolumeRemoteRestoreClient,
	pvcCache ctlv1.PersistentVolumeClaimCache,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
	remoteBackupCache ctlharvesterv1.VolumeRemoteBackupCache,
	bo common.BackupOperator,
) types.Validator {
	return &remoteRestoreValidator{
		remoteBackupCache: remoteBackupCache,
		ro: common.NewRestoreOperator(
			remoteRestoreClient,
			pvcCache,
			scCache,
			settingCache,
			remoteBackupCache,
			bo,
		),
	}
}

func (v *remoteRestoreValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.VolumeRemoteRestoreResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.VolumeRemoteRestore{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *remoteRestoreValidator) Create(request *types.Request, newObj runtime.Object) error {
	vrr := newObj.(*v1beta1.VolumeRemoteRestore)

	if v.ro.GetFrom(vrr) == "" {
		return werror.NewInvalidError("spec.from is required", fieldFrom)
	}

	// Parse the backup reference (namespace/name format)
	vrbNamespace, vrbName := ref.Parse(v.ro.GetFrom(vrr))
	if vrbNamespace == "" {
		vrbNamespace = v.ro.GetNamespace(vrr)
	}

	// Check if the PVCBackup exists and is ready
	vrb, err := v.remoteBackupCache.Get(vrbNamespace, vrbName)
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get PVCBackup %s/%s: %v", vrbNamespace, vrbName, err))
	}

	// Check if the backup is ready (success = true)
	if !vrb.Status.Success {
		return werror.NewInvalidError(fmt.Sprintf("PVCBackup %s/%s is not ready", vrbNamespace, vrbName), fieldFrom)
	}

	// Check if a PVCBackup with the same name exists in the restore's namespace
	_, err = v.remoteBackupCache.Get(v.ro.GetNamespace(vrr), v.ro.GetName(vrr))
	if err == nil {
		return werror.NewInvalidError(fmt.Sprintf("a RemoteBackup with name %s already exists in namespace %s",
			v.ro.GetName(vrr), v.ro.GetNamespace(vrr)), "metadata.name")
	}

	return nil
}

func (v *remoteRestoreValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldVrr := oldObj.(*v1beta1.VolumeRemoteRestore)
	newVrr := newObj.(*v1beta1.VolumeRemoteRestore)

	if v.ro.GetType(oldVrr) != v.ro.GetType(newVrr) {
		return werror.NewInvalidError("spec.type cannot be changed", fieldType)
	}

	if v.ro.GetFrom(oldVrr) != v.ro.GetFrom(newVrr) {
		return werror.NewInvalidError("spec.from cannot be changed", fieldFrom)
	}

	return nil
}
