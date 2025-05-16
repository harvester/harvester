package backingimage

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type Validator struct {
	vmiv common.VMIValidator
}

func GetValidator(vmiv common.VMIValidator) backend.Validator {
	return &Validator{vmiv}
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

	if err := biv.vmiv.CheckSecurityParameters(vmi); err != nil {
		return err
	}

	if err := biv.vmiv.CheckImagePVC(request, vmi); err != nil {
		return err
	}

	return nil
}

func (biv *Validator) Update(oldVMI, newVMI *harvesterv1.VirtualMachineImage) error {
	if err := biv.vmiv.SCParametersConsistency(oldVMI, newVMI); err != nil {
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

	if err := biv.vmiv.SecurityParameterConsistency(oldVMI, newVMI); err != nil {
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
	if biv.vmiv.GetStatusSC(vmi) == "" {
		return nil
	}

	if err := biv.vmiv.VMTemplateVersionOccupation(vmi); err != nil {
		return err
	}

	if err := biv.vmiv.PVCOccupation(vmi); err != nil {
		return err
	}

	if err := biv.vmiv.VMBackupOccupation(vmi); err != nil {
		return err
	}

	return nil
}
