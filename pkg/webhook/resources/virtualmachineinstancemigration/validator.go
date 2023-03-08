package virtualmachineinstancemigration

import (
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/resourcequota"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(
	ns ctlcorev1.NamespaceCache,
	vms ctlkubevirtv1.VirtualMachineCache,
	vmims ctlkubevirtv1.VirtualMachineInstanceMigrationCache,
) types.Validator {
	return &virtualMachineInstanceMigrationValidator{
		ns:    ns,
		vms:   vms,
		vmims: vmims,

		arq: resourcequota.NewAvailableResourceQuota(vms, vmims, nil, nil, ns),
	}
}

type virtualMachineInstanceMigrationValidator struct {
	types.DefaultValidator

	ns    ctlcorev1.NamespaceCache
	vms   ctlkubevirtv1.VirtualMachineCache
	vmims ctlkubevirtv1.VirtualMachineInstanceMigrationCache

	arq *resourcequota.AvailableResourceQuota
}

func (v *virtualMachineInstanceMigrationValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"virtualmachineinstancemigrations"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   kubevirtv1.SchemeGroupVersion.Group,
		APIVersion: kubevirtv1.SchemeGroupVersion.Version,
		ObjectType: &kubevirtv1.VirtualMachineInstanceMigration{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (v *virtualMachineInstanceMigrationValidator) Create(request *types.Request, newObj runtime.Object) error {
	return v.arq.CheckMaintenanceAvailableResoruces(newObj.(*kubevirtv1.VirtualMachineInstanceMigration))
}
