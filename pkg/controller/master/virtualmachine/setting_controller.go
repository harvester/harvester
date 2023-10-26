package virtualmachine

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	kubevirtctrl "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/settings"
)

type SettingController struct {
	vmController kubevirtctrl.VirtualMachineController
	vmCache      kubevirtctrl.VirtualMachineCache
}

func (h *SettingController) OnVMForceRestartPolicyChanged(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil ||
		setting.Name != settings.VMForceRestartPolicySettingName || setting.Value == "" {
		return setting, nil
	}

	vmForceRestartPolicy, err := settings.DecodeVMForceRestartPolicy(setting.Value)
	if err != nil {
		return nil, err
	}

	if !vmForceRestartPolicy.Enable {
		return setting, nil
	}

	vms, err := h.vmCache.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, vm := range vms {
		h.vmController.Enqueue(vm.Namespace, vm.Name)
	}
	return setting, nil
}
