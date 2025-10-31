package podeviction

import (
	"strings"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	virtv1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(podCache ctlcorev1.PodCache, vmiCache ctlkubevirtv1.VirtualMachineInstanceCache) types.Validator {
	return &podEvictionValidator{
		podCache: podCache,
		vmiCache: vmiCache,
	}
}

type podEvictionValidator struct {
	types.DefaultValidator
	podCache ctlcorev1.PodCache
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache
}

func (v *podEvictionValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"pods/eviction"},
		Scope:      admissionregv1.AllScopes,
		APIGroup:   "",
		APIVersion: "v1",
		ObjectType: &v1.Eviction{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (v *podEvictionValidator) Create(_ *types.Request, newObj runtime.Object) error {
	podEviction := newObj.(*v1.Eviction)

	// only validate virt-launcher pod eviction
	if !strings.HasPrefix(podEviction.Name, "virt-launcher") {
		return nil
	}

	pod, err := v.podCache.Get(podEviction.Namespace, podEviction.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if pod.DeletionTimestamp != nil {
		return nil
	}

	vmiName, exists := pod.GetAnnotations()[virtv1.DomainAnnotation]
	if !exists {
		return nil
	}

	vmi, err := v.vmiCache.Get(podEviction.Namespace, vmiName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if vmi.DeletionTimestamp != nil {
		return nil
	}

	if !vmi.IsMigratable() {
		return werror.NewInvalidError("Eviction of virt-launcher pod for non-migratable VMI is prohibited", "")
	}
	return nil
}
