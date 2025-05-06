package backend

import (
	"net/http"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type Backend interface {
	Check(vmi *harvesterv1.VirtualMachineImage) error
	Initialize(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error)
	UpdateVirtualSize(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error)
	Delete(vmi *harvesterv1.VirtualMachineImage) error
	AddSidecarHandler()
}

type Validator interface {
	Create(request *types.Request, vmi *harvesterv1.VirtualMachineImage) error
	Update(oldVMI, newVMI *harvesterv1.VirtualMachineImage) error
	Delete(vmi *harvesterv1.VirtualMachineImage) error
}

type Mutator interface {
	Create(vmi *harvesterv1.VirtualMachineImage) (types.PatchOps, error)
	Update(oldVMI, newVMI *harvesterv1.VirtualMachineImage) (types.PatchOps, error)
}

type Downloader interface {
	DoDownload(vmi *harvesterv1.VirtualMachineImage, rw http.ResponseWriter, req *http.Request) error
	DoCancel(vmi *harvesterv1.VirtualMachineImage) error
}

type Uploader interface {
	DoUpload(vmi *harvesterv1.VirtualMachineImage, req *http.Request) error
}
