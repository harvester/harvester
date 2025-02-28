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

	// check pvc is related to the vm image
	_, err := m.vmImageCache.Get(pvc.Namespace, pvc.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("PVC %s/%s is not related to the VM image, skip patch", pvc.Namespace, pvc.Name)
			return nil, nil
		}
		return nil, err
	}

	annotations := pvc.GetAnnotations()

	if _, find := annotations[util.AnnotationGoldenImage]; find {
		if annotations[util.AnnotationGoldenImage] == "true" {
			return nil, nil
		}
	}
	annotations[util.AnnotationGoldenImage] = "true"

	annoVal, err := json.Marshal(annotations)
	if err != nil {
		logrus.Warnf("failed to marshal annotations: %v, err: %v", annotations, err)
		return nil, err
	}

	// patch annotation
	return []string{fmt.Sprintf(`{"op": "replace", "path": "/metadata/annotations", "value": %s}`, string(annoVal))}, nil
}
