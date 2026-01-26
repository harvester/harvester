package migration

import (
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlharvcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlvirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

type Handler struct {
	namespace      string
	rqs            ctlharvcorev1.ResourceQuotaClient
	rqCache        ctlharvcorev1.ResourceQuotaCache
	vmiCache       ctlvirtv1.VirtualMachineInstanceCache
	vms            ctlvirtv1.VirtualMachineClient
	vmCache        ctlvirtv1.VirtualMachineCache
	vmimController ctlvirtv1.VirtualMachineInstanceMigrationController
	podCache       ctlcorev1.PodCache
	pods           ctlcorev1.PodClient
	settingCache   ctlharvesterv1.SettingCache
	restClient     rest.Interface
}

func (h *Handler) OnVmiChanged(_ string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	if vmi == nil || vmi.DeletionTimestamp != nil ||
		vmi.Annotations == nil || vmi.Status.MigrationState == nil {
		return vmi, nil
	}

	if isVmiWithHarvesterMigrationAnnotation(vmi) && isVmiMigrationDone(vmi) {
		logrus.Debugf("vmi %s/%s finished migration, reset state", vmi.Namespace, vmi.Name)
		if err := h.resetHarvesterMigrationStateInVmiAndSyncVM(vmi); err != nil {
			logrus.Infof("vmi %s/%s finished migration but fail to reset state %s", vmi.Namespace, vmi.Name, err.Error())
			return vmi, err
		}
	}

	return vmi, nil
}
