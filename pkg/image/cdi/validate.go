package cdi

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type Validator struct {
}

func GetValidator() backend.Validator {
	return &Validator{}
}

func (cv *Validator) Create(_ *types.Request, _ *harvesterv1.VirtualMachineImage) error {
	return nil
}

func (cv *Validator) Update(_, _ *harvesterv1.VirtualMachineImage) error {
	return nil
}

func (cv *Validator) Delete(_ *harvesterv1.VirtualMachineImage) error {
	return nil
}
