package backingimage

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

func (bim *Mutator) Create(vmi *harvesterv1.VirtualMachineImage) (types.PatchOps, error) {
	return bim.vmim.PatchImageSCParams(vmi)
}
