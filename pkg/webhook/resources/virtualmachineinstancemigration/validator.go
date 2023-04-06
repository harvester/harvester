package virtualmachineinstancemigration

import (
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/resourcequota"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(
	nodeCache ctlcorev1.NodeCache,
	nsCache ctlcorev1.NamespaceCache,
	podCache ctlcorev1.PodCache,
	vmCache ctlkubevirtv1.VirtualMachineCache,
	vmimCache ctlkubevirtv1.VirtualMachineInstanceMigrationCache,
	settingCache ctlharvesterv1.SettingCache,
) types.Validator {
	return &virtualMachineInstanceMigrationValidator{
		nsCache:   nsCache,
		vmCache:   vmCache,
		vmimCache: vmimCache,

		arq: resourcequota.NewAvailableResourceQuota(vmCache, vmimCache, nil, nil, nsCache),
		mtPercentage: resourcequota.NewMaintenancePercentage(
			nodeCache, podCache, nil, vmimCache, settingCache),
	}
}

type virtualMachineInstanceMigrationValidator struct {
	types.DefaultValidator

	nsCache   ctlcorev1.NamespaceCache
	vmCache   ctlkubevirtv1.VirtualMachineCache
	vmimCache ctlkubevirtv1.VirtualMachineInstanceMigrationCache

	arq          *resourcequota.AvailableResourceQuota
	mtPercentage *resourcequota.MaintenancePercentage
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

func (v *virtualMachineInstanceMigrationValidator) Create(request *types.Request, newObj runtime.Object) (err error) {
	// Todo remove
	//return v.arq.CheckMaintenanceAvailableResoruces(newObj.(*kubevirtv1.VirtualMachineInstanceMigration))
	_, err = v.mtPercentage.IsLessAndEqualThanMaintenanceResource(newObj.(*kubevirtv1.VirtualMachineInstanceMigration))
	return
}
