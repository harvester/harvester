package machine

import (
	"context"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/harvester/harvester/pkg/config"
	ctlclusterv1 "github.com/harvester/harvester/pkg/generated/controllers/cluster.x-k8s.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	machineControlPlaneControllerName = "machine-control-plane-controller"
)

// machineControlPlaneHandler add cluster.x-k8s.io/control-plane to machine
// if rke.cattle.io/control-plane-role label is true
type machineControlPlaneHandler struct {
	machineClient ctlclusterv1.MachineClient
}

// machineControlPlaneHandler registers a controller to sync labels
func ControlPlaneRegister(ctx context.Context, management *config.Management, _ config.Options) error {
	machines := management.ClusterFactory.Cluster().V1beta1().Machine()
	handler := &machineControlPlaneHandler{
		machineClient: machines,
	}

	machines.OnChange(ctx, machineControlPlaneControllerName, handler.OnMachineChanged)

	return nil
}

func (h *machineControlPlaneHandler) OnMachineChanged(_ string, machine *clusterv1.Machine) (*clusterv1.Machine, error) {
	if machine == nil || machine.DeletionTimestamp != nil || machine.Labels == nil {
		return machine, nil
	}

	if v1, ok := machine.Labels[util.RKEControlPlaneRoleLabel]; ok && v1 == "true" {
		if v2, ok := machine.Labels[clusterv1.MachineControlPlaneLabel]; !ok || v2 != "true" {
			machineCopy := machine.DeepCopy()
			machineCopy.Labels[clusterv1.MachineControlPlaneLabel] = "true"
			return h.machineClient.Update(machineCopy)
		}
	}
	return nil, nil
}
