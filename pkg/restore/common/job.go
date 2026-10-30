package common

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

// JobHandler cancels Jobs only after restore failure has been persisted.
// Parent deletion uses the same engine Delete method through its finalizer.
type JobHandler struct {
	Reader VMRestoreReader
	Delete func(*harvesterv1.VirtualMachineRestore) error
}

func (h JobHandler) OnRestoreChange(_ string, vmr *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	if vmr == nil || h.Reader.IsDeleting(vmr) || !h.Reader.IsFailed(vmr) {
		return vmr, nil
	}
	return vmr, h.Delete(vmr)
}
