package cdi

import (
	"fmt"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	cdicaps "kubevirt.io/containerized-data-importer/pkg/storagecapabilities"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Validator struct {
	vmiv    common.VMIValidator
	scCache ctlstoragev1.StorageClassCache
	spCache ctlcdiv1.StorageProfileCache
	client  client.Client
}

func GetValidator(
	vmiv common.VMIValidator,
	scCache ctlstoragev1.StorageClassCache,
	spCache ctlcdiv1.StorageProfileCache,
	client client.Client) backend.Validator {
	return &Validator{vmiv, scCache, spCache, client}
}

func (cv *Validator) Create(req *types.Request, vmImg *harvesterv1.VirtualMachineImage) error {
	if err := cv.vmiv.CheckDisplayName(vmImg); err != nil {
		return err
	}

	if err := cv.vmiv.SCConsistency(nil, vmImg); err != nil {
		return err
	}

	if err := cv.vmiv.CheckURL(vmImg); err != nil {
		return err
	}

	if err := cv.vmiv.CheckPVCInUse(vmImg); err != nil {
		return err
	}

	if err := cv.vmiv.CheckImagePVC(req, vmImg); err != nil {
		return err
	}

	if err := cv.checkVolumeModeAccessModes(vmImg); err != nil {
		return err
	}

	return nil
}

func (cv *Validator) Update(oldVMImg, newVMImg *harvesterv1.VirtualMachineImage) error {
	if err := cv.vmiv.SourceTypeConsistency(oldVMImg, newVMImg); err != nil {
		return err
	}

	if err := cv.vmiv.SCConsistency(oldVMImg, newVMImg); err != nil {
		return err
	}

	if cv.vmiv.IsExportVolume(newVMImg) {
		if err := cv.vmiv.PVCConsistency(oldVMImg, newVMImg); err != nil {
			return err
		}
	}

	if err := cv.vmiv.URLConsistency(oldVMImg, newVMImg); err != nil {
		return err
	}

	if err := cv.vmiv.CheckUpdateDisplayName(oldVMImg, newVMImg); err != nil {
		return err
	}

	return nil
}

func (cv *Validator) Delete(_ *harvesterv1.VirtualMachineImage) error {
	return nil
}

func (cv *Validator) checkVolumeModeAccessModes(vmImg *harvesterv1.VirtualMachineImage) error {
	if vmImg.Spec.TargetStorageClassName == "" {
		return werror.NewInvalidError("targetStorageClassName is required", "spec.targetStorageClassName")
	}

	// storage profile name is always the same as storage class name
	sp, err := cv.spCache.Get(vmImg.Spec.TargetStorageClassName)
	if err != nil {
		return werror.NewInvalidError(fmt.Sprintf("failed to get storage profile %q: %v", vmImg.Spec.TargetStorageClassName, err), "spec.targetStorageClassName")
	}

	if len(sp.Spec.ClaimPropertySets) != 0 {
		return nil
	}

	sc, err := cv.scCache.Get(vmImg.Spec.TargetStorageClassName)
	if err != nil {
		return werror.NewInvalidError(fmt.Sprintf("failed to get storage class %q: %v", vmImg.Spec.TargetStorageClassName, err), "spec.targetStorageClassName")
	}
	if _, found := cdicaps.GetCapabilities(cv.client, sc); !found {
		return werror.NewInvalidError(
			fmt.Sprintf("no default volume mode or access mode, please specify %s annotation in storage class %q",
				util.AnnotationStorageProfileVolumeModeAccessModes, vmImg.Spec.TargetStorageClassName),
			"spec.targetStorageClassName")
	}

	return nil
}
