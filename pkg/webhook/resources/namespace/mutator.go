package namespace

import (
	"encoding/json"
	"strings"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/resourcequota"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type namespaceMutator struct {
	types.DefaultMutator
}

func NewMutator() types.Mutator {
	return &namespaceMutator{}
}

func (v *namespaceMutator) Create(request *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	return v.reconcile(newObj)
}

func (v *namespaceMutator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	return v.reconcile(newObj)
}

func (v *namespaceMutator) Resource() types.Resource {
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

func (v *namespaceMutator) reconcile(newObj runtime.Object) (types.PatchOps, error) {
	ns := newObj.(*corev1.Namespace)
	if ns == nil || ns.DeletionTimestamp != nil || ns.Annotations == nil {
		return nil, nil
	}

	_, ok1 := ns.Annotations[util.AnnotationResourceQuota]
	_, ok2 := ns.Annotations[util.AnnotationMaintenanceQuota]
	if !ok1 || !ok2 {
		return v.removeAnnotation(ns)
	}

	return v.addAnnotation(ns)
}

func (v *namespaceMutator) removeAnnotation(ns *corev1.Namespace) (types.PatchOps, error) {
	var removePaths types.PatchOps
	if _, ok := ns.Annotations[util.AnnotationMaintenanceAvailable]; ok {
		removePaths = append(removePaths, genPatch("remove",
			"/metadata/annotations/"+strings.ReplaceAll(util.AnnotationMaintenanceAvailable, "/", "~1"),
			""))
	}
	if _, ok := ns.Annotations[util.AnnotationMaintenanceAvailable]; ok {
		removePaths = append(removePaths, genPatch("remove",
			"/metadata/annotations/"+strings.ReplaceAll(util.AnnotationVMAvailable, "/", "~1"),
			""))
	}

	return removePaths, nil
}

func (v *namespaceMutator) addAnnotation(ns *corev1.Namespace) (types.PatchOps, error) {
	vmAvail, maintAvail, err := resourcequota.CalcAvailableResourcesToString(
		ns.Annotations[util.AnnotationResourceQuota],
		ns.Annotations[util.AnnotationMaintenanceQuota])
	if err != nil {
		return nil, err
	}

	var (
		vmaVerb   = "add"
		maintVerb = "add"
	)
	if _, ok := ns.Annotations[util.AnnotationVMAvailable]; ok {
		vmaVerb = "replace"
	}
	if _, ok := ns.Annotations[util.AnnotationMaintenanceAvailable]; ok {
		maintVerb = "replace"
	}

	return types.PatchOps{
		genPatch(vmaVerb,
			"/metadata/annotations/"+strings.ReplaceAll(util.AnnotationVMAvailable, "/", "~1"),
			vmAvail),
		genPatch(maintVerb,
			"/metadata/annotations/"+strings.ReplaceAll(util.AnnotationMaintenanceAvailable, "/", "~1"),
			maintAvail),
	}, nil
}

func genPatch(op, path, value string) string {
	patch := map[string]interface{}{
		"op":    op,
		"path":  path,
		"value": value,
	}
	b, _ := json.Marshal(patch)
	return string(b)
}
