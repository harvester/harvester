package storageclass

import (
	"fmt"
	"strconv"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"kubevirt.io/kubevirt/pkg/apimachinery/patch"

	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

var patchAnnotation = `{"op": "add", "path": "/metadata/annotations/%s", "value": %s}`

func NewMutator() types.Mutator {
	return &storageClassMutator{}
}

type storageClassMutator struct {
	types.DefaultMutator
}

func newResource(ops []admissionregv1.OperationType) types.Resource {
	return types.Resource{
		Names:          []string{"storageclasses"},
		Scope:          admissionregv1.ClusterScope,
		APIGroup:       storagev1.SchemeGroupVersion.Group,
		APIVersion:     storagev1.SchemeGroupVersion.Version,
		ObjectType:     &storagev1.StorageClass{},
		OperationTypes: ops,
	}
}

func (m *storageClassMutator) Resource() types.Resource {
	return newResource([]admissionregv1.OperationType{
		admissionregv1.Create,
		admissionregv1.Update,
	})
}

func (m *storageClassMutator) Create(_ *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	return generatePatchOps(newObj.(*storagev1.StorageClass)), nil
}

func (m *storageClassMutator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	return generatePatchOps(newObj.(*storagev1.StorageClass)), nil
}

// generatePatchOps generates patch operations for the storage class
func generatePatchOps(sc *storagev1.StorageClass) types.PatchOps {
	var patchOps types.PatchOps

	switch sc.Provisioner {
	case util.CSIProvisionerLonghorn:
		patchOps = append(patchOps, generateLonghornPatchOps(sc)...)
	case util.CSIProvisionerLVM:
		patchOps = append(patchOps, generateLVMPatchOps(sc)...)
	}

	return patchOps
}

func generateLonghornPatchOps(sc *storagev1.StorageClass) types.PatchOps {
	var patchOps types.PatchOps

	if !isLonghornV2(sc) {
		return patchOps
	}

	// Add clone strategy annotation
	if shouldAddAnnotation(sc, util.AnnotationStorageProfileCloneStrategy) {
		patchOps = append(patchOps, fmt.Sprintf(
			patchAnnotation,
			patch.EscapeJSONPointer(util.AnnotationStorageProfileCloneStrategy),
			cdiv1.CloneStrategyHostAssisted,
		))
	}

	// Add snapshot class annotation
	if shouldAddAnnotation(sc, util.AnnotationStorageProfileSnapshotClass) {
		patchOps = append(patchOps, fmt.Sprintf(
			patchAnnotation,
			patch.EscapeJSONPointer(util.AnnotationStorageProfileSnapshotClass),
			"longhorn-snapshot",
		))
	}

	return patchOps
}

func generateLVMPatchOps(sc *storagev1.StorageClass) types.PatchOps {
	var patchOps types.PatchOps

	// Add volume mode access modes annotation
	if shouldAddAnnotation(sc, util.AnnotationStorageProfileVolumeModeAccessModes) {
		json := `{"Block":["ReadWriteOnce"]}`
		patchOps = append(patchOps, fmt.Sprintf(
			patchAnnotation,
			patch.EscapeJSONPointer(util.AnnotationStorageProfileVolumeModeAccessModes),
			strconv.Quote(json),
		))
	}

	// Add clone strategy annotation
	if shouldAddAnnotation(sc, util.AnnotationStorageProfileCloneStrategy) {
		patchOps = append(patchOps, fmt.Sprintf(
			patchAnnotation,
			patch.EscapeJSONPointer(util.AnnotationStorageProfileCloneStrategy),
			cdiv1.CloneStrategySnapshot,
		))
	}

	// Add snapshot class annotation
	if shouldAddAnnotation(sc, util.AnnotationStorageProfileSnapshotClass) {
		patchOps = append(patchOps, fmt.Sprintf(
			patchAnnotation,
			patch.EscapeJSONPointer(util.AnnotationStorageProfileSnapshotClass),
			strconv.Quote("lvm-snapshot"),
		))
	}

	return patchOps
}

func isLonghornV2(sc *storagev1.StorageClass) bool {
	if sc.Parameters == nil {
		return false
	}

	dataEngine, exists := sc.Parameters["dataEngine"]
	return exists && dataEngine == string(longhornv1.DataEngineTypeV2)
}

func shouldAddAnnotation(sc *storagev1.StorageClass, annotation string) bool {
	if sc.Annotations == nil {
		return true
	}

	_, exists := sc.Annotations[annotation]
	return !exists
}
