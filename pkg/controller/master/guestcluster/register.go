package guestcluster

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	guestClusterVMController = "GuestClusterVMController"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	var (
		vmClient     = management.VirtFactory.Kubevirt().V1().VirtualMachine()
		vmCache      = vmClient.Cache()
		settingCache = management.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache()
	)

	// registers the vm controller
	var vmCtrl = &VMController{
		vmClient:     vmClient,
		vmCache:      vmCache,
		vmController: vmClient,

		settingCache: settingCache,
	}
	var virtualMachineClient = management.VirtFactory.Kubevirt().V1().VirtualMachine()
	virtualMachineClient.OnChange(ctx, guestClusterVMController, vmCtrl.OnChange)
	virtualMachineClient.OnRemove(ctx, guestClusterVMController, vmCtrl.OnDelete)

	return nil
}
