package datavolume

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	KindVMImage = "VirtualMachineImage"
)

func NewValidator(vmCache ctlkv1.VirtualMachineCache,
	imageCache ctlharvesterv1.VirtualMachineImageCache) types.Validator {
	return &dataVolumeValidator{
		vmCache:    vmCache,
		imageCache: imageCache,
	}
}

type dataVolumeValidator struct {
	types.DefaultValidator
	vmCache    ctlkv1.VirtualMachineCache
	imageCache ctlharvesterv1.VirtualMachineImageCache
}

func (v *dataVolumeValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"datavolumes"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   cdiv1.SchemeGroupVersion.Group,
		APIVersion: cdiv1.SchemeGroupVersion.Version,
		ObjectType: &cdiv1.DataVolume{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Delete,
		},
	}
}

func (v *dataVolumeValidator) Delete(request *types.Request, curObj runtime.Object) error {
	if request.IsGarbageCollection() || request.IsFromController() {
		return nil
	}

	curDV := curObj.(*cdiv1.DataVolume)
	dvName := curDV.Name
	dvNamespace := curDV.Namespace

	logrus.Debugf("checking DataVolume delete validation for %s/%s", dvNamespace, dvName)

	// check it owns by VMImage
	if curDV.OwnerReferences != nil {
		for _, owner := range curDV.OwnerReferences {
			if owner.Kind == KindVMImage {
				vmImageName := owner.Name
				vmImage, err := v.imageCache.Get(dvNamespace, vmImageName)
				if err != nil && !apierrors.IsNotFound(err) {
					return werror.NewInternalError(fmt.Sprintf("failed to get VMImage %s/%s: %s", dvNamespace, vmImageName, err))
				}
				if vmImage.GetUID() == owner.UID {
					return werror.NewInvalidError(fmt.Sprintf("can not delete DataVolume %s/%s which is owned by VMImage %s/%s", dvNamespace, dvName, dvNamespace, vmImageName), "")
				}
			}
		}
	}

	// check it related to the VM
	vms, err := v.vmCache.List(dvNamespace, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to list VMs in namespace %s: %s", dvNamespace, err))
	}
	for _, vm := range vms {
		if dataVolumeIsAttachedToVM(vm, curDV) {
			return werror.NewInvalidError(fmt.Sprintf("can not delete DataVolume %s/%s which is attached to VM %s/%s", dvNamespace, dvName, dvNamespace, vm.Name), "")
		}
	}

	return nil
}

func dataVolumeIsAttachedToVM(vm *kubevirtv1.VirtualMachine, curDV *cdiv1.DataVolume) bool {
	volumeClaimTemplates, ok := vm.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok || volumeClaimTemplates == "" {
		return false
	}
	var pvcs []*corev1.PersistentVolumeClaim
	if err := json.Unmarshal([]byte(volumeClaimTemplates), &pvcs); err != nil {
		logrus.Warnf("failed to unmarshal volumeClaimTemplates: %s, assume the dataVolume is in use", err)
		return false
	}
	for _, pvc := range pvcs {
		if pvc.Name == curDV.Name {
			return true
		}
	}
	return false
}
