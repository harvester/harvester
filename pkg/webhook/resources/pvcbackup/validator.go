package pvcbackup

import (
	"fmt"

	ctlv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/pvcbackup/common"
	"github.com/harvester/harvester/pkg/ref"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	fieldType   = "spec.type"
	fieldSource = "spec.source"
	fieldFrom   = "spec.from"
)

type pvcBackupValidator struct {
	types.DefaultValidator
	pvcCache ctlv1.PersistentVolumeClaimCache
	pbo      common.PVCBackupOperator
}

func NewBackupValidator(
	pvcCache ctlv1.PersistentVolumeClaimCache,
	pvcBackupClient ctlharvesterv1.PVCBackupClient,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
) types.Validator {
	return &pvcBackupValidator{
		pvcCache: pvcCache,
		pbo:      common.GetPVCBackupOperator(pvcBackupClient, pvcCache, scCache, settingCache),
	}
}

func (v *pvcBackupValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.PVCBackupResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.PVCBackup{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *pvcBackupValidator) Create(request *types.Request, newObj runtime.Object) error {
	pb := newObj.(*v1beta1.PVCBackup)

	if v.pbo.GetSource(pb) == "" {
		return werror.NewInvalidError("spec.source is required", fieldSource)
	}

	// Check if PVC exists in the same namespace
	_, err := v.pvcCache.Get(v.pbo.GetNamespace(pb), v.pbo.GetSource(pb))
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get PVC %s/%s: %v", v.pbo.GetNamespace(pb), v.pbo.GetSource(pb), err))
	}

	return nil
}

func (v *pvcBackupValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldPb := oldObj.(*v1beta1.PVCBackup)
	newPb := newObj.(*v1beta1.PVCBackup)

	if v.pbo.GetType(oldPb) != v.pbo.GetType(newPb) {
		return werror.NewInvalidError("spec.type cannot be changed", fieldType)
	}

	if v.pbo.GetSource(oldPb) != v.pbo.GetSource(newPb) {
		return werror.NewInvalidError("spec.source cannot be changed", fieldSource)
	}

	return nil
}

type pvcRestoreValidator struct {
	types.DefaultValidator
	pvcBackupCache ctlharvesterv1.PVCBackupCache
	pro            common.PVCRestoreOperator
}

func NewRestoreValidator(
	pvcRestoreClient ctlharvesterv1.PVCRestoreClient,
	pvcCache ctlv1.PersistentVolumeClaimCache,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
	pvcBackupCache ctlharvesterv1.PVCBackupCache,
	pbo common.PVCBackupOperator,
) types.Validator {
	return &pvcRestoreValidator{
		pvcBackupCache: pvcBackupCache,
		pro: common.GetPVCRestoreOperator(
			pvcRestoreClient,
			pvcCache,
			scCache,
			settingCache,
			pvcBackupCache,
			pbo,
		),
	}
}

func (v *pvcRestoreValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.PVCRestoreResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.PVCRestore{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *pvcRestoreValidator) Create(request *types.Request, newObj runtime.Object) error {
	pr := newObj.(*v1beta1.PVCRestore)

	if v.pro.GetFrom(pr) == "" {
		return werror.NewInvalidError("spec.from is required", fieldFrom)
	}

	// Parse the backup reference (namespace/name format)
	pbNamespace, pbName := ref.Parse(v.pro.GetFrom(pr))
	if pbNamespace == "" {
		pbNamespace = v.pro.GetNamespace(pr)
	}

	// Check if the PVCBackup exists and is ready
	pb, err := v.pvcBackupCache.Get(pbNamespace, pbName)
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get PVCBackup %s/%s: %v", pbNamespace, pbName, err))
	}

	// Check if the backup is ready (success = true)
	if !pb.Status.Success {
		return werror.NewInvalidError(fmt.Sprintf("PVCBackup %s/%s is not ready", pbNamespace, pbName), fieldFrom)
	}

	// Check if a PVCBackup with the same name exists in the restore's namespace
	_, err = v.pvcBackupCache.Get(v.pro.GetNamespace(pr), v.pro.GetName(pr))
	if err == nil {
		return werror.NewInvalidError(fmt.Sprintf("a PVCBackup with name %s already exists in namespace %s",
			v.pro.GetName(pr), v.pro.GetNamespace(pr)), "metadata.name")
	}

	return nil
}

func (v *pvcRestoreValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldPr := oldObj.(*v1beta1.PVCRestore)
	newPr := newObj.(*v1beta1.PVCRestore)

	if v.pro.GetType(oldPr) != v.pro.GetType(newPr) {
		return werror.NewInvalidError("spec.type cannot be changed", fieldType)
	}

	if v.pro.GetFrom(oldPr) != v.pro.GetFrom(newPr) {
		return werror.NewInvalidError("spec.from cannot be changed", fieldFrom)
	}

	return nil
}
