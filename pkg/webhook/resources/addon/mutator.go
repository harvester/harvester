package addon

import (
	//	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewMutator(addons ctlharvesterv1.AddonCache) types.Mutator {
	return &addonMutator{
		addons: addons,
	}
}

// addonMutator injects last operation and timestamp
type addonMutator struct {
	types.DefaultMutator

	addons ctlharvesterv1.AddonCache
}

func newResource(ops []admissionregv1.OperationType) types.Resource {
	return types.Resource{
		Names:          []string{string(harvesterv1.AddonResourceName)},
		Scope:          admissionregv1.NamespacedScope,
		APIGroup:       corev1.SchemeGroupVersion.Group,
		APIVersion:     corev1.SchemeGroupVersion.Version,
		ObjectType:     &harvesterv1.Addon{},
		OperationTypes: ops,
	}
}

func (m *addonMutator) Resource() types.Resource {
	return newResource([]admissionregv1.OperationType{
		admissionregv1.Create,
		admissionregv1.Update,
	})
}

func (m *addonMutator) Create(request *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	newAddon := newObj.(*harvesterv1.Addon)

	var patchOps types.PatchOps

	return patchLastOperation(newAddon, patchOps, "create")
}

func (m *addonMutator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	newAddon := newObj.(*harvesterv1.Addon)
	oldAddon := oldObj.(*harvesterv1.Addon)

	operation := "update"

	if newAddon.Spec.Enabled != oldAddon.Spec.Enabled {
		if newAddon.Spec.Enabled {
			operation = "enable"
		} else {
			operation = "disable"
		}
	}

	var patchOps types.PatchOps

	return patchLastOperation(newAddon, patchOps, operation)
}

func patchLastOperation(addon *harvesterv1.Addon, patchOps types.PatchOps, operation string) (types.PatchOps, error) {
	op1 := "add"
	op2 := "add"
	if addon.Annotations == nil {
		addon.Annotations = make(map[string]string, 2)
	} else {
		if op, ok := addon.Annotations[util.AnnotationAddonLastOperation]; ok {
			// operation not changed, e.g. update content
			if op == operation {
				op1 = ""
			} else {
				op1 = "replace"
			}
		}

		// timestamp is there
		if _, ok := addon.Annotations[util.AnnotationAddonLastOperationTimestamp]; ok {
			op2 = "replace"
		}
	}

	if op1 != "" {
		// patch last operation
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "%s", "path": "/spec/annotations/%s", "value": %s}`, op1, util.AnnotationAddonLastOperation, operation))
	}

	// patch last operation timestamp
	patchOps = append(patchOps, fmt.Sprintf(`{"op": "%s", "path": "/spec/annotations/%s", "value": %s}`, op2, util.AnnotationAddonLastOperationTimestamp, time.Now().UTC().Format(time.RFC3339)))

	return patchOps, nil
}
