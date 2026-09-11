package backingimage

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type Validator struct {
	vmiv common.VMIValidator
	secv common.SecurityParamsVMIValidator
	occv common.OccupationVMIValidator
}

func GetValidator(vmiv common.VMIValidator, secv common.SecurityParamsVMIValidator, occv common.OccupationVMIValidator) backend.Validator {
	return &Validator{vmiv: vmiv, secv: secv, occv: occv}
}

func (biv *Validator) Create(request *types.Request, vmi *harvesterv1.VirtualMachineImage) error {
	if err := biv.vmiv.CheckDisplayName(vmi); err != nil {
		return err
	}

	if err := biv.vmiv.SCConsistency(nil, vmi); err != nil {
		return err
	}

	if err := biv.vmiv.CheckURL(vmi); err != nil {
		return err
	}

	if err := biv.secv.CheckSecurityParameters(request, vmi); err != nil {
		return err
	}

	if err := biv.vmiv.CheckImagePVC(request, vmi); err != nil {
		return err
	}

	return nil
}

func (biv *Validator) Update(oldVMI, newVMI *harvesterv1.VirtualMachineImage) error {
	if err := biv.secv.SCParametersConsistency(oldVMI, newVMI); err != nil {
		return err
	}

	if err := biv.vmiv.SCConsistency(oldVMI, newVMI); err != nil {
		return err
	}

	if err := biv.vmiv.SourceTypeConsistency(oldVMI, newVMI); err != nil {
		return err
	}

	if biv.vmiv.IsExportVolume(newVMI) {
		if err := biv.vmiv.PVCConsistency(oldVMI, newVMI); err != nil {
			return err
		}
	}

	if err := biv.vmiv.URLConsistency(oldVMI, newVMI); err != nil {
		return err
	}

	if err := biv.secv.SecurityParameterConsistency(oldVMI, newVMI); err != nil {
		return err
	}

	if err := biv.vmiv.CheckUpdateDisplayName(oldVMI, newVMI); err != nil {
		return err
	}

	if err := biv.vmiv.CheckURL(newVMI); err != nil {
		return err
	}

	return nil
}

func (biv *Validator) Delete(vmi *harvesterv1.VirtualMachineImage) error {
	if vmi.Status.StorageClassName == "" {
		return nil
	}

	if err := biv.occv.VMTemplateVersionOccupation(vmi); err != nil {
		return err
	}

	if err := biv.occv.PVCOccupation(vmi); err != nil {
		return err
	}

	if err := biv.occv.VMBackupOccupation(vmi); err != nil {
		return err
	}

	return nil
}
