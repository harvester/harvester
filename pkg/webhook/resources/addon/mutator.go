package addon

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/sirupsen/logrus"

	admissionregv1 "k8s.io/api/admissionregistration/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
		APIGroup:       harvesterv1.SchemeGroupVersion.Group,
		APIVersion:     harvesterv1.SchemeGroupVersion.Version,
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

	addonOperation := harvesterv1.AddonDisableOperation

	if newAddon.Spec.Enabled {
		addonOperation = harvesterv1.AddonEnableOperation
	}

	return patchLastOperation(newAddon, patchOps, string(addonOperation))
}

func (m *addonMutator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	newAddon := newObj.(*harvesterv1.Addon)
	oldAddon := oldObj.(*harvesterv1.Addon)

	addonOperation := harvesterv1.AddonUpdateOperation

	if newAddon.Spec.Enabled != oldAddon.Spec.Enabled {
		if newAddon.Spec.Enabled {
			addonOperation = harvesterv1.AddonEnableOperation
		} else {
			addonOperation = harvesterv1.AddonDisableOperation
		}
	}

	var patchOps types.PatchOps

	return patchLastOperation(newAddon, patchOps, string(addonOperation))
}

func patchLastOperation(addon *harvesterv1.Addon, patchOps types.PatchOps, addonOperation string) (types.PatchOps, error) {
	jsonOp1 := "add"
	jsonOp2 := "add"
	if addon.Annotations == nil {
		addon.Annotations = make(map[string]string, 2)
	} else {
		if _, ok := addon.Annotations[util.AnnotationAddonLastOperation]; ok {
			jsonOp1 = "replace"
		}

		// timestamp is there
		if _, ok := addon.Annotations[util.AnnotationAddonLastOperationTimestamp]; ok {
			jsonOp2 = "replace"
		}
	}

	// patch last operation, the key should be like harvesterhci.io~1addon-last-operation instead of harvesterhci.io/addon-last-operation
	key := strings.Replace(util.AnnotationAddonLastOperation, "/", "~1", 1)
	patchOps = append(patchOps, fmt.Sprintf(`{"op": "%s", "path": "/metadata/annotations/%s", "value": "%s"}`, jsonOp1, key, addonOperation))

	// patch last operation timestamp
	key = strings.Replace(util.AnnotationAddonLastOperationTimestamp, "/", "~1", 1)
	patchOps = append(patchOps, fmt.Sprintf(`{"op": "%s", "path": "/metadata/annotations/%s", "value": "%s"}`, jsonOp2, key, metav1.Now().UTC().Format(time.RFC3339)))

	logrus.Debugf("addon mutation result: %v", patchOps)

	return patchOps, nil
}
