package cdi

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type Mutator struct {
}

func GetMutator() backend.Mutator {
	return &Mutator{}
}

func (cm *Mutator) Create(_ *harvesterv1.VirtualMachineImage) (types.PatchOps, error) {
	return nil, nil
}
