package cdi

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
)

type Backend struct {
}

func GetBackend() backend.Backend {
	return &Backend{}
}

func (cb *Backend) Initialize(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	return vmi, nil
}

func (cb *Backend) Check(_ *harvesterv1.VirtualMachineImage) error {
	return nil
}

func (cb *Backend) UpdateVirtualSize(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	return vmi, nil
}

func (cb *Backend) Delete(_ *harvesterv1.VirtualMachineImage) error {
	return nil
}

func (cb *Backend) AddSidecarHandler() {
}
