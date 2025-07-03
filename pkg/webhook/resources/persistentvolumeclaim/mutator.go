package persistentvolumeclaim

import (
	"encoding/json"
	"fmt"

	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"
	"kubevirt.io/kubevirt/pkg/apimachinery/patch"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewMutator(pvcCache v1.PersistentVolumeClaimCache,
	imageCache ctlharvesterv1.VirtualMachineImageCache) types.Mutator {
	return &pvcMutator{
		pvcCache:     pvcCache,
		vmImageCache: imageCache,
	}
}

type pvcMutator struct {
	types.DefaultMutator
	pvcCache     v1.PersistentVolumeClaimCache
	vmImageCache ctlharvesterv1.VirtualMachineImageCache
}

func (m *pvcMutator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"persistentvolumeclaims"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   corev1.SchemeGroupVersion.Group,
		APIVersion: corev1.SchemeGroupVersion.Version,
		ObjectType: &corev1.PersistentVolumeClaim{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (m *pvcMutator) Create(_ *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	pvc := newObj.(*corev1.PersistentVolumeClaim)

	logrus.Debugf("create PVC %s/%s with mutator", pvc.Namespace, pvc.Name)
	patchOPs := []string{}
	isGoldenImage := false

	// check pvc is related to the vm image
	patchOp, err := m.patchGoldenImageAnnotation(pvc)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, err
	}

	if patchOp != "" || apierrors.IsAlreadyExists(err) {
		isGoldenImage = true
		if patchOp != "" {
			patchOPs = append(patchOPs, patchOp)
		}
	}

	// patch populator
	if !isGoldenImage {
		patchOp, err = m.patchDataSource(pvc)
		if err != nil {
			return nil, err
		}
		patchOPs = append(patchOPs, patchOp)
	}

	// no need to patch
	if len(patchOPs) == 0 {
		return nil, nil
	}
	return patchOPs, nil
}

func (m *pvcMutator) patchGoldenImageAnnotation(pvc *corev1.PersistentVolumeClaim) (string, error) {
	imgObj, err := m.vmImageCache.Get(pvc.Namespace, pvc.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("PVC %s/%s is not related to the VM image, skip patch", pvc.Namespace, pvc.Name)
			return "", nil
		}
		return "", err
	}

	// longhorn backing image based pvc's do not need to be annotated as golden images
	if imgObj.Spec.Backend == v1beta1.VMIBackendBackingImage {
		return "", nil
	}

	annotations := pvc.GetAnnotations()

	if v, find := annotations[util.AnnotationGoldenImage]; find && v == "true" {
		return "", apierrors.NewAlreadyExists(schema.GroupResource{}, pvc.Name)
	}

	// patch annotation
	return fmt.Sprintf(`{"op": "replace", "path": "/metadata/annotations/%s", "value": "true"}`, patch.EscapeJSONPointer(util.AnnotationGoldenImage)), nil
}

func (m pvcMutator) patchDataSource(pvc *corev1.PersistentVolumeClaim) (string, error) {
	// do no-op if volume mode is block
	if pvc.Spec.VolumeMode != nil && *pvc.Spec.VolumeMode == corev1.PersistentVolumeBlock {
		logrus.Debugf("PVC %s/%s is block volume, skip patch", pvc.Namespace, pvc.Name)
		return "", nil
	}
	annotations := pvc.GetAnnotations()
	if v, find := annotations[util.AnnotationVolForVM]; !find || v != "true" {
		logrus.Debugf("PVC %s/%s is not for VM, skip patch", pvc.Namespace, pvc.Name)
		return "", nil
	}

	// do not replace any existing data source, such as csi snapshot, with the
	// filesystem-blank-source VolumeImportSource. if set, the 'blank'
	// VolumeImportSource would wipe any restore or prepopulated data.
	if pvc.Spec.DataSourceRef != nil || pvc.Spec.DataSource != nil {
		dataSource := ""
		if pvc.Spec.DataSourceRef != nil {
			dataSource = fmt.Sprintf("%+v", pvc.Spec.DataSourceRef)
		} else if pvc.Spec.DataSource != nil {
			dataSource = fmt.Sprintf("%+v", pvc.Spec.DataSource)
		}
		pvcName := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
		logrus.Debugf("PVC %s has existing data source: %s, skip patch", pvcName, dataSource)
		return "", nil
	}

	cdiAPIGroup := cdicommon.AnnAPIGroup
	dataSourceRef := corev1.TypedLocalObjectReference{
		APIGroup: &cdiAPIGroup,
		Kind:     util.KindVolumeImportSource,
		Name:     util.ImportSourceFSBlank,
	}
	dataSourceRefVal, err := json.Marshal(dataSourceRef)
	if err != nil {
		logrus.Warnf("failed to marshal data source ref: %v, err: %v", dataSourceRef, err)
		return "", err
	}

	// patch annotation
	return fmt.Sprintf(`{"op": "replace", "path": "/spec/dataSourceRef", "value": %s}`, string(dataSourceRefVal)), nil
}
