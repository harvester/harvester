package cdi

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type Mutator struct {
	vmim common.VMIMutator
}

func GetMutator(vmim common.VMIMutator) backend.Mutator {
	return &Mutator{vmim}
}

func (cm *Mutator) Create(vmImg *harvesterv1.VirtualMachineImage) (types.PatchOps, error) {
	return cm.vmim.EnsureTargetSC(vmImg)
}

func (cm *Mutator) Update(_, newVMImg *harvesterv1.VirtualMachineImage) (types.PatchOps, error) {
	return cm.vmim.EnsureTargetSC(newVMImg)
}
