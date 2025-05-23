package cdi

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
