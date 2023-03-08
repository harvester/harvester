package namespace

import (
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/resourcequota"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type namespaceValidator struct {
	types.DefaultValidator

	vmCache       ctlkubevirtv1.VirtualMachineCache
	vmrCache      ctlharvesterv1.VirtualMachineRestoreCache
	vmbackupCache ctlharvesterv1.VirtualMachineBackupCache
	vmimCache     ctlkubevirtv1.VirtualMachineInstanceMigrationCache

	arq *resourcequota.AvailableResourceQuota
}

func NewValidator(vmCache ctlkubevirtv1.VirtualMachineCache,
	vmimCache ctlkubevirtv1.VirtualMachineInstanceMigrationCache,
	vmbackupCache ctlharvesterv1.VirtualMachineBackupCache,
	vmrCache ctlharvesterv1.VirtualMachineRestoreCache,
) types.Validator {
	return &namespaceValidator{
		vmCache:       vmCache,
		vmimCache:     vmimCache,
		vmbackupCache: vmbackupCache,
		vmrCache:      vmrCache,

		arq: resourcequota.NewAvailableResourceQuota(vmCache, vmimCache, vmbackupCache, vmrCache, nil),
	}
}

func (v *namespaceValidator) Create(request *types.Request, newObj runtime.Object) error {
	return v.reconcile(newObj)
}

func (v *namespaceValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	return v.reconcile(newObj)
}

func (v *namespaceValidator) reconcile(newObj runtime.Object) error {
	ns := newObj.(*corev1.Namespace)
	if ns == nil || ns.DeletionTimestamp != nil || ns.Annotations == nil {
		return nil
	}

	_, ok := ns.Annotations[util.AnnotationMaintenanceQuota]
	if !ok {
		return nil
	}

	if err := v.arq.ValidateMaintenanceResourcesField(ns); err != nil {
		return err
	}
	return v.arq.ValidateAvailableResources(ns)
}

func (v *namespaceValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"namespaces"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   corev1.SchemeGroupVersion.Group,
		APIVersion: corev1.SchemeGroupVersion.Version,
		ObjectType: &corev1.Namespace{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}
